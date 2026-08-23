package bind

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/target/typeindex"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestStatementTraversalKeepsLoopLocalsVisibleAtTheirAuthoredBoundaries(t *testing.T) {
	statements, err := parse.ParseString(`
local total = 0
for index = 1, 2 do
	local next = total + index
	total = next
end
`, "statement_traversal.lua")
	if err != nil {
		t.Fatal(err)
	}
	result := BindChunk(statements, typeindex.Table{})
	loop := statements[1].(*ast.NumberForStmt)
	loopID, ok := result.NumForSymbol(loop)
	if !ok || loopID == 0 {
		t.Fatal("numeric loop declaration has no binder identity")
	}
	bodyLocal := loop.Stmts[0].(*ast.LocalAssignStmt)
	nextID, ok := result.LocalSymbolAt(bodyLocal, 0)
	if !ok || nextID == 0 || nextID == loopID {
		t.Fatalf("body local = %d/%v, want distinct identity from loop %d", nextID, ok, loopID)
	}
	index := bodyLocal.Exprs[0].(*ast.ArithmeticOpExpr).Rhs.(*ast.IdentExpr)
	if got, ok := result.SymbolOf(index); !ok || got != loopID {
		t.Fatalf("loop index read = %d/%v, want loop identity %d", got, ok, loopID)
	}
}
