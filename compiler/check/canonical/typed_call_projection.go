package canonical

import (
	"github.com/wippyai/go-lua/compiler/ast"
	canonicalcall "github.com/wippyai/go-lua/compiler/check/canonical/call"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

// typedCallProjection is the type-carrier counterpart to productCallProjection.
// It keeps typed call projections behind one boundary instead of spreading
// outcome reconstruction across each projected axis.
type typedCallProjection struct {
	typer    callTyper
	call     *ast.FuncCallExpr
	exprType func(ast.Expr) typ.Type
	cells    flow.CaptureCells
	refs     flow.FunctionRefs
	outcome  canonicalcall.CallOutcome
}

func (ct callTyper) typedCallProjection(
	call *ast.FuncCallExpr,
	exprType func(ast.Expr) typ.Type,
	cells flow.CaptureCells,
	refs flow.FunctionRefs,
) (typedCallProjection, bool) {
	if ct.d == nil || call == nil || ct.d.activeProgram == nil {
		return typedCallProjection{}, false
	}
	return typedCallProjection{
		typer:    ct,
		call:     call,
		exprType: exprType,
		cells:    cells,
		refs:     refs,
		outcome:  ct.callOutcomeForTypedCall(call, exprType, cells, refs),
	}, true
}

func (p typedCallProjection) inferredReturnTypes() []typ.Type {
	return p.outcome.InferredReturnTypes()
}
