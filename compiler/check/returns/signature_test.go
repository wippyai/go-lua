package returns

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/typ"
)

func TestBuildFunctionSignatureWithSummary(t *testing.T) {
	t.Run("nil sig returns nil", func(t *testing.T) {
		result := BuildFunctionSignatureWithSummary(nil, nil)
		if result != nil {
			t.Error("expected nil")
		}
	})

	t.Run("sig with returns preserved", func(t *testing.T) {
		sig := typ.Func().Returns(typ.String).Build()
		result := BuildFunctionSignatureWithSummary(sig, []typ.Type{typ.Number})
		if len(result.Returns) != 1 || result.Returns[0] != typ.String {
			t.Error("expected original returns to be preserved")
		}
	})

	t.Run("empty summary gets unknown", func(t *testing.T) {
		sig := typ.Func().Build()
		result := BuildFunctionSignatureWithSummary(sig, nil)
		if len(result.Returns) != 1 || result.Returns[0] != typ.Unknown {
			t.Error("expected unknown return")
		}
	})
}

func TestBuildFunctionTypeFromSummary(t *testing.T) {
	t.Run("empty returns unknown", func(t *testing.T) {
		result := BuildFunctionTypeFromSummary(nil)
		fn, ok := result.(*typ.Function)
		if !ok || len(fn.Returns) != 1 || fn.Returns[0] != typ.Unknown {
			t.Error("expected function with unknown return")
		}
	})

	t.Run("with returns", func(t *testing.T) {
		result := BuildFunctionTypeFromSummary([]typ.Type{typ.Number, typ.String})
		fn, ok := result.(*typ.Function)
		if !ok || len(fn.Returns) != 2 {
			t.Error("expected function with 2 returns")
		}
	})
}

func TestBuildSeedFunctionTypeWithBindings_ImplicitSelf(t *testing.T) {
	stmts, err := parse.ParseString(`
		function T:m(x)
			return x
		end
	`, "test.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	moduleFn := &ast.FunctionExpr{
		ParList: &ast.ParList{HasVargs: true},
		Stmts:   stmts,
	}
	bindings := bind.Bind(moduleFn, []string{"T"})
	def, ok := stmts[0].(*ast.FuncDefStmt)
	if !ok || def.Func == nil {
		t.Fatal("expected function definition statement")
	}

	fnType, ok := BuildSeedFunctionTypeWithBindings(def.Func, nil, scope.New(), bindings).(*typ.Function)
	if !ok || fnType == nil {
		t.Fatal("expected function seed type")
	}
	if len(fnType.Params) != 2 {
		t.Fatalf("expected 2 params (self + x), got %d", len(fnType.Params))
	}
	if fnType.Params[0].Name != "self" {
		t.Fatalf("expected first param to be self, got %q", fnType.Params[0].Name)
	}
	if fnType.Params[1].Name != "x" {
		t.Fatalf("expected second param to be x, got %q", fnType.Params[1].Name)
	}
}

func TestBuildSeedFunctionTypeWithBindings_WithoutBindings_NoImplicitSelf(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"x"}},
	}
	fnType, ok := BuildSeedFunctionTypeWithBindings(fn, nil, scope.New(), nil).(*typ.Function)
	if !ok || fnType == nil {
		t.Fatal("expected function seed type")
	}
	if len(fnType.Params) != 1 {
		t.Fatalf("expected 1 param without bindings, got %d", len(fnType.Params))
	}
	if fnType.Params[0].Name != "x" {
		t.Fatalf("expected param x, got %q", fnType.Params[0].Name)
	}
}
