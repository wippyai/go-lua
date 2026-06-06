package flow

import "github.com/wippyai/go-lua/types/cfg"

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
