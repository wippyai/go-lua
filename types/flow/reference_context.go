package flow

import (
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/lattice"
)

// ReferenceContext is the callee-entry view of caller-owned reference state:
// captured cell values plus function and closure identity paths. These axes are
// projected, overlaid, and keyed together because closure entry and call entry
// semantics depend on their correlation.
type ReferenceContext struct {
	cells        CaptureCells
	functionRefs FunctionRefs
	closureRefs  ClosureRefs
}

// ReferenceContextKey is the comparable cache key for a normalized reference
// context. It preserves axis correlation at cache boundaries without exposing
// independent key fields to callers.
type ReferenceContextKey struct {
	cells        CaptureCellsKey
	functionRefs FunctionRefsKey
	closureRefs  ClosureRefsKey
}

// ReferenceContextOf constructs a canonical reference context from independent
// axes. The map-shaped axes are cloned so callers cannot mutate a stored context.
func ReferenceContextOf(cells CaptureCells, functionRefs FunctionRefs, closureRefs ClosureRefs) ReferenceContext {
	return ReferenceContext{
		cells:        CaptureCellsDomain.Join(cells, CaptureCellsDomain.Bottom()),
		functionRefs: FunctionRefsDomain.Join(functionRefs, FunctionRefsDomain.Bottom()),
		closureRefs:  ClosureRefsDomain.Join(closureRefs, ClosureRefsDomain.Bottom()),
	}
}

// ReferenceContextBottom returns the empty finite reference context.
func ReferenceContextBottom() ReferenceContext {
	return ReferenceContextOf(CaptureCellsDomain.Bottom(), FunctionRefsDomain.Bottom(), ClosureRefsDomain.Bottom())
}

// ReferenceContextTop returns the greatest reference context.
func ReferenceContextTop() ReferenceContext {
	return ReferenceContextOf(CaptureCellsDomain.Top(), FunctionRefsDomain.Top(), ClosureRefsDomain.Top())
}

// ReferenceContextDomain is the product lattice for correlated lexical/reference
// context. Callers use it when a boundary owns all reference axes together.
var ReferenceContextDomain = lattice.Lattice[ReferenceContext]{
	Bottom: ReferenceContextBottom,
	Top:    ReferenceContextTop,
	Equal: func(a, b ReferenceContext) bool {
		return CaptureCellsDomain.Equal(a.CaptureCells(), b.CaptureCells()) &&
			FunctionRefsDomain.Equal(a.FunctionRefs(), b.FunctionRefs()) &&
			ClosureRefsDomain.Equal(a.ClosureRefs(), b.ClosureRefs())
	},
	LessOrEq: func(a, b ReferenceContext) bool {
		return CaptureCellsDomain.LessOrEq(a.CaptureCells(), b.CaptureCells()) &&
			FunctionRefsDomain.LessOrEq(a.FunctionRefs(), b.FunctionRefs()) &&
			ClosureRefsDomain.LessOrEq(a.ClosureRefs(), b.ClosureRefs())
	},
	Join: func(a, b ReferenceContext) ReferenceContext {
		return ReferenceContextOf(
			CaptureCellsDomain.Join(a.CaptureCells(), b.CaptureCells()),
			FunctionRefsDomain.Join(a.FunctionRefs(), b.FunctionRefs()),
			ClosureRefsDomain.Join(a.ClosureRefs(), b.ClosureRefs()),
		)
	},
	Meet: nil,
	Widen: func(prev, next ReferenceContext) ReferenceContext {
		return ReferenceContextOf(
			CaptureCellsDomain.Widen(prev.CaptureCells(), next.CaptureCells()),
			FunctionRefsDomain.Widen(prev.FunctionRefs(), next.FunctionRefs()),
			ClosureRefsDomain.Widen(prev.ClosureRefs(), next.ClosureRefs()),
		)
	},
}

// ReferenceContextKeyOf constructs the comparable key for a reference context.
func ReferenceContextKeyOf(c ReferenceContext) ReferenceContextKey {
	return ReferenceContextKey{
		cells:        c.CaptureCells().Key(),
		functionRefs: FunctionRefsKeyOf(c.FunctionRefs()),
		closureRefs:  ClosureRefsKeyOf(c.ClosureRefs()),
	}
}

// ReferenceContextFromPoint extracts the live reference context from a point
// state. Nil points represent the empty caller context.
func ReferenceContextFromPoint(point *PointState) ReferenceContext {
	if point == nil {
		return ReferenceContextOf(CaptureCellsDomain.Bottom(), FunctionRefsDomain.Bottom(), ClosureRefsDomain.Bottom())
	}
	return ReferenceContextOf(point.Cells, point.FunctionRefs, point.ClosureRefs)
}

// CaptureCells returns the captured-cell axis.
func (c ReferenceContext) CaptureCells() CaptureCells {
	return CaptureCellsDomain.Join(c.cells, CaptureCellsDomain.Bottom())
}

// FunctionRefs returns the function-identity axis.
func (c ReferenceContext) FunctionRefs() FunctionRefs {
	return FunctionRefsDomain.Join(c.functionRefs, FunctionRefsDomain.Bottom())
}

// ClosureRefs returns the closure-identity axis.
func (c ReferenceContext) ClosureRefs() ClosureRefs {
	return ClosureRefsDomain.Join(c.closureRefs, ClosureRefsDomain.Bottom())
}

// CaptureCells returns the captured-cell key axis.
func (k ReferenceContextKey) CaptureCells() CaptureCells {
	return k.cells.Cells()
}

// FunctionRefs returns the function-identity key axis.
func (k ReferenceContextKey) FunctionRefs() FunctionRefs {
	return k.functionRefs.Refs()
}

// ClosureRefs returns the closure-identity key axis.
func (k ReferenceContextKey) ClosureRefs() ClosureRefs {
	return k.closureRefs.Refs()
}

// Context reconstructs the normalized reference context represented by this key.
func (k ReferenceContextKey) Context() ReferenceContext {
	return ReferenceContextOf(k.CaptureCells(), k.FunctionRefs(), k.ClosureRefs())
}

// Join adds independent reference evidence from other into c. This is the
// ordinary lattice join for call-entry evidence; use OverlayReferenceContext for
// mutable closure-entry snapshots where live locations override older captures.
func (c ReferenceContext) Join(other ReferenceContext) ReferenceContext {
	return ReferenceContextOf(
		CaptureCellsDomain.Join(c.cells, other.cells),
		FunctionRefsDomain.Join(c.functionRefs, other.functionRefs),
		ClosureRefsDomain.Join(c.closureRefs, other.closureRefs),
	)
}

// CallableIdentity returns only function and closure identity facts. Captured
// lexical cells have distinct value seeding policy at closure entry and should
// not be copied by identity-only projections.
func (c ReferenceContext) CallableIdentity() ReferenceContext {
	return ReferenceContextOf(CaptureCellsDomain.Bottom(), c.functionRefs, c.closureRefs)
}

// RootSymbols returns the finite symbol roots referenced by any axis.
func (c ReferenceContext) RootSymbols() []cfg.SymbolID {
	var symbols []cfg.SymbolID
	for _, entry := range c.CaptureCells().Entries() {
		if entry.Symbol != 0 {
			symbols = append(symbols, entry.Symbol)
		}
	}
	symbols = append(symbols, FunctionRefRootSymbols(c.FunctionRefs())...)
	symbols = append(symbols, ClosureRefRootSymbols(c.ClosureRefs())...)
	return compactSortedSymbols(symbols)
}

// RebaseCallablePaths moves callable identity facts under source to target.
// Captured cells are lexical storage, not callable identity paths, so this
// operation intentionally returns an empty cell axis.
func (c ReferenceContext) RebaseCallablePaths(source, target constraint.Path) ReferenceContext {
	if source.IsEmpty() || target.IsEmpty() {
		return ReferenceContextBottom()
	}
	return ReferenceContextOf(
		CaptureCellsDomain.Bottom(),
		RebaseFunctionRefsPath(c.functionRefs, source, target),
		RebaseClosureRefsPath(c.closureRefs, source, target),
	)
}

// JoinFunctionRefAt additively publishes function identity at path.
func (c ReferenceContext) JoinFunctionRefAt(path constraint.PathKey, set FunctionRefSet) ReferenceContext {
	if set.IsBottom() {
		return c
	}
	refs := c.functionRefs
	if prev, ok := FunctionRefAt(refs, path); ok {
		set = FunctionRefSetDomain.Join(prev, set)
	}
	return ReferenceContextOf(c.cells, WithFunctionRef(refs, path, set), c.closureRefs)
}

// JoinClosureRefAt additively publishes closure identity at path.
func (c ReferenceContext) JoinClosureRefAt(path constraint.PathKey, set ClosureRefSet) ReferenceContext {
	if set.IsBottom() {
		return c
	}
	refs := c.closureRefs
	if prev, ok := ClosureRefAt(refs, path); ok {
		set = ClosureRefSetDomain.Join(prev, set)
	}
	return ReferenceContextOf(c.cells, c.functionRefs, WithClosureRef(refs, path, set))
}

// ProjectPaths keeps only paths visible through projection on every reference
// axis.
func (c ReferenceContext) ProjectPaths(projection ReferencePathProjection) ReferenceContext {
	return ReferenceContextOf(
		c.cells.ProjectPaths(projection),
		ProjectFunctionRefsByReferencePaths(c.functionRefs, projection),
		ProjectClosureRefsByReferencePaths(c.closureRefs, projection),
	)
}

// ProjectSymbols keeps only captured symbols on every reference axis.
func (c ReferenceContext) ProjectSymbols(symbols []cfg.SymbolID) ReferenceContext {
	return ReferenceContextOf(
		c.cells.Project(symbols),
		ProjectFunctionRefsBySymbols(c.functionRefs, symbols),
		ProjectClosureRefsBySymbols(c.closureRefs, symbols),
	)
}

// OverlayReferenceContext returns a closure-entry reference context where live
// captured locations override allocation-time snapshots.
func OverlayReferenceContext(base, live ReferenceContext) ReferenceContext {
	return ReferenceContextOf(
		OverlayCaptureCells(base.cells, live.cells),
		OverlayFunctionRefs(base.functionRefs, live.functionRefs),
		OverlayClosureRefs(base.closureRefs, live.closureRefs),
	)
}
