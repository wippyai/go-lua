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
