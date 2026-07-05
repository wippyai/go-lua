package body

import (
	"github.com/wippyai/go-lua/analysis/check/body/internal/readexpr"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callproducer"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func (r *Result) cachedSourceValue(
	mode sourceValueReadMode,
	point cfg.Point,
	source factflow.ValueSource,
	compute func() (product.Value, bool),
) (product.Value, bool) {
	if r == nil || compute == nil {
		return product.Value{}, false
	}
	key := sourceValueCacheKey{mode: mode, point: point, source: source}
	return r.queries.sourceValue(key, compute)
}

func (r *Result) cachedPathValue(
	mode sourceValueReadMode,
	point cfg.Point,
	p pathdom.Path,
	compute func() (product.Value, bool),
) (product.Value, bool) {
	if r == nil || compute == nil || p.IsEmpty() {
		return product.Value{}, false
	}
	key, ok := r.pathValueCacheKey(mode, point, p)
	if !ok {
		return product.Value{}, false
	}
	return r.queries.pathValue(key, compute)
}

func (r *Result) pathValueCacheKey(mode sourceValueReadMode, point cfg.Point, p pathdom.Path) (pathValueCacheKey, bool) {
	return newPathValueCacheKey(r.pathValueKeySpace(), mode, point, p)
}

func (r *Result) pathValueKeySpace() *keyspace.KeySpace {
	if r == nil || r.visibility == nil {
		return nil
	}
	return r.visibility.KeySpace()
}

func (r *Result) boundaryStateAt(point cfg.Point) (state.State, bool) {
	if r == nil {
		return state.State{}, false
	}
	if r.needsBoundaryNodeOutput(point) {
		if out, ok := r.nodeOutputAt(point); ok {
			return out, true
		}
	}
	return r.solvedStateAt(point)
}

// StateAtBoundary returns the diagnostic/call-boundary state for point. Unlike
// StateAt, this includes the point's boundary transfer when that transfer
// materializes facts needed by same-point consumers, such as object-literal heap
// entries for a call argument.
func (r *Result) StateAtBoundary(point cfg.Point) (state.State, bool) {
	st, ok := r.boundaryStateAt(point)
	if !ok {
		return state.State{}, false
	}
	return st.Snapshot(), true
}

func (r *Result) boundaryRead(point cfg.Point) state.State {
	if out, ok := r.nodeOutputAt(point); ok {
		return out
	}
	if st, ok := r.solvedStateAt(point); ok {
		return st
	}
	return state.State{}
}

func (r *Result) nodeOutputAt(point cfg.Point) (state.State, bool) {
	if r == nil {
		return state.State{}, false
	}
	if out, ok := r.boundary[point]; ok {
		return out, true
	}
	graph := r.Graph()
	if r.registry == nil || graph == nil || r.boundaryXfer == nil {
		return state.State{}, false
	}
	in, ok := r.solvedStateAt(point)
	if !ok {
		return state.State{}, false
	}
	out := r.boundaryXfer(transfer.NodeContext{
		Graph:    graph,
		Registry: r.registry,
		Point:    point,
		Node:     graph.Node(point),
		Read:     r.stateRead,
	}, in)
	if r.boundary == nil {
		r.boundary = make(map[cfg.Point]state.State)
	}
	r.boundary[point] = out
	return out, true
}

func (r *Result) stateRead(point cfg.Point) state.State {
	if st, ok := r.solvedStateAt(point); ok {
		return st
	}
	return state.State{}
}

func (r *Result) needsBoundaryNodeOutput(point cfg.Point) bool {
	if r == nil {
		return false
	}
	if _, ok := r.facts.RootAssignment(point); ok {
		return true
	}
	if _, ok := r.facts.PathAssignment(point); ok {
		return true
	}
	if _, ok := r.facts.PathDescendantInvalidation(point); ok {
		return true
	}
	if _, ok := r.facts.DynamicIndexWrite(point); ok {
		return true
	}
	if _, ok := r.facts.PathStaticMemberWrite(point); ok {
		return true
	}
	if _, ok := r.facts.Return(point); ok {
		return true
	}
	if callproducer.Has(r.facts, point) {
		return true
	}
	if r.callOutcome != nil {
		if _, ok := r.facts.CallSiteView(point); ok {
			return true
		}
	}
	if r.facts.NoNormalReturn(point) {
		return true
	}
	if len(r.facts.CallResultValues(point)) != 0 {
		return true
	}
	if len(r.facts.ChannelSelects(point)) != 0 {
		return true
	}
	if len(r.facts.CovariantExposures(point)) != 0 {
		return true
	}
	return len(r.facts.PostconditionRefinements(point)) != 0 ||
		len(r.facts.PostconditionPathRelations(point)) != 0
}

func (r *Result) readExprConfig(mode sourceValueReadMode) readexpr.Config {
	if r == nil {
		return readexpr.Config{}
	}
	resolver := r.visibility
	var proofState func(cfg.Point) (state.State, bool)
	var proofVisibility *visibility.Resolver
	if mode == sourceValueReadBeforeBoundary {
		proofState = r.boundaryStateAt
		proofVisibility = resolver
		resolver = resolver.Before()
	}
	return readexpr.Config{
		Registry:        r.registry,
		Facts:           r.facts,
		Visibility:      resolver,
		TypeValues:      r.typeValues,
		ProofState:      proofState,
		ProofVisibility: proofVisibility,
	}
}

func (r *Result) boundarySources(mode sourceValueReadMode) sourcevalue.SourceValues {
	if r == nil || r.sources == nil {
		return nil
	}
	if cached := r.queries.sourceResolver(mode); cached != nil {
		return cached
	}
	sources := sourcevalue.WithExpressionValue(r.sources, readexpr.Provider(r.readExprConfig(mode)))
	sources = r.exprRefinements.Bind(r.registry, sources)
	r.queries.rememberSourceResolver(mode, sources)
	return sources
}

func (r *Result) sourceValueAtPoint(mode sourceValueReadMode, point cfg.Point, source factflow.ValueSource, st state.State, read func(cfg.Point) state.State) (product.Value, bool) {
	sources := r.boundarySources(mode)
	if sources == nil {
		return product.Value{}, false
	}
	value, ok := sources.ValueOfSource(point, source, st, read)
	if !ok {
		return product.Value{}, false
	}
	return value, true
}
