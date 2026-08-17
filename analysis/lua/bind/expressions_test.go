package bind

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestExpressionTraversalRecordsDirectGlobalCallsAndNestedReads(t *testing.T) {
	statements, err := parse.ParseString(`
local value = produce(input)
local table = {value = value, [key] = lookup()}
`, "expression_traversal.lua")
	if err != nil {
		t.Fatal(err)
	}
	result := BindChunk(statements)
	calls := result.DirectGlobalCalls()
	if len(calls) != 2 {
		t.Fatalf("DirectGlobalCalls length = %d, want 2", len(calls))
	}
	for index, want := range []string{"produce", "lookup"} {
		if calls[index].Call == nil || !calls[index].Global.Matches(want) {
			t.Fatalf("call[%d] = %#v, want global %q", index, calls[index], want)
		}
	}
	input := statements[0].(*ast.LocalAssignStmt).Exprs[0].(*ast.FuncCallExpr).Args[0].(*ast.IdentExpr)
	key := statements[1].(*ast.LocalAssignStmt).Exprs[0].(*ast.TableExpr).Fields[1].Key.(*ast.IdentExpr)
	for _, ident := range []*ast.IdentExpr{input, key} {
		if !result.IsImplicitGlobalUse(ident) {
			t.Fatalf("nested read %q was not recorded as an implicit global", ident.Value)
		}
	}
}
