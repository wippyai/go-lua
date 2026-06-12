package check_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check"
	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
	"github.com/wippyai/go-lua/analysis/domain/effect/signature"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestErrorReturnSignatureRefinesValuePresenceAcrossErrorBranch(t *testing.T) {
	reg := product.DefaultRegistry()
	m := manifest.New("test")
	m.DefineFunctionSignature("f", signature.Function{
		Type: typ.Func().
			Returns(typ.NewOptional(typ.Number), typ.NewOptional(typ.String)).
			Build(),
		Effect: effect.Empty.With(returns.ErrorReturn{ValueIndex: 0, ErrorIndex: 1}),
	})

	stmts := parseChunk(t, `
		local value, err = f()
		if err == nil then
			local ok = value
		else
			local failed = value
		end
	`)
	result, err := check.CheckChunk(stmts, check.Config{
		Registry: reg,
		Signatures: signaturelookup.Source{
			Manifests: []*manifest.Manifest{m},
		},
	})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}

	value := localSymbolAt(t, result, stmts[0].(*ast.LocalAssignStmt), 0)
	branch := firstBranchPoint(t, result)
	thenPoint := branchSuccessor(t, result.Graph(), branch, true)
	elsePoint := branchSuccessor(t, result.Graph(), branch, false)

	thenState := stateAt(t, result, thenPoint)
	assertSymbolPresence(t, reg, thenState, value, presence.Present())
	elseState := stateAt(t, result, elsePoint)
	assertSymbolPresence(t, reg, elseState, value, presence.Absent())

	if diags := diagnostics.Produce(result, diagnostics.Config{Registry: reg}); len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diags)
	}
}

func parseChunk(t *testing.T, src string) []ast.Stmt {
	t.Helper()
	stmts, err := parse.ParseString(src, "error_return_integration_test.lua")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return stmts
}

func localSymbolAt(t *testing.T, result *check.Result, stmt *ast.LocalAssignStmt, index int) symbol.ID {
	t.Helper()
	locals := result.LocalSymbols(stmt)
	if index < 0 || index >= len(locals) || locals[index] == 0 {
		t.Fatalf("local symbol %d missing in %#v", index, locals)
	}
	return locals[index]
}

func firstBranchPoint(t *testing.T, result *check.Result) cfg.Point {
	t.Helper()
	graph := result.Graph()
	for _, point := range graph.RPO() {
		if _, ok := result.BranchCondition(point); ok {
			return point
		}
	}
	t.Fatalf("missing branch condition")
	return 0
}

func branchSuccessor(t *testing.T, graph cfg.Graph, branch cfg.Point, cond bool) cfg.Point {
	t.Helper()
	for _, succ := range graph.Successors(branch) {
		gotCond, ok := graph.EdgeCond(branch, succ)
		if ok && gotCond == cond {
			return succ
		}
	}
	t.Fatalf("missing branch successor for %v edge from %d", cond, branch)
	return 0
}

func stateAt(t *testing.T, result *check.Result, point cfg.Point) state.State {
	t.Helper()
	got, ok := result.StateAt(point)
	if !ok {
		t.Fatalf("missing state at %d", point)
	}
	return got
}

func assertSymbolPresence(t *testing.T, reg *axis.Registry, st state.State, sym symbol.ID, want presence.Value) {
	t.Helper()
	got := product.PresenceOf(st.ReadValue(reg, key.SymbolValue(sym)))
	if !presence.Equal(got, want) {
		t.Fatalf("symbol %d presence = %s, want %s", sym, got, want)
	}
}
