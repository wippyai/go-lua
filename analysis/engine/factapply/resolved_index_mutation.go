package factapply

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

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
}

// ResolvedIndexMutation is an opaque closed N3+N4 transaction.
type ResolvedIndexMutation struct{ data *resolvedIndexMutationData }

type resolvedIndexMutationData struct {
	output     state.State
	candidate  state.State
	laneDeltas []ResolvedIndexMutationLaneDelta
	executions uint8
}

type ResolvedIndexMutationLaneDelta struct {
	Lane    state.LaneID
	Changed bool
}
type ResolvedIndexMutationResult struct {
	Output            state.State
	LaneDeltas        []ResolvedIndexMutationLaneDelta
	Applied, Canceled bool
}

// FreezeResolvedIndexMutation performs all provider/fact/visibility discovery,
// including the precise invalidation member, and owns the complete result.
func FreezeResolvedIndexMutation(request ResolvedIndexMutationFreezeRequest) (ResolvedIndexMutation, error) {
	if request.Context.Registry == nil || request.Resolver == nil {
		return ResolvedIndexMutation{}, fmt.Errorf("factapply: resolved index mutation requires registry and resolver")
	}
	container, table := request.Invalidation.ContainerPathRef(), request.Write.TablePathRef()
	if container.IsEmpty() || table.IsEmpty() || !container.Equal(table) || request.Write.Admission() == 0 {
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
		if !request.HasKeyValue {
			return ResolvedIndexMutation{}, fmt.Errorf("factapply: missing precise invalidation key product")
		}
		if err := putSource(keySource, request.KeyValue); err != nil {
			return ResolvedIndexMutation{}, err
		}
	}
	address, err := FreezeResolvedPathAddress(request.Resolver, request.Context.Point, container)
	if err != nil {
		return ResolvedIndexMutation{}, err
	}
	invData := &resolvedPathDescendantInvalidationData{Container: address}
	read := func(cfg.Point) state.State { return request.Input }
	if precise, ok := freezePreciseDynamicDescendantAddress(request.Context, request.Resolver, sources, read, request.Input, request.Output, request.Invalidation); ok {
		invData.Precise, invData.HasPrecise = precise, true
	}
	inv := ResolvedPathDescendantInvalidation{data: invData}
	intermediate, ok := ApplyResolvedPathDescendantInvalidation(request.Context.Registry, request.Resolver.KeySpace(), request.Output, inv)
	if !ok {
		return ResolvedIndexMutation{}, fmt.Errorf("factapply: invalid frozen invalidation")
	}
	write, ok := freezeResolvedDynamicIndexWrite(request.Context, request.Resolver, request.Facts, sources, read, request.Input, intermediate, request.Write)
	if !ok {
		return ResolvedIndexMutation{}, fmt.Errorf("factapply: invalid frozen dynamic write")
	}
	candidate, ok := ApplyResolvedDynamicIndexWrite(request.Context.Registry, request.Resolver.KeySpace(), intermediate, write)
	if !ok {
		return ResolvedIndexMutation{}, fmt.Errorf("factapply: invalid closed dynamic write")
	}
	deltas, err := resolvedIndexMutationLaneDeltas(request.Context.Registry, request.Output, candidate)
	if err != nil {
		return ResolvedIndexMutation{}, err
	}
	return ResolvedIndexMutation{data: &resolvedIndexMutationData{
		output: request.Output, candidate: candidate,
		laneDeltas: append([]ResolvedIndexMutationLaneDelta(nil), deltas...), executions: 1,
	}}, nil
}

// ApplyResolvedIndexMutation atomically publishes both closed phases or none.
func ApplyResolvedIndexMutation(artifact ResolvedIndexMutation, token *cancellation.Token) (ResolvedIndexMutationResult, error) {
	if artifact.data == nil || artifact.data.executions != 1 {
		return ResolvedIndexMutationResult{}, fmt.Errorf("factapply: invalid resolved index mutation")
	}
	d := artifact.data
	rollback := ResolvedIndexMutationResult{Output: d.output}
	if token != nil && token.Canceled() {
		rollback.Canceled = true
		return rollback, nil
	}
	return ResolvedIndexMutationResult{
		Output:     d.candidate,
		LaneDeltas: append([]ResolvedIndexMutationLaneDelta(nil), d.laneDeltas...),
		Applied:    true,
	}, nil
}

type frozenMutationSources struct {
	values map[factflow.ValueSource]product.Value
}

func (s *frozenMutationSources) ValueOfSource(_ cfg.Point, source factflow.ValueSource, _ state.State, _ func(cfg.Point) state.State) (product.Value, bool) {
	value, ok := s.values[source]
	return value, ok
}

var _ sourcevalue.SourceValues = (*frozenMutationSources)(nil)

func resolvedIndexMutationLaneDeltas(reg *axis.Registry, before, after state.State) ([]ResolvedIndexMutationLaneDelta, error) {
	lanes := state.DefaultLaneCatalog().LaneSet().IDs()
	out := make([]ResolvedIndexMutationLaneDelta, len(lanes))
	for i, lane := range lanes {
		domain, err := state.TryDomainWithLanes(reg, []state.LaneID{lane})
		if err != nil {
			return nil, err
		}
		out[i] = ResolvedIndexMutationLaneDelta{Lane: lane, Changed: !domain.Equal(before, after)}
	}
	return out, nil
}
