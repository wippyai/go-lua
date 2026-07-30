package bind

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestStableLocalFunctionIdentity(t *testing.T) {
	t.Run("unique local initializer", func(t *testing.T) {
		fn := function(nil)
		definition := localAssign([]string{"target"}, fn)
		bindings := BindChunk([]ast.Stmt{definition}, Options{})
		target := mustLocalAt(t, bindings, definition, 0)
		want := mustFunctionSymbol(t, bindings, fn)

		if got, ok := bindings.StableLocalFunctionIdentity(target); !ok || got != want {
			t.Fatalf("StableLocalFunctionIdentity(%d) = %d/%v, want %d/true", target, got, ok, want)
		}
	})

	t.Run("ambiguous origin", func(t *testing.T) {
		bindings := newResult(Options{})
		target := bindings.newSymbol("target", symbol.Local)
		details := functionOriginDetails{
			kind:            FunctionOriginLocalAssignment,
			targetSymbol:    target,
			hasTargetSymbol: true,
			localIndex:      0,
		}
		bindings.registerFunction(function(nil), nil, details)
		bindings.registerFunction(function(nil), nil, details)

		if got, ok := bindings.StableLocalFunctionIdentity(target); ok || got != 0 {
			t.Fatalf("ambiguous StableLocalFunctionIdentity(%d) = %d/%v, want 0/false", target, got, ok)
		}
	})

	t.Run("rebound local", func(t *testing.T) {
		fn := function(nil)
		definition := localAssign([]string{"target"}, fn)
		rebound := ident("target")
		bindings := BindChunk([]ast.Stmt{
			definition,
			&ast.AssignStmt{Lhs: []ast.Expr{rebound}, Rhs: []ast.Expr{function(nil)}},
		}, Options{})
		target := mustLocalAt(t, bindings, definition, 0)
		if got, ok := bindings.StableLocalFunctionIdentity(target); ok || got != 0 {
			t.Fatalf("rebound StableLocalFunctionIdentity(%d) = %d/%v, want 0/false", target, got, ok)
		}
	})

	t.Run("method origin", func(t *testing.T) {
		bindings := newResult(Options{})
		target := bindings.newSymbol("method", symbol.Local)
		bindings.registerFunction(function(nil), nil, functionOriginDetails{
			kind:            FunctionOriginMethod,
			targetSymbol:    target,
			hasTargetSymbol: true,
			method:          "method",
		})

		if got, ok := bindings.StableLocalFunctionIdentity(target); ok || got != 0 {
			t.Fatalf("method StableLocalFunctionIdentity(%d) = %d/%v, want 0/false", target, got, ok)
		}
	})

	t.Run("global binding", func(t *testing.T) {
		bindings := newResult(Options{})
		target := bindings.global("target", true)
		bindings.registerFunction(function(nil), nil, functionOriginDetails{
			kind:            FunctionOriginDeclaration,
			targetSymbol:    target,
			hasTargetSymbol: true,
		})

		if got, ok := bindings.StableLocalFunctionIdentity(target); ok || got != 0 {
			t.Fatalf("global StableLocalFunctionIdentity(%d) = %d/%v, want 0/false", target, got, ok)
		}
	})
}
