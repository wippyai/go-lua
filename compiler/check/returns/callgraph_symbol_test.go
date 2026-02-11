package returns

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
)

func TestCanonicalLocalSymbol_ResolvesStaticFieldPath(t *testing.T) {
	bindings := bind.NewBindingTable()
	base := &ast.IdentExpr{Value: "M"}
	baseSym := cfg.SymbolID(1001)
	bindings.Bind(base, baseSym)
	bindings.SetName(baseSym, "M")

	localSym := bindings.GetOrCreateFieldSymbol(baseSym, "handlers.run")
	localFuncs := map[cfg.SymbolID]*LocalFuncInfo{
		localSym: {Sym: localSym},
	}

	expr := &ast.AttrGetExpr{
		Object: &ast.AttrGetExpr{
			Object: base,
			Key:    &ast.StringExpr{Value: "handlers"},
		},
		Key: &ast.StringExpr{Value: "run"},
	}

	got := canonicalLocalSymbol(localFuncs, nil, bindings, expr, 0)
	if got != localSym {
		t.Fatalf("canonicalLocalSymbol(M.handlers.run) = %d, want %d", got, localSym)
	}
}

func TestCanonicalLocalSymbol_PrefersKnownLocalOverRaw(t *testing.T) {
	bindings := bind.NewBindingTable()
	base := &ast.IdentExpr{Value: "M"}
	baseSym := cfg.SymbolID(2001)
	bindings.Bind(base, baseSym)
	bindings.SetName(baseSym, "M")

	localSym := bindings.GetOrCreateFieldSymbol(baseSym, "f")
	localFuncs := map[cfg.SymbolID]*LocalFuncInfo{
		localSym: {Sym: localSym},
	}

	expr := &ast.AttrGetExpr{Object: base, Key: &ast.StringExpr{Value: "f"}}
	rawNonLocal := cfg.SymbolID(9999)
	got := canonicalLocalSymbol(localFuncs, nil, bindings, expr, rawNonLocal)
	if got != localSym {
		t.Fatalf("canonicalLocalSymbol should prefer local symbol %d, got %d", localSym, got)
	}
}
