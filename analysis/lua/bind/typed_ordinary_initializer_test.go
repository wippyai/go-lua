package bind

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestParsedTypedOrdinaryFunctionInitializerIsNotRecursive(t *testing.T) {
	stmts, err := parse.ParseString(`
local typed: () -> number = function(): number
	return typed()
end
`, "typed_recursive_local.lua")
	if err != nil {
		t.Fatal(err)
	}
	definition := stmts[0].(*ast.LocalAssignStmt)
	fn := definition.Exprs[0].(*ast.FunctionExpr)
	call := fn.Stmts[0].(*ast.ReturnStmt).Exprs[0].(*ast.FuncCallExpr)
	self := call.Func.(*ast.IdentExpr)

	result := BindChunk(stmts)
	target := mustLocalAt(t, result, definition, 0)
	got := mustSymbol(t, result, self)
	if got == target {
		t.Fatalf("typed ordinary initializer self-read resolved to new local %d", target)
	}
	if kind, known := result.Kind(got); !known || kind != symbol.Global {
		t.Fatalf("typed ordinary initializer self-read kind = %v/%v, want Global", kind, known)
	}
	if annotation, ok := result.SymbolTypeAnnotation(target); !ok || annotation != definition.Types[0] {
		t.Fatalf("typed local annotation = %T/%v, want exact parsed annotation", annotation, ok)
	}
	if origin := mustOrigin(t, result, fn); origin.Stmt != definition || origin.LocalIndex != 0 {
		t.Fatalf("typed function origin = %#v, want local initializer", origin)
	}
}
