package canonical

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/state"
	"github.com/wippyai/go-lua/compiler/check/domain/functionsymbols"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

// pathProjector is the read-only observation boundary from solved product facts
// to path-shaped diagnostic facts. It does not infer new precision: it projects
// FunctionState.InPoints through product domains, StaticMembers, numeric length
// proofs, gradual-top provenance, and callable projection.
type pathProjector struct {
	state             state.FunctionState
	unannotatedParams functionsymbols.Set
	callables         callableProjector
	preferPostState   bool
}

func newPathProjector(fs state.FunctionState, unannotated functionsymbols.Set, callables callableProjector) pathProjector {
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
	return p.pointFacts(point).ReadSymbolValue(sym, p.readPolicy())
}

func (p pathProjector) RefinedPathAt(point cfg.Point, path constraint.Path) flow.TypedValue {
	return p.pointFacts(point).ReadPathType(path, p.readPolicy())
}

func (p pathProjector) RefinedPathValueAt(point cfg.Point, path constraint.Path) flow.ProductValue {
	return p.pointFacts(point).ReadPathValue(path, p.readPolicy())
}

func (p pathProjector) readPolicy() flow.PointReadPolicy {
	return flow.PointReadPolicy{
		UnannotatedSymbol: func(sym cfg.SymbolID) bool {
			return p.unannotatedParams.Contains(sym)
		},
		CallableSignature: p.callableSignatureResolver(),
	}
}

func (p pathProjector) callableSignatureResolver() flow.CallableSignatureResolver {
	return func(query flow.CallableSignatureQuery) (typ.Type, bool) {
		state := query.State
		if query.Ref != (flow.FunctionRef{}) {
			sig := p.callables.FunctionTypeByRef(query.Ref, flow.ReferenceContextFromPoint(&state))
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

func (p pathProjector) pathValueAt(point cfg.Point, path constraint.Path) (product.AbstractValue, bool) {
	return p.pointFacts(point).PathValue(path)
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
