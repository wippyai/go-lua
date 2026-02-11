package callsite

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
)

func TestCalleeSymbolCandidates_UsesCanonicalAndNameFallback(t *testing.T) {
	callee := &ast.IdentExpr{Value: "collect"}
	primary := bind.NewBindingTable()
	fallback := bind.NewBindingTable()

	const primarySym cfg.SymbolID = 11
	const fallbackNamedSym cfg.SymbolID = 22
	primary.Bind(callee, primarySym)
	fallback.SetName(fallbackNamedSym, "collect")

	candidates := CalleeSymbolCandidates(&cfg.CallInfo{
		Callee:     callee,
		CalleeName: "collect",
	}, primary, fallback)
	if len(candidates) != 2 {
		t.Fatalf("len(candidates) = %d, want 2 (%v)", len(candidates), candidates)
	}
	if candidates[0] != primarySym || candidates[1] != fallbackNamedSym {
		t.Fatalf("candidates = %v, want [%d %d]", candidates, primarySym, fallbackNamedSym)
	}
}

func TestCalleeSymbolCandidates_DeduplicatesRawAndCanonical(t *testing.T) {
	callee := &ast.IdentExpr{Value: "f"}
	primary := bind.NewBindingTable()
	const sym cfg.SymbolID = 31
	primary.Bind(callee, sym)

	candidates := CalleeSymbolCandidates(&cfg.CallInfo{
		Callee:       callee,
		CalleeName:   "f",
		CalleeSymbol: sym,
	}, primary, nil)
	if len(candidates) != 1 || candidates[0] != sym {
		t.Fatalf("candidates = %v, want [%d]", candidates, sym)
	}
}

func TestCalleeSymbolCandidates_IncludesPrimaryExprSymbolWhenRawDiffers(t *testing.T) {
	callee := &ast.IdentExpr{Value: "f"}
	primary := bind.NewBindingTable()
	const (
		rawSym  cfg.SymbolID = 51
		exprSym cfg.SymbolID = 52
	)
	primary.Bind(callee, exprSym)

	candidates := CalleeSymbolCandidates(&cfg.CallInfo{
		Callee:       callee,
		CalleeSymbol: rawSym,
	}, primary, nil)
	if len(candidates) != 2 {
		t.Fatalf("len(candidates) = %d, want 2 (%v)", len(candidates), candidates)
	}
	if candidates[0] != rawSym || candidates[1] != exprSym {
		t.Fatalf("candidates = %v, want [%d %d]", candidates, rawSym, exprSym)
	}
}

func TestCalleeSymbolCandidates_IncludesPrimaryNameMatches(t *testing.T) {
	primary := bind.NewBindingTable()
	const byName cfg.SymbolID = 41
	primary.SetName(byName, "f")

	candidates := CalleeSymbolCandidates(&cfg.CallInfo{
		CalleeName: "f",
	}, primary, nil)
	if len(candidates) != 1 || candidates[0] != byName {
		t.Fatalf("candidates = %v, want [%d]", candidates, byName)
	}
}

func TestCalleeSymbolCandidates_IncludeMethodSymbolFromReceiver(t *testing.T) {
	primary := bind.NewBindingTable()
	receiver := &ast.IdentExpr{Value: "T"}
	const (
		receiverSym cfg.SymbolID = 81
		rawSym      cfg.SymbolID = 82
	)
	primary.Bind(receiver, receiverSym)
	methodSym := primary.GetOrCreateFieldSymbol(receiverSym, "foo")

	candidates := CalleeSymbolCandidates(&cfg.CallInfo{
		Method:       "foo",
		Receiver:     receiver,
		CalleeSymbol: rawSym,
	}, primary, nil)
	if len(candidates) < 2 {
		t.Fatalf("expected at least raw and method candidates, got %v", candidates)
	}
	if candidates[0] != rawSym || candidates[1] != methodSym {
		t.Fatalf("candidates = %v, want prefix [%d %d]", candidates, rawSym, methodSym)
	}
}

func TestPreferredCalleeSymbol_PrefersMatchingCandidate(t *testing.T) {
	callee := &ast.IdentExpr{Value: "f"}
	primary := bind.NewBindingTable()
	const (
		rawSym  cfg.SymbolID = 61
		exprSym cfg.SymbolID = 62
	)
	primary.Bind(callee, exprSym)

	got := PreferredCalleeSymbol(&cfg.CallInfo{
		Callee:       callee,
		CalleeSymbol: rawSym,
	}, primary, nil, func(sym cfg.SymbolID) bool { return sym == exprSym })
	if got != exprSym {
		t.Fatalf("preferred symbol = %d, want %d", got, exprSym)
	}
}

func TestPreferredCalleeSymbol_FallsBackToFirstCandidate(t *testing.T) {
	callee := &ast.IdentExpr{Value: "f"}
	primary := bind.NewBindingTable()
	const (
		rawSym  cfg.SymbolID = 71
		exprSym cfg.SymbolID = 72
	)
	primary.Bind(callee, exprSym)

	got := PreferredCalleeSymbol(&cfg.CallInfo{
		Callee:       callee,
		CalleeSymbol: rawSym,
	}, primary, nil, func(sym cfg.SymbolID) bool { return false })
	if got != rawSym {
		t.Fatalf("fallback symbol = %d, want %d", got, rawSym)
	}
}
