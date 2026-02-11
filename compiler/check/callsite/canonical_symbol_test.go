package callsite

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
)

func TestCanonicalSymbolFromExpr_PrefersCandidateByPredicate(t *testing.T) {
	ident := &ast.IdentExpr{Value: "f"}
	primary := bind.NewBindingTable()
	fallback := bind.NewBindingTable()
	primary.Bind(ident, 11)
	fallback.Bind(ident, 22)

	got := CanonicalSymbolFromExpr(
		ident,
		33,
		primary,
		fallback,
		func(sym cfg.SymbolID) bool { return sym == 22 },
	)
	if got != 22 {
		t.Fatalf("CanonicalSymbolFromExpr(...) = %d, want 22", got)
	}
}

func TestCanonicalSymbolFromExpr_FallsBackToFirstNonZero(t *testing.T) {
	ident := &ast.IdentExpr{Value: "f"}
	primary := bind.NewBindingTable()
	primary.Bind(ident, 11)

	got := CanonicalSymbolFromExpr(ident, 0, primary, nil, nil)
	if got != 11 {
		t.Fatalf("CanonicalSymbolFromExpr(...) = %d, want 11", got)
	}
}

func TestCanonicalSymbolFromExpr_UsesFunctionLiteralSymbol(t *testing.T) {
	fn := &ast.FunctionExpr{}
	primary := bind.NewBindingTable()
	fallback := bind.NewBindingTable()
	primary.SetFuncLitSymbol(fn, 41)
	fallback.SetFuncLitSymbol(fn, 42)

	got := CanonicalSymbolFromExpr(
		fn,
		0,
		primary,
		fallback,
		func(sym cfg.SymbolID) bool { return sym == 42 },
	)
	if got != 42 {
		t.Fatalf("CanonicalSymbolFromExpr(function) = %d, want 42", got)
	}
}
