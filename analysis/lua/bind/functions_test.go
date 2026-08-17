package bind

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestFunctionTraversalPublishesParametersVarargAndCaptureOrder(t *testing.T) {
	statements, err := parse.ParseString(`
local outer = function(first: number, ...: string)
	local captured = first
	return function() return first, captured end
end
`, "function_traversal.lua")
	if err != nil {
		t.Fatal(err)
	}
	outerDecl := statements[0].(*ast.LocalAssignStmt)
	outer := outerDecl.Exprs[0].(*ast.FunctionExpr)
	innerDecl := outer.Stmts[1].(*ast.ReturnStmt).Exprs[0].(*ast.FunctionExpr)
	result := BindChunk(statements)
	params := result.ParamSlots(outer)
	if len(params) != 2 || params[0].Name != "first" || !params[1].Vararg {
		t.Fatalf("ParamSlots = %#v, want fixed and vararg parameters", params)
	}
	if got, ok := result.VarargSymbol(outer); !ok || got != params[1].Symbol {
		t.Fatalf("VarargSymbol = %d/%v, want %d/true", got, ok, params[1].Symbol)
	}
	var captures []Capture
	result.ForEachEntryCapture(func(fn *ast.FunctionExpr, capture Capture) bool {
		if fn == innerDecl {
			captures = append(captures, capture)
		}
		return true
	})
	if len(captures) != 2 || captures[0].Captured != params[0].Symbol {
		t.Fatalf("inner captures = %#v, want first then captured local", captures)
	}
}
