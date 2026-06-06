package canonical

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	canonref "github.com/wippyai/go-lua/compiler/check/canonical/ref"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/compiler/check/canonical/transfer"
	"github.com/wippyai/go-lua/types/flow"
)

// callEntryArgProjection resolves callable identities carried by call arguments
// when projecting caller evidence into a callee entry context. It has two
// evidence modes: solved point state for diagnostic contexts, and product call
// context for transfer-time projection.
type callEntryArgProjection struct {
	program *program
	graph   *cfg.Graph
	typer   callTyper

	transfer *transfer.Transfer
	state    *flow.PointState

	productCtx transfer.ProductCallContext
	hasProduct bool
}

func (c callEntryProjector) pointArgProjection(in *flow.PointState) callEntryArgProjection {
	return callEntryArgProjection{
		program:  c.program,
		graph:    c.graph,
		typer:    c.typer,
		transfer: c.transfer,
		state:    in,
	}
}

func (c callEntryProjector) productArgProjection(ctx transfer.ProductCallContext) callEntryArgProjection {
	return callEntryArgProjection{
		program:    c.program,
		graph:      c.graph,
		typer:      c.typer,
		productCtx: ctx,
		hasProduct: true,
	}
}

func (p callEntryArgProjection) callbackRefs(arg ast.Expr, rawSym cfg.SymbolID) ([]summary.FuncRef, bool) {
	if p.program == nil || p.graph == nil || arg == nil {
		return nil, false
	}
	resolver := p.typer.targetResolver(p.program)
	return resolver.ResolveCallbackArgRefsOrSymbol(arg, p.functionRefs(), rawSym, p.program.refByFunc)
}

func (p callEntryArgProjection) functionArgRefs(arg ast.Expr) (flow.FunctionRefSet, bool) {
	got, ok := p.callbackRefs(arg, 0)
	return functionRefSetFromSummaryRefs(got, ok)
}

func (p callEntryArgProjection) functionArgTreeRefs(arg ast.Expr) (flow.FunctionRefs, bool) {
	if !p.hasProduct && (p.transfer == nil || p.state == nil) {
		return flow.FunctionRefsDomain.Bottom(), false
	}
	call, ok := valueCallExpr(arg)
	if !ok {
		return flow.FunctionRefsDomain.Bottom(), false
	}
	returns := p.typer.CallReturnRefsFromValues(call, p.productContextFor(call)).FunctionRefs
	if len(returns) == 0 || flow.FunctionRefsDomain.Equal(returns[0], flow.FunctionRefsDomain.Bottom()) {
		return flow.FunctionRefsDomain.Bottom(), false
	}
	return returns[0], true
}

func (p callEntryArgProjection) closureArgRefs(arg ast.Expr) (flow.ClosureRefSet, bool) {
	if p.program == nil || arg == nil {
		return flow.ClosureRefSet{}, false
	}
	if fn, ok := arg.(*ast.FunctionExpr); ok && fn != nil {
		ref, ok := p.program.refByFunc(fn)
		if !ok {
			return flow.ClosureRefSet{}, false
		}
		captured := p.program.capturedSymbols(ref)
		projection := p.program.referenceProjection(ref)
		cells := p.captureCells(captured)
		cells = cells.ProjectPaths(projection)
		return flow.ClosureRefSetOf(flow.ClosureRefOf(
			canonref.ToFlow(ref),
			cells,
			flow.ProjectFunctionRefsByReferencePaths(p.functionRefs(), projection),
			flow.ProjectClosureRefsByReferencePaths(p.closureRefs(), projection),
		)), true
	}
	resolver := p.typer.targetResolver(p.program)
	return resolver.ResolveClosureRefSetAtExpr(arg, p.closureRefs())
}

func (p callEntryArgProjection) closureArgTreeRefs(arg ast.Expr) (flow.ClosureRefs, bool) {
	if !p.hasProduct && (p.transfer == nil || p.state == nil) {
		return flow.ClosureRefsDomain.Bottom(), false
	}
	call, ok := valueCallExpr(arg)
	if !ok {
		return flow.ClosureRefsDomain.Bottom(), false
	}
	returns := p.typer.CallReturnRefsFromValues(call, p.productContextFor(call)).ClosureRefs
	if len(returns) == 0 || flow.ClosureRefsDomain.Equal(returns[0], flow.ClosureRefsDomain.Bottom()) {
		return flow.ClosureRefsDomain.Bottom(), false
	}
	return returns[0], true
}

func (p callEntryArgProjection) productContextFor(call *ast.FuncCallExpr) transfer.ProductCallContext {
	if p.hasProduct {
		return p.productCtx.ForCall(call)
	}
	if p.transfer == nil || p.state == nil {
		return transfer.ProductCallContext{}
	}
	return p.transfer.ProductCallContext(p.state, call)
}

func (p callEntryArgProjection) captureCells(captured []cfg.SymbolID) flow.CaptureCells {
	if p.hasProduct {
		cells := p.productCtx.Cells.Project(captured)
		return p.program.normalizeCapturedMethodReceiverCellsFromCells(p.graph, cells, captured)
	}
	if p.state == nil {
		return flow.CaptureCellsDomain.Bottom()
	}
	cells := captureCellsFromPoint(p.state, captured)
	return p.program.normalizeCapturedMethodReceiverCells(p.graph, p.state, cells, captured)
}

func (p callEntryArgProjection) functionRefs() flow.FunctionRefs {
	if p.hasProduct {
		return p.productCtx.FunctionRefs
	}
	if p.state != nil {
		return p.state.FunctionRefs
	}
	return flow.FunctionRefsDomain.Bottom()
}

func (p callEntryArgProjection) closureRefs() flow.ClosureRefs {
	if p.hasProduct {
		return p.productCtx.ClosureRefs
	}
	if p.state != nil {
		return p.state.ClosureRefs
	}
	return flow.ClosureRefsDomain.Bottom()
}

func valueCallExpr(expr ast.Expr) (*ast.FuncCallExpr, bool) {
	switch e := expr.(type) {
	case *ast.FuncCallExpr:
		return e, e != nil
	case *ast.CastExpr:
		return valueCallExpr(e.Expr)
	default:
		return nil, false
	}
}

func functionRefSetFromSummaryRefs(refs []summary.FuncRef, ok bool) (flow.FunctionRefSet, bool) {
	if !ok {
		return flow.FunctionRefSet{}, false
	}
	if len(refs) == 0 {
		return flow.FunctionRefSetTop(), true
	}
	flowRefs := make([]flow.FunctionRef, 0, len(refs))
	for _, ref := range refs {
		if ref == (summary.FuncRef{}) {
			continue
		}
		flowRefs = append(flowRefs, canonref.ToFlow(ref))
	}
	if len(flowRefs) == 0 {
		return flow.FunctionRefSet{}, false
	}
	return flow.FunctionRefSetOf(flowRefs...), true
}
