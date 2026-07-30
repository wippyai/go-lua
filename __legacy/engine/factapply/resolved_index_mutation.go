package factapply

import (
	"context"
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// ApplyBoundaryIndexMutation closes and applies one already-symbolized N3+N4
// pair through the canonical concrete transaction. Facts retain source roles,
// visibility owns addresses, and the supplied products are the exact values
// evaluated by the relation owner; no mutation semantics are reimplemented by
// the boundary executor.
func (a *PathSemanticAuthority) ApplyBoundaryIndexMutation(
	ctx context.Context,
	reg *axis.Registry,
	graph cfg.Graph,
	facts factflow.Facts,
	point cfg.Point,
	keyValue, value product.Value,
	input, output state.State,
	boundaryTable ResolvedPathAddress,
	hasBoundaryTable bool,
) (state.State, error) {
	if ctx == nil || reg == nil || graph == nil || !a.Valid() {
		return state.State{}, fmt.Errorf("factapply: invalid boundary index-mutation authority")
	}
	invalidation, hasInvalidation := facts.PathDescendantInvalidation(point)
	write, hasWrite := facts.DynamicIndexWrite(point)
	if !hasInvalidation || !hasWrite {
		return state.State{}, fmt.Errorf("factapply: boundary index mutation requires its frozen N3+N4 pair")
	}
	session := cancellation.FromContext(ctx)
	artifact, err := FreezeResolvedIndexMutation(ResolvedIndexMutationFreezeRequest{
		Context: transfer.NodeContext{
			Context: ctx, Session: session, Graph: graph, Registry: reg,
			Point: point, Node: graph.Node(point),
		},
		Resolver: a.resolver, Facts: facts, Input: input, Output: output,
		Invalidation: invalidation, Write: write,
		KeyValue: keyValue, Value: value, HasKeyValue: true, HasValue: true,
		BoundaryTable: boundaryTable, HasBoundaryTable: hasBoundaryTable,
	})
	if err != nil {
		return state.State{}, err
	}
	result, err := ApplyResolvedIndexMutation(artifact, tokenOf(session), input, output)
	if err != nil {
		return state.State{}, err
	}
	if result.Canceled {
		if err := ctx.Err(); err != nil {
			return input, err
		}
		return input, context.Canceled
	}
	if !result.Applied {
		return state.State{}, fmt.Errorf("factapply: boundary index mutation was not applied")
	}
	return result.Output, nil
}

// ResolvedIndexMutationFreezeRequest is transient discovery input. Resolver,
// Facts, and source roles are consumed here and never retained by the artifact.
type ResolvedIndexMutationFreezeRequest struct {
	Context               transfer.NodeContext
	Resolver              *visibility.Resolver
	Facts                 factflow.Facts
	Input, Output         state.State
	Invalidation          factflow.PathDescendantInvalidation
	Write                 factflow.DynamicIndexWrite
	KeyValue              product.Value
	Value                 product.Value
	HasKeyValue, HasValue bool
	BoundaryTable         ResolvedPathAddress
	HasBoundaryTable      bool
}

// ResolvedIndexMutation is an opaque closed N3+N4 transaction.
type ResolvedIndexMutation struct{ data *resolvedIndexMutationData }

type resolvedIndexMutationData struct {
	registry       *axis.Registry
	keys           *keyspace.KeySpace
	invalidation   ResolvedPathDescendantInvalidation
	write          ResolvedDynamicIndexWrite
	effectDelta    state.EffectDeltaFactorPlan
	hasEffectDelta bool
}
type ResolvedIndexMutationResult struct {
	Output            state.State
	Applied, Canceled bool
}

// FreezeResolvedIndexMutation performs all provider/fact/visibility discovery,
// including the precise invalidation member, and owns the complete result.
func FreezeResolvedIndexMutation(request ResolvedIndexMutationFreezeRequest) (ResolvedIndexMutation, error) {
	if request.Context.Registry == nil || request.Resolver == nil {
		return ResolvedIndexMutation{}, fmt.Errorf("factapply: resolved index mutation requires registry and resolver")
	}
	container, table := request.Invalidation.ContainerPathRef(), request.Write.TablePathRef()
	invalidationTarget, hasInvalidationTarget := request.Invalidation.DynamicTargetContract()
	writeTarget := request.Write.TargetRef()
	if container.IsEmpty() || table.IsEmpty() || !container.Equal(table) || request.Write.Admission() == 0 ||
		!hasInvalidationTarget || !writeTarget.Equal(invalidationTarget) {
		return ResolvedIndexMutation{}, fmt.Errorf("factapply: invalid resolved index mutation shape")
	}
	sources := &frozenMutationSources{values: make(map[factflow.ValueSource]product.Value, 2)}
	putSource := func(source factflow.ValueSource, value product.Value) error {
		if !source.Valid() {
			return fmt.Errorf("factapply: invalid resolved source role")
		}
		if prior, exists := sources.values[source]; exists && !product.Equal(request.Context.Registry, prior, value) {
			return fmt.Errorf("factapply: conflicting resolved products for one source role")
		}
		sources.values[source] = value
		return nil
	}
	readKey, readValue := dynamicIndexReadback(request.Write.ReadbackIntent())
	if readKey {
		if !request.HasKeyValue {
			return ResolvedIndexMutation{}, fmt.Errorf("factapply: missing resolved key product")
		}
		if err := putSource(request.Write.KeySource(), request.KeyValue); err != nil {
			return ResolvedIndexMutation{}, err
		}
	}
	if readValue {
		if !request.HasValue {
			return ResolvedIndexMutation{}, fmt.Errorf("factapply: missing resolved value product")
		}
		if err := putSource(request.Write.Source(), request.Value); err != nil {
			return ResolvedIndexMutation{}, err
		}
	}
	if _, keySource, _, ok := request.Invalidation.DynamicTargetRef(); ok {
		if request.HasKeyValue {
			if err := putSource(keySource, request.KeyValue); err != nil {
				return ResolvedIndexMutation{}, err
			}
		}
	}
	address := request.BoundaryTable
	if request.HasBoundaryTable {
		if !address.belongsTo(request.Resolver.KeySpace()) || !address.path.Equal(container) {
			return ResolvedIndexMutation{}, fmt.Errorf("factapply: index-mutation boundary table is foreign")
		}
	} else {
		var err error
		address, err = FreezeResolvedPathAddress(request.Resolver, request.Context.Point, container)
		if err != nil {
			return ResolvedIndexMutation{}, fmt.Errorf("factapply: index-mutation container %v at point %d: %w", container, request.Context.Point, err)
		}
	}
	invData := &resolvedPathDescendantInvalidationData{Container: address}
	read := func(cfg.Point) state.State { return request.Input }
	if precise, ok := freezeResolvedIndexPreciseAddress(request, sources, read, address); ok {
		invData.Precise, invData.HasPrecise = precise, true
	}
	inv := ResolvedPathDescendantInvalidation{data: invData}
	intermediate, ok := ApplyResolvedPathDescendantInvalidation(request.Context.Registry, request.Resolver.KeySpace(), request.Output, inv)
	if !ok {
		return ResolvedIndexMutation{}, fmt.Errorf("factapply: invalid frozen invalidation")
	}
	var boundaryTable *ResolvedPathAddress
	if request.HasBoundaryTable {
		boundaryTable = &address
	}
	write, ok := freezeResolvedDynamicIndexWriteAt(request.Context, request.Resolver, request.Facts, sources, read, request.Input, intermediate, request.Write, boundaryTable)
	if !ok {
		return ResolvedIndexMutation{}, fmt.Errorf("factapply: invalid frozen dynamic write")
	}
	data := &resolvedIndexMutationData{
		registry: request.Context.Registry, keys: request.Resolver.KeySpace(), invalidation: inv, write: write,
	}
	if request.HasBoundaryTable {
		key := effectdelta.Key{
			Target: address.rootOrVisibleLocal,
			Site:   callboundary.PathStructuralPreservingInvalidationEffectSite(),
			Kind:   effectdelta.Mutation,
		}
		plan, err := state.RegisteredProductDomain(request.Context.Registry).PrepareEffectDeltaFactorPlan(key, effectdelta.Top())
		if err != nil {
			return ResolvedIndexMutation{}, fmt.Errorf("factapply: invalid boundary effect delta: %w", err)
		}
		data.effectDelta = plan
		data.hasEffectDelta = true
	}
	return ResolvedIndexMutation{data: data}, nil
}

func freezeResolvedIndexPreciseAddress(request ResolvedIndexMutationFreezeRequest, sources sourcevalue.SourceValues, read func(cfg.Point) state.State, table ResolvedPathAddress) (ResolvedPathAddress, bool) {
	if !request.HasBoundaryTable {
		return freezePreciseDynamicDescendantAddress(request.Context, request.Resolver, sources, read, request.Input, request.Output, request.Invalidation)
	}
	_, keySource, suffix, exact := request.Invalidation.DynamicTargetRef()
	if !exact {
		return ResolvedPathAddress{}, false
	}
	key, exact := sources.ValueOfSource(request.Context.Point, keySource, request.Input, readWithCurrentPointState(request.Context.Point, read, request.Output))
	if !exact {
		return ResolvedPathAddress{}, false
	}
	member, exact := staticSegmentFromValue(request.Context.Registry, key)
	if !exact {
		return ResolvedPathAddress{}, false
	}
	root := table.owner.FromPath(table.path.RootOnly())
	address, err := FreezeBoundaryPathAddress(table.owner, root, table.path.Append(member).AppendSegments(suffix))
	return address, err == nil
}

// ApplyResolvedIndexMutation atomically executes the closed ordered program.
// The artifact retains no State or precomputed candidate: cancellation and
// failure return the caller-owned input snapshot exactly.
func ApplyResolvedIndexMutation(artifact ResolvedIndexMutation, token *cancellation.Token, input, output state.State) (ResolvedIndexMutationResult, error) {
	if artifact.data == nil || artifact.data.registry == nil || artifact.data.keys == nil {
		return ResolvedIndexMutationResult{}, fmt.Errorf("factapply: invalid resolved index mutation")
	}
	d := artifact.data
	rollback := ResolvedIndexMutationResult{Output: input}
	if token != nil && token.Canceled() {
		rollback.Canceled = true
		return rollback, nil
	}
	intermediate, ok := ApplyResolvedPathDescendantInvalidation(d.registry, d.keys, output, d.invalidation)
	if !ok {
		return rollback, fmt.Errorf("factapply: resolved descendant invalidation rejected")
	}
	if token != nil && token.Canceled() {
		rollback.Canceled = true
		return rollback, nil
	}
	candidate, ok := ApplyResolvedDynamicIndexWrite(d.registry, d.keys, intermediate, d.write)
	if !ok {
		return rollback, fmt.Errorf("factapply: resolved dynamic write rejected")
	}
	if d.hasEffectDelta {
		written, err := state.RegisteredProductDomain(d.registry).ApplyEffectDelta(d.effectDelta, candidate)
		if err != nil {
			return rollback, fmt.Errorf("factapply: resolved effect delta rejected: %w", err)
		}
		candidate = written
	}
	if token != nil && token.Canceled() {
		rollback.Canceled = true
		return rollback, nil
	}
	return ResolvedIndexMutationResult{Output: candidate, Applied: true}, nil
}

type frozenMutationSources struct {
	values map[factflow.ValueSource]product.Value
}

func (s *frozenMutationSources) ValueOfSource(_ cfg.Point, source factflow.ValueSource, _ state.State, _ func(cfg.Point) state.State) (product.Value, bool) {
	value, ok := s.values[source]
	return value, ok
}

var _ sourcevalue.SourceValues = (*frozenMutationSources)(nil)
