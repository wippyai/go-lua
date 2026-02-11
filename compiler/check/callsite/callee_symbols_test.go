package callsite

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/parse"
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

func TestCalleeSymbolCandidatesWithAliases_IncludesDirectAliasSource(t *testing.T) {
	stmts, err := parse.ParseString(`
		local function B()
			return 1
		end
		local f = B
		local x = f()
	`, "test.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	graph := cfg.Build(&ast.FunctionExpr{Stmts: stmts})
	if graph == nil {
		t.Fatal("expected graph")
	}

	bindings := graph.Bindings()
	if bindings == nil {
		t.Fatal("expected bindings")
	}

	var (
		callInfo  *cfg.CallInfo
		callPoint cfg.Point
	)
	graph.EachCallSite(func(p cfg.Point, info *cfg.CallInfo) {
		if info == nil || info.Callee == nil || callInfo != nil {
			return
		}
		callPoint = p
		callInfo = info
	})
	if callInfo == nil {
		t.Fatal("expected callsite")
	}

	calleeSym := SymbolFromExpr(callInfo.Callee, bindings)
	if calleeSym == 0 {
		t.Fatal("expected non-zero callee symbol from expression")
	}
	aliasSym := graph.DirectAliasSymbol(calleeSym)
	if aliasSym == 0 {
		t.Fatal("expected non-zero direct alias symbol for local f = B")
	}

	if byName, ok := graph.SymbolAt(callPoint, "B"); ok && byName != 0 && byName != aliasSym {
		t.Fatalf("alias symbol = %d, want %d", aliasSym, byName)
	}

	candidates := CalleeSymbolCandidatesWithAliases(callInfo, graph, bindings, nil)
	if len(candidates) < 2 {
		t.Fatalf("expected alias-expanded candidates, got %v", candidates)
	}
	if candidates[0] != calleeSym || candidates[1] != aliasSym {
		t.Fatalf("candidates = %v, want prefix [%d %d]", candidates, calleeSym, aliasSym)
	}
}

func TestPreferredCalleeSymbolWithAliases_PrefersAliasCandidate(t *testing.T) {
	stmts, err := parse.ParseString(`
		local function B()
			return 1
		end
		local f = B
		local x = f()
	`, "test.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	graph := cfg.Build(&ast.FunctionExpr{Stmts: stmts})
	if graph == nil {
		t.Fatal("expected graph")
	}

	bindings := graph.Bindings()
	if bindings == nil {
		t.Fatal("expected bindings")
	}

	var callInfo *cfg.CallInfo
	graph.EachCallSite(func(_ cfg.Point, info *cfg.CallInfo) {
		if info == nil || info.CalleeName != "f" {
			return
		}
		callInfo = info
	})
	if callInfo == nil {
		t.Fatal("expected f() call site")
	}

	calleeSym := SymbolFromExpr(callInfo.Callee, bindings)
	if calleeSym == 0 {
		t.Fatal("expected callee symbol for f")
	}
	aliasSym := graph.DirectAliasSymbol(calleeSym)
	if aliasSym == 0 {
		t.Fatal("expected alias symbol for f")
	}

	got := PreferredCalleeSymbolWithAliases(callInfo, graph, bindings, nil, func(sym cfg.SymbolID) bool {
		return sym == aliasSym
	})
	if got != aliasSym {
		t.Fatalf("preferred symbol = %d, want %d", got, aliasSym)
	}
}
