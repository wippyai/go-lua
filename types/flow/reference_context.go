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

// ReferenceContextOf constructs a canonical reference context from independent
// axes. The map-shaped axes are cloned so callers cannot mutate a stored context.
func ReferenceContextOf(cells CaptureCells, functionRefs FunctionRefs, closureRefs ClosureRefs) ReferenceContext {
	return ReferenceContext{
		cells:        CaptureCellsDomain.Join(cells, CaptureCellsDomain.Bottom()),
		functionRefs: FunctionRefsDomain.Join(functionRefs, FunctionRefsDomain.Bottom()),
		closureRefs:  ClosureRefsDomain.Join(closureRefs, ClosureRefsDomain.Bottom()),
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

// WithFunctionRefs joins additional function identities into this context.
func (c ReferenceContext) WithFunctionRefs(functionRefs FunctionRefs) ReferenceContext {
	return ReferenceContextOf(
		c.cells,
		FunctionRefsDomain.Join(c.functionRefs, functionRefs),
		c.closureRefs,
	)
}

// WithClosureRefs joins additional closure identities into this context.
func (c ReferenceContext) WithClosureRefs(closureRefs ClosureRefs) ReferenceContext {
	return ReferenceContextOf(
		c.cells,
		c.functionRefs,
		ClosureRefsDomain.Join(c.closureRefs, closureRefs),
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
