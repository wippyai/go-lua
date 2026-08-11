package bind_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

// TestLocalFunctionDeclarationIsTheOnlyRecursiveInitializer verifies Lua's
// lexical distinction directly from parsed source.
func TestLocalFunctionDeclarationIsTheOnlyRecursiveInitializer(t *testing.T) {
	sources := []string{
		`local function f(n) if n == 0 then return 0 end return f(n - 1) end return f(3)`,
		`local f = function(n) if n == 0 then return 0 end return f(n - 1) end return f(3)`,
	}
	for index, source := range sources {
		stmts, err := parse.ParseString(source, "local-function-parity.lua")
		if err != nil {
			t.Fatal(err)
		}
		local := stmts[0].(*ast.LocalAssignStmt)
		fn := local.Exprs[0].(*ast.FunctionExpr)
		recur := fn.Stmts[1].(*ast.ReturnStmt).Exprs[0].(*ast.FuncCallExpr).Func.(*ast.IdentExpr)
		bindings := bind.BindChunk(stmts)
		target, ok := bindings.LocalSymbolAt(local, 0)
		if !ok {
			t.Fatal("singleton local target missing")
		}
		got, found := bindings.SymbolOf(recur)
		if !found {
			t.Fatal("self-read symbol missing")
		}
		if index == 0 && got != target {
			t.Fatalf("local-function self-read = %d, want local %d", got, target)
		}
		if index == 1 && got == target {
			t.Fatalf("ordinary initializer self-read = new local %d", target)
		}
		if index == 1 {
			if kind, known := bindings.Kind(got); !known || kind != symbol.Global {
				t.Fatalf("ordinary initializer self-read kind = %v/%v, want Global", kind, known)
			}
		}

	}
}

func TestOrdinaryFunctionInitializerSeesOuterBindingWhileDeclarationSeesItself(t *testing.T) {
	stmts, err := parse.ParseString(`
local f = function() return 1 end
do
	local f = function() return f() end
end
do
	local function f() return f() end
end
`, "local-function-shadow.lua")
	if err != nil {
		t.Fatal(err)
	}
	outer := stmts[0].(*ast.LocalAssignStmt)
	ordinaryBlock := stmts[1].(*ast.DoBlockStmt)
	ordinary := ordinaryBlock.Stmts[0].(*ast.LocalAssignStmt)
	ordinaryFn := ordinary.Exprs[0].(*ast.FunctionExpr)
	ordinaryRead := ordinaryFn.Stmts[0].(*ast.ReturnStmt).Exprs[0].(*ast.FuncCallExpr).Func.(*ast.IdentExpr)
	recursiveBlock := stmts[2].(*ast.DoBlockStmt)
	recursive := recursiveBlock.Stmts[0].(*ast.LocalAssignStmt)
	recursiveFn := recursive.Exprs[0].(*ast.FunctionExpr)
	recursiveRead := recursiveFn.Stmts[0].(*ast.ReturnStmt).Exprs[0].(*ast.FuncCallExpr).Func.(*ast.IdentExpr)
	bindings := bind.BindChunk(stmts)

	outerID, _ := bindings.LocalSymbolAt(outer, 0)
	ordinaryID, _ := bindings.LocalSymbolAt(ordinary, 0)
	recursiveID, _ := bindings.LocalSymbolAt(recursive, 0)
	if got, _ := bindings.SymbolOf(ordinaryRead); got != outerID || got == ordinaryID {
		t.Fatalf("ordinary initializer read = %d, want outer %d not new local %d", got, outerID, ordinaryID)
	}
	if got, _ := bindings.SymbolOf(recursiveRead); got != recursiveID {
		t.Fatalf("local-function declaration read = %d, want self %d", got, recursiveID)
	}
}
