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

// ResolvedSourceValue is one provider-independent ValueSource result. The
// source remains as provenance for the canonical path/equality rules, while
// Value is the already resolved product observed at the mutation boundary.
type ResolvedSourceValue struct {
	Source factflow.ValueSource
	Value  product.Value
}

// ResolvedIndexMutationRequest is the atomic concrete N3+N4 transaction for a
// dynamic index write. Sources contains only already-resolved values: applying
// the request never calls a call-result, expression, or vararg provider.
//
// Facts remains the immutable structural fact snapshot used by the canonical
// append, path-membership, and equality proof gates. Input is the immutable
// source snapshot; Output is the point-local state onto which this transaction
// publishes. Cancellation rolls publication back to Output, never Input.
type ResolvedIndexMutationRequest struct {
	Context      transfer.NodeContext
	Resolver     *visibility.Resolver
	Facts        factflow.Facts
	Input        state.State
	Output       state.State
	Invalidation factflow.PathDescendantInvalidation
	Write        factflow.DynamicIndexWrite
	Sources      []ResolvedSourceValue
	Token        *cancellation.Token
}

// ResolvedIndexMutationLaneDelta records the exact before/after verdict for
// one registered State lane. Results contain one entry for every LaneCatalog
// lane, including unchanged lanes, so catalog growth cannot be missed.
type ResolvedIndexMutationLaneDelta struct {
	Lane    state.LaneID
	Changed bool
}

// ResolvedIndexMutationResult is published only after the complete N3+N4
// transaction succeeds. Canceled and invalid transactions never expose a
// prefix. LaneDeltas is exhaustive when Applied is true.
type ResolvedIndexMutationResult struct {
	Output     state.State
	LaneDeltas []ResolvedIndexMutationLaneDelta
	Applied    bool
	Canceled   bool
}

// ApplyConcreteResolvedIndexMutation executes the same authoritative
// invalidation-then-write functions as the ordinary node executor, but from a
// closed set of resolved source products. This is the shared-kernel parity seam
// for future transformer effect resolvers; it does not activate them.
func ApplyConcreteResolvedIndexMutation(request ResolvedIndexMutationRequest) (ResolvedIndexMutationResult, error) {
	rollback := ResolvedIndexMutationResult{Output: request.Output}
	if request.Context.Registry == nil {
		return rollback, fmt.Errorf("factapply: resolved index mutation requires registry")
	}
	if request.Resolver == nil {
		return rollback, fmt.Errorf("factapply: resolved index mutation requires visibility resolver")
	}
	container := request.Invalidation.ContainerPathRef()
	table := request.Write.TablePathRef()
	if container.IsEmpty() || table.IsEmpty() || !container.Equal(table) {
		return rollback, fmt.Errorf("factapply: resolved index mutation requires one shared invalidation/write table path")
	}
	if request.Write.Admission() == 0 {
		return rollback, fmt.Errorf("factapply: resolved index mutation cannot publish bottom admission")
	}
	sources, err := newResolvedMutationSources(request.Context.Registry, request.Sources)
	if err != nil {
		return rollback, err
	}
	if err := validateResolvedMutationSources(request.Invalidation, request.Write, sources); err != nil {
		return rollback, err
	}
	token := request.Token
	if token == nil && request.Context.Session != nil {
		token = request.Context.Session.Token()
	}
	if token != nil && token.Canceled() {
		rollback.Canceled = true
		return rollback, nil
	}
	read := func(cfg.Point) state.State { return request.Input }
	out := applyPathDescendantInvalidation(
		request.Context, request.Resolver, request.Facts, sources, read,
		request.Input, request.Output, request.Invalidation, false,
	)
	if token != nil && token.Canceled() {
		rollback.Canceled = true
		return rollback, nil
	}
	out = applyDynamicIndexWrite(
		request.Context, request.Resolver, request.Facts, sources, read,
		request.Input, out, request.Write,
	)
	if token != nil && token.Canceled() {
		rollback.Canceled = true
		return rollback, nil
	}
	deltas, err := resolvedIndexMutationLaneDeltas(request.Context.Registry, request.Output, out)
	if err != nil {
		return rollback, err
	}
	return ResolvedIndexMutationResult{Output: out, LaneDeltas: deltas, Applied: true}, nil
}

type resolvedMutationSources struct {
	reg    *axis.Registry
	values map[factflow.ValueSource]product.Value
}

func newResolvedMutationSources(registry *axis.Registry, entries []ResolvedSourceValue) (*resolvedMutationSources, error) {
	out := &resolvedMutationSources{reg: registry, values: make(map[factflow.ValueSource]product.Value, len(entries))}
	for i, entry := range entries {
		if !entry.Source.Valid() {
			return nil, fmt.Errorf("factapply: resolved index mutation source %d is invalid", i)
		}
		if prior, duplicate := out.values[entry.Source]; duplicate {
			if !product.Equal(registry, prior, entry.Value) {
				return nil, fmt.Errorf("factapply: resolved index mutation source %d has conflicting products", i)
			}
			continue
		}
		out.values[entry.Source] = entry.Value
	}
	return out, nil
}

func validateResolvedMutationSources(
	invalidation factflow.PathDescendantInvalidation,
	write factflow.DynamicIndexWrite,
	sources *resolvedMutationSources,
) error {
	if sources == nil {
		return fmt.Errorf("factapply: resolved index mutation has no source set")
	}
	require := func(source factflow.ValueSource, label string) error {
		if _, ok := sources.values[source]; !ok {
			return fmt.Errorf("factapply: resolved index mutation missing %s source", label)
		}
		return nil
	}
	readKey, readValue := dynamicIndexReadback(write.ReadbackIntent())
	if readKey {
		if err := require(write.KeySource(), "key"); err != nil {
			return err
		}
	}
	if readValue {
		if err := require(write.Source(), "value"); err != nil {
			return err
		}
	}
	if _, keySource, _, ok := invalidation.DynamicTargetRef(); ok {
		if err := require(keySource, "precise invalidation key"); err != nil {
			return err
		}
	}
	return nil
}

func (s *resolvedMutationSources) ValueOfSource(_ cfg.Point, source factflow.ValueSource, _ state.State, _ func(cfg.Point) state.State) (product.Value, bool) {
	if s == nil {
		return product.Value{}, false
	}
	value, ok := s.values[source]
	return value, ok
}

var _ sourcevalue.SourceValues = (*resolvedMutationSources)(nil)

func resolvedIndexMutationLaneDeltas(registry *axis.Registry, before, after state.State) ([]ResolvedIndexMutationLaneDelta, error) {
	lanes := state.DefaultLaneCatalog().LaneSet().IDs()
	out := make([]ResolvedIndexMutationLaneDelta, len(lanes))
	for i, lane := range lanes {
		domain, err := state.TryDomainWithLanes(registry, []state.LaneID{lane})
		if err != nil {
			return nil, fmt.Errorf("factapply: resolved index mutation lane %q: %w", lane, err)
		}
		out[i] = ResolvedIndexMutationLaneDelta{Lane: lane, Changed: !domain.Equal(before, after)}
	}
	return out, nil
}
