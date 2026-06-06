package canonical

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	canonref "github.com/wippyai/go-lua/compiler/check/canonical/ref"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/compiler/check/canonical/transfer"
	"github.com/wippyai/go-lua/types/flow"
)

// callEntryArgEvidence is the caller-side evidence needed to resolve function
// and closure identities carried by call arguments.
type callEntryArgEvidence struct {
	references   flow.ReferenceContext
	captureCells func([]cfg.SymbolID) flow.CaptureCells
	nestedCall   func(*ast.FuncCallExpr) transfer.ProductCallContext
}

// callEntryArgProjection resolves callable identities carried by call arguments
// from a normalized caller-evidence carrier.
type callEntryArgProjection struct {
	program *program
	graph   *cfg.Graph
	typer   callTyper

	evidence callEntryArgEvidence
}

func (c callEntryProjector) pointArgProjection(in *flow.PointState) callEntryArgProjection {
	evidence := callEntryArgEvidence{
		captureCells: func(captured []cfg.SymbolID) flow.CaptureCells {
			if c.program == nil || in == nil {
				return flow.CaptureCellsDomain.Bottom()
			}
			cells := captureCellsFromPoint(in, captured)
			return c.program.normalizeCapturedMethodReceiverCells(c.graph, in, cells, captured)
		},
		nestedCall: func(call *ast.FuncCallExpr) transfer.ProductCallContext {
			if c.transfer == nil || in == nil {
				return transfer.ProductCallContext{}
			}
			return c.transfer.ProductCallContext(in, call)
		},
	}
	if in != nil {
		evidence.references = flow.ReferenceContextFromPoint(in)
	}
	return callEntryArgProjection{
		program:  c.program,
		graph:    c.graph,
		typer:    c.typer,
		evidence: evidence,
	}
}

func (c callEntryProjector) productArgProjection(ctx transfer.ProductCallContext) callEntryArgProjection {
	return callEntryArgProjection{
		program: c.program,
		graph:   c.graph,
		typer:   c.typer,
		evidence: callEntryArgEvidence{
			references: ctx.References,
			captureCells: func(captured []cfg.SymbolID) flow.CaptureCells {
				if c.program == nil {
					return flow.CaptureCellsDomain.Bottom()
				}
				cells := ctx.CaptureCells().Project(captured)
				return c.program.normalizeCapturedMethodReceiverCellsFromCells(c.graph, cells, captured)
			},
			nestedCall: ctx.NestedCall,
		},
	}
}

func (p callEntryArgProjection) callbackRefs(arg ast.Expr, rawSym cfg.SymbolID) ([]summary.FuncRef, bool) {
	if p.program == nil || p.graph == nil || arg == nil {
		return nil, false
	}
	resolver := p.typer.targetResolver(p.program)
	return resolver.ResolveCallbackArgRefsOrSymbol(arg, p.evidence.references, rawSym, p.program.refByFunc)
}

func (p callEntryArgProjection) functionArgRefs(arg ast.Expr) (flow.FunctionRefSet, bool) {
	got, ok := p.callbackRefs(arg, 0)
	return functionRefSetFromSummaryRefs(got, ok)
}

func (p callEntryArgProjection) argRefTrees(arg ast.Expr) (flow.ReferenceContext, bool) {
	bottom := flow.ReferenceContextOf(flow.CaptureCellsDomain.Bottom(), flow.FunctionRefsDomain.Bottom(), flow.ClosureRefsDomain.Bottom())
	if p.evidence.nestedCall == nil {
		return bottom, false
	}
	call, ok := valueCallExpr(arg)
	if !ok {
		return bottom, false
	}
	returns := p.typer.ProductCallFromValues(call, p.evidence.nestedCall(call)).ReturnRefs
	references, ok := returns.SlotReferenceContext(0)
	if !ok {
		return bottom, false
	}
	return references, true
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

func (p callEntryArgProjection) captureCells(captured []cfg.SymbolID) flow.CaptureCells {
	if len(captured) == 0 || p.evidence.captureCells == nil {
		return flow.CaptureCellsDomain.Bottom()
	}
	return p.evidence.captureCells(captured)
}

func (p callEntryArgProjection) functionRefs() flow.FunctionRefs {
	return p.evidence.references.FunctionRefs()
}

func (p callEntryArgProjection) closureRefs() flow.ClosureRefs {
	return p.evidence.references.ClosureRefs()
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
