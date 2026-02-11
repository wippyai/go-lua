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

func TestCanonicalLocalCalleeSymbol_UsesCalleeNameFallback(t *testing.T) {
	bindings := bind.NewBindingTable()
	const localSym cfg.SymbolID = 3001
	bindings.SetName(localSym, "runner")

	localFuncs := map[cfg.SymbolID]*LocalFuncInfo{
		localSym: {Sym: localSym},
	}

	callInfo := &cfg.CallInfo{
		CalleeName: "runner",
	}
	got := canonicalLocalCalleeSymbol(localFuncs, nil, bindings, callInfo)
	if got != localSym {
		t.Fatalf("canonicalLocalCalleeSymbol via name fallback = %d, want %d", got, localSym)
	}
}

func TestCanonicalLocalCalleeSymbol_PrefersKnownLocalOverRaw(t *testing.T) {
	bindings := bind.NewBindingTable()
	ident := &ast.IdentExpr{Value: "f"}
	const (
		localSym    cfg.SymbolID = 4001
		rawNonLocal cfg.SymbolID = 4999
	)
	bindings.Bind(ident, localSym)

	localFuncs := map[cfg.SymbolID]*LocalFuncInfo{
		localSym: {Sym: localSym},
	}

	callInfo := &cfg.CallInfo{
		Callee:       ident,
		CalleeSymbol: rawNonLocal,
		CalleeName:   "f",
	}
	got := canonicalLocalCalleeSymbol(localFuncs, nil, bindings, callInfo)
	if got != localSym {
		t.Fatalf("canonicalLocalCalleeSymbol should prefer local symbol %d, got %d", localSym, got)
	}
}

func TestCanonicalLocalCalleeSymbol_ResolvesMethodFromReceiverPath(t *testing.T) {
	bindings := bind.NewBindingTable()
	recv := &ast.IdentExpr{Value: "T"}
	const (
		recvSym     cfg.SymbolID = 5001
		rawNonLocal cfg.SymbolID = 5999
	)
	bindings.Bind(recv, recvSym)
	bindings.SetName(recvSym, "T")
	methodSym := bindings.GetOrCreateFieldSymbol(recvSym, "foo")

	localFuncs := map[cfg.SymbolID]*LocalFuncInfo{
		methodSym: {Sym: methodSym},
	}

	callInfo := &cfg.CallInfo{
		Method:       "foo",
		Receiver:     recv,
		CalleeSymbol: rawNonLocal,
	}

	got := canonicalLocalCalleeSymbol(localFuncs, nil, bindings, callInfo)
	if got != methodSym {
		t.Fatalf("canonicalLocalCalleeSymbol should resolve method symbol %d from receiver path, got %d", methodSym, got)
	}
}
