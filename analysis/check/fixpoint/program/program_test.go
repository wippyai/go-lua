package program

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/ref"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/standard"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestRunBoundChunkUsesSuppliedBindIdentityForLocalCallee(t *testing.T) {
	reg := standard.Registry()
	want := product.Top()
	stmts := parseChunk(t, `
local f = function()
	return 1
end
return f()
`)
	local := stmts[0].(*ast.LocalAssignStmt)
	bindings := bind.BindChunk(stmts, bind.Options{})
	fTarget := mustBoundLocalAt(t, bindings, local, 0)
	origin := onlyFunctionOrigin(t, bindings)
	if !origin.HasTargetSymbol || origin.TargetSymbol != fTarget {
		t.Fatalf("function origin target = %d/%v, want local symbol %d", origin.TargetSymbol, origin.HasTargetSymbol, fTarget)
	}

	result, err := RunBoundChunk(stmts, bindings, Config{
		Check: body.Config{
			Registry:        reg,
			ExpressionValue: fixedExpressionValue(want),
		},
	})
	if err != nil {
		t.Fatalf("RunBoundChunk: %v", err)
	}

	targetKey, ok := result.TargetKey(fTarget)
	if !ok {
		t.Fatalf("TargetKey(%d) missing", fTarget)
	}
	if wantKey := summary.DefaultSummaryKey(ref.FromSymbol(origin.Symbol)); targetKey != wantKey {
		t.Fatalf("TargetKey(%d) = %#v, want %#v", fTarget, targetKey, wantKey)
	}
	assertSummaryReturn(t, reg, result.Snapshot(), result.RootKey(), want)
	assertSummaryReturn(t, reg, result.Snapshot(), targetKey, want)
}

func TestRunChunkReexportsChainedWrapperNormalReturnParam(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local requireValue = function(x: string?)
	assert(x)
end
local requireAgain = function(x: string?)
	requireValue(x)
end
`)
	firstLocal := stmts[0].(*ast.LocalAssignStmt)
	secondLocal := stmts[1].(*ast.LocalAssignStmt)
	bindings := bind.BindChunk(stmts, bind.Options{})
	requireValue := mustBoundLocalAt(t, bindings, firstLocal, 0)
	requireAgain := mustBoundLocalAt(t, bindings, secondLocal, 0)

	result, err := RunBoundChunk(stmts, bindings, Config{
		Check: body.Config{Registry: reg},
	})
	if err != nil {
		t.Fatalf("RunBoundChunk: %v", err)
	}

	valueKey, ok := result.TargetKey(requireValue)
	if !ok {
		t.Fatalf("TargetKey(requireValue) missing")
	}
	againKey, ok := result.TargetKey(requireAgain)
	if !ok {
		t.Fatalf("TargetKey(requireAgain) missing")
	}
	assertSummaryNormalReturnParam(t, reg, result.Snapshot(), valueKey, 0, presence.Present(), runtimekind.Singleton(runtimekind.String))
	assertSummaryNormalReturnParam(t, reg, result.Snapshot(), againKey, 0, presence.Present(), runtimekind.Singleton(runtimekind.String))
}

func TestRunChunkUsesExactConfiguredRootKey(t *testing.T) {
	reg := standard.Registry()
	want := product.Top()
	stmts := parseChunk(t, "return 1")
	rootKey := summary.SummaryKey{
		Ref:   ref.FuncRef{Kind: ref.KindRoot, ID: 42},
		Entry: summary.EntryKey{Values: 1, Facts: 2, References: 3},
	}

	result, err := RunChunk(stmts, Config{
		Check: body.Config{
			Registry:        reg,
			ExpressionValue: fixedExpressionValue(want),
		},
		RootKey: rootKey,
	})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}

	assertSummaryReturn(t, reg, result.Snapshot(), rootKey, want)
	if got, ok := result.Snapshot().Read(summary.DefaultSummaryKey(ref.Root())); ok {
		t.Fatalf("default root summary = %#v, want missing exact key", got)
	}
}

func fixedExpressionValue(value product.Value) func(cfg.Point, factflow.ExprRef, factflow.ValueSource, state.State) (product.Value, bool) {
	return func(cfg.Point, factflow.ExprRef, factflow.ValueSource, state.State) (product.Value, bool) {
		return value, true
	}
}

func parseChunk(t *testing.T, src string) []ast.Stmt {
	t.Helper()
	stmts, err := parse.ParseString(src, "fixpoint_program_test.lua")
	if err != nil {
		t.Fatalf("ParseString(%q): %v", src, err)
	}
	return stmts
}

func onlyFunctionOrigin(t *testing.T, bindings *bind.Result) bind.FunctionOrigin {
	t.Helper()
	origins := bindings.FunctionOrigins()
	if len(origins) != 1 {
		t.Fatalf("FunctionOrigins length = %d, want 1: %#v", len(origins), origins)
	}
	return origins[0]
}

func mustBoundLocalAt(t *testing.T, bindings *bind.Result, stmt *ast.LocalAssignStmt, index int) symbol.ID {
	t.Helper()
	locals := bindings.LocalSymbols(stmt)
	if index < 0 || index >= len(locals) {
		t.Fatalf("bound local index %d out of range for %d locals", index, len(locals))
	}
	if locals[index] == 0 {
		t.Fatalf("bound local symbol at %d is zero", index)
	}
	return locals[index]
}

func assertSummaryReturn(t *testing.T, reg *axis.Registry, snapshot summary.Snapshot, key summary.SummaryKey, want product.Value) {
	t.Helper()
	got, ok := snapshot.Read(key)
	if !ok {
		t.Fatalf("summary %s missing", key.Ref)
	}
	if len(got.Returns) != 1 {
		t.Fatalf("summary %s returns = %d, want 1: %#v", key.Ref, len(got.Returns), got)
	}
	if !product.Equal(reg, got.Returns[0], want) {
		t.Fatalf("summary %s return = %v, want %v", key.Ref, got.Returns[0], want)
	}
}

func assertSummaryNormalReturnParam(
	t *testing.T,
	reg *axis.Registry,
	snapshot summary.Snapshot,
	key summary.SummaryKey,
	index int,
	wantPresence presence.Value,
	wantKind runtimekind.Value,
) {
	t.Helper()
	got, ok := snapshot.Read(key)
	if !ok {
		t.Fatalf("summary %s missing", key.Ref)
	}
	if len(got.NormalReturnParams) <= index {
		t.Fatalf("summary %s normal return params = %d, want index %d: %#v", key.Ref, len(got.NormalReturnParams), index, got)
	}
	value := got.NormalReturnParams[index]
	if gotPresence := product.PresenceOf(value); !presence.Equal(gotPresence, wantPresence) {
		t.Fatalf("summary %s param %d presence = %s, want %s", key.Ref, index, gotPresence, wantPresence)
	}
	if gotKind := product.Get(reg, value, runtimekind.Key); !runtimekind.Equal(gotKind, wantKind) {
		t.Fatalf("summary %s param %d runtime kind = %s, want %s", key.Ref, index, gotKind, wantKind)
	}
}
