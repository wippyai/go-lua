package cfg

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func localFunctionNames(t *testing.T, code string) map[string]*ast.FunctionExpr {
	t.Helper()
	stmts, err := parse.ParseString(code, "test.lua")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	g := Build(&ast.FunctionExpr{ParList: &ast.ParList{HasVargs: true}, Stmts: stmts})
	out := make(map[string]*ast.FunctionExpr)
	g.EachLocalFunction(func(_ Point, sym SymbolID, fn *ast.FunctionExpr) {
		out[g.NameOf(sym)] = fn
	})
	return out
}

func TestEachLocalFunction_DeclaredThenAssigned(t *testing.T) {
	got := localFunctionNames(t, `
local direct = function() return 1 end
local function named() return 2 end
local dec
dec = function(n) if n == 0 then return 0 end return dec(n - 1) end
local nilled = nil
nilled = function() return 3 end
`)
	for _, name := range []string{"direct", "named", "dec", "nilled"} {
		if got[name] == nil {
			t.Errorf("%s is a local bound to one function literal, got %v", name, got)
		}
	}
}

func TestEachLocalFunction_RejectsRebindings(t *testing.T) {
	got := localFunctionNames(t, `
local twice
twice = function() return 1 end
twice = function() return 2 end
local mixed
mixed = function() return 1 end
mixed = 5
local valued = 5
valued = function() return 1 end
global_fn = function() return 1 end
`)
	for _, name := range []string{"twice", "mixed", "valued", "global_fn"} {
		if got[name] != nil {
			t.Errorf("%s is not bound to a single function literal, got %v", name, got)
		}
	}
}
