package canonical

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/state"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

// pathProjector is the read-only observation boundary from solved product facts
// to path-shaped diagnostic facts. It does not infer new precision: it projects
// FunctionState.InPoints through product domains, StaticMembers, numeric length
// proofs, gradual-top provenance, and callable projection.
type pathProjector struct {
	state             state.FunctionState
	unannotatedParams map[cfg.SymbolID]bool
	callables         callableProjector
	preferPostState   bool
}

func newPathProjector(fs state.FunctionState, unannotated map[cfg.SymbolID]bool, callables callableProjector) pathProjector {
	return pathProjector{
		state:             fs,
		unannotatedParams: unannotated,
		callables:         callables,
	}
}

func (p pathProjector) WithPostState() pathProjector {
	p.preferPostState = true
	return p
}

func (p pathProjector) RefinedValueAt(point cfg.Point, sym cfg.SymbolID) flow.ProductValue {
	if sym == 0 {
		return flow.ProductValue{State: flow.StateUnknown}
	}
	av, ok := p.pointFacts(point).SymbolValue(sym)
	if ok && !av.IsZero() {
		return flow.ProductValue{Value: av, State: flow.StateResolved}
	}
	if p.unannotatedParams[sym] {
		return flow.ProductValue{Value: product.GradualAny(), State: flow.StateResolved}
	}
	return flow.ProductValue{State: flow.StateUnknown}
}

func (p pathProjector) RefinedPathAt(point cfg.Point, path constraint.Path) flow.TypedValue {
	if len(path.Segments) == 0 {
		pv := p.RefinedValueAt(point, path.Symbol)
		if pv.State != flow.StateResolved || pv.Value.IsZero() {
			return flow.TypedValue{Type: nil, State: flow.StateUnknown}
		}
		t := product.ProjectValueOrUnknown(pv.Value)
		if typ.IsAbsentOrUnknown(t) {
			return flow.TypedValue{Type: nil, State: flow.StateUnknown}
		}
		return flow.TypedValue{Type: t, State: flow.StateResolved}
	}
	pv := p.RefinedPathValueAt(point, path)
	if pv.State != flow.StateResolved || pv.Value.IsZero() {
		return flow.TypedValue{Type: nil, State: flow.StateUnknown}
	}
	t := product.ProjectValueOrUnknown(pv.Value)
	if typ.IsAbsentOrUnknown(t) {
		return flow.TypedValue{Type: nil, State: flow.StateUnknown}
	}
	t = p.refineStaticIndexPath(point, path, t)
	return flow.TypedValue{Type: t, State: flow.StateResolved}
}

func (p pathProjector) RefinedPathValueAt(point cfg.Point, path constraint.Path) flow.ProductValue {
	if path.Symbol == 0 {
		return flow.ProductValue{State: flow.StateUnknown}
	}
	if len(path.Segments) == 0 {
		return p.RefinedValueAt(point, path.Symbol)
	}
	if av, ok := p.pointFacts(point).CallablePathValue(path, p.callableSignatureResolver()); ok && !av.IsZero() {
		return flow.ProductValue{Value: av, State: flow.StateResolved}
	}
	if p.unannotatedParams[path.Symbol] {
		root := p.RefinedValueAt(point, path.Symbol)
		if root.State == flow.StateResolved && !root.Value.IsZero() {
			if !root.Value.IsGradualTop() {
				return flow.ProductValue{State: flow.StateUnknown}
			}
			if av, ok := flow.ProductMemberPathValue(root.Value, path.Segments); ok && !av.IsZero() {
				return flow.ProductValue{Value: av, State: flow.StateResolved}
			}
			return flow.ProductValue{State: flow.StateUnknown}
		}
		if av, ok := flow.ProductMemberPathValue(product.GradualAny(), path.Segments); ok && !av.IsZero() {
			return flow.ProductValue{Value: av, State: flow.StateResolved}
		}
		return flow.ProductValue{State: flow.StateUnknown}
	}
	return flow.ProductValue{State: flow.StateUnknown}
}

func (p pathProjector) callableSignatureResolver() flow.CallableSignatureResolver {
	return func(query flow.CallableSignatureQuery) (typ.Type, bool) {
		state := query.State
		if query.Ref != (flow.FunctionRef{}) {
			sig := p.callables.FunctionTypeByRef(query.Ref, state.Cells, state.FunctionRefs, state.ClosureRefs)
			return sig, !typ.IsAbsentOrUnknown(sig)
		}
		if !query.Path.IsEmpty() {
			sig := p.callables.TypeAt(state, query.Path)
			return sig, !typ.IsAbsentOrUnknown(sig)
		}
		return nil, false
	}
}

func (p pathProjector) LengthLowerBoundForPathAt(point cfg.Point, path constraint.Path) (int64, bool) {
	if path.Symbol == 0 {
		return 0, false
	}
	return p.pointFacts(point).LengthLowerBound(path)
}

func (p pathProjector) ObserveChildPaths(q flow.PathChildQuery) []flow.PathFact {
	if q.Path.Symbol == 0 {
		return nil
	}
	if q.View == flow.PathReadPost {
		p = p.WithPostState()
	}
	return p.pointFacts(q.Point).ChildPathFacts(q.Path)
}

func (p pathProjector) refineStaticIndexPath(point cfg.Point, path constraint.Path, t typ.Type) typ.Type {
	if t == nil || len(path.Segments) == 0 {
		return t
	}
	idx := -1
	for i := len(path.Segments) - 1; i >= 0; i-- {
		if path.Segments[i].Kind == constraint.SegmentIndexInt {
			idx = i
			break
		}
	}
	if idx < 0 {
		return t
	}
	seg := path.Segments[idx]
	if seg.Index < 1 {
		return t
	}
	containerPath := constraint.Path{
		Root:     path.Root,
		Symbol:   path.Symbol,
		Version:  path.Version,
		Segments: append([]constraint.Segment(nil), path.Segments[:idx]...),
	}
	lower, ok := p.LengthLowerBoundForPathAt(point, containerPath)
	if !ok && len(containerPath.Segments) == 0 {
		lower, ok = p.lengthLowerBoundAt(point, path.Symbol)
	}
	if !ok || lower < int64(seg.Index) {
		return t
	}
	container, ok := p.pathContainerTypeAt(point, containerPath)
	if !ok || typ.IsAbsentOrUnknown(container) {
		return t
	}
	if refined := narrow.RefineSequenceIndex(container, t, int64(seg.Index)); refined != nil {
		return refined
	}
	return t
}

func (p pathProjector) pathContainerTypeAt(point cfg.Point, path constraint.Path) (typ.Type, bool) {
	if path.Symbol == 0 {
		return nil, false
	}
	if len(path.Segments) == 0 {
		return p.pointFacts(point).SymbolType(path.Symbol)
	}
	return p.pointFacts(point).PathType(path)
}

func (p pathProjector) pathValueAt(point cfg.Point, path constraint.Path) (product.AbstractValue, bool) {
	return p.pointFacts(point).PathValue(path)
}

func (p pathProjector) lengthLowerBoundAt(point cfg.Point, sym cfg.SymbolID) (int64, bool) {
	if sym == 0 {
		return 0, false
	}
	return p.pointFacts(point).LengthLowerBound(constraint.NewPath(sym, ""))
}

func (p pathProjector) inState(point cfg.Point) flow.PointState {
	if p.preferPostState {
		if ps, ok := p.state.Points[point]; ok {
			return ps
		}
	}
	if ps, ok := p.state.InPoints[point]; ok {
		return ps
	}
	return flow.PointStateDomain.Bottom()
}

func (p pathProjector) pointFacts(point cfg.Point) flow.PointFacts {
	return flow.PointFactsOf(p.inState(point))
}
