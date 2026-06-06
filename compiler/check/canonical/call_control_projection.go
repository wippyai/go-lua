package canonical

import (
	"github.com/wippyai/go-lua/compiler/ast"
	canonicalcall "github.com/wippyai/go-lua/compiler/check/canonical/call"
	"github.com/wippyai/go-lua/compiler/check/canonical/transfer"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
)

// callControlProjection owns call-site control effects: parameter refinements
// proven by predicate calls and no-return termination proven by selected module
// callees.
type callControlProjection struct {
	typer callTyper
}

func (ct callTyper) callControlProjection() (callControlProjection, bool) {
	if ct.d == nil || ct.d.activeProgram == nil {
		return callControlProjection{}, false
	}
	return callControlProjection{typer: ct}, true
}

func (p callControlProjection) paramNarrows(call *ast.FuncCallExpr) []transfer.ParamNarrow {
	if call == nil {
		return nil
	}
	prog := p.typer.d.activeProgram
	return (canonicalcall.ParamNarrowProjection{
		Call: call,
		SummaryNarrows: func(call *ast.FuncCallExpr) ([]paramevidence.ParamNarrow, bool) {
			ref, ok := p.typer.resolveCalleeRef(call, prog)
			if !ok {
				return nil, false
			}
			return p.typer.d.summaryReader().ParamNarrows(ref), true
		},
		Resolver: p.typer.callTypeResolver(nil),
	}).Narrows()
}
