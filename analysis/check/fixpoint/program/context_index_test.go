package program

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestProgramKeysUpsertCallContextOwnsMergeAndStaleRefRecovery(t *testing.T) {
	fn := &ast.FunctionExpr{}
	owner := summary.SummaryKey{}
	base := summary.SummaryKey{Entry: summary.EntryKey{Facts: 1}}
	ref := callContextRef{owner: canonicalContextOwner(owner), expr: factflow.ExprRef(42)}
	keys := programKeys{contexts: newContextIndex(10)}

	first, ok := keys.upsertCallContext(nil, ref, base, fn, state.State{}, nil, nil)
	if !ok {
		t.Fatal("upsertCallContext returned false for new context")
	}
	if keys.contexts.Len() != 1 {
		t.Fatalf("contexts = %d, want 1", keys.contexts.Len())
	}
	if got := first.Entry.Facts; got != 10 {
		t.Fatalf("context facts digest = %d, want 10", got)
	}

	second, ok := keys.upsertCallContext(nil, ref, base, fn, state.State{}, nil, nil)
	if !ok || second != first {
		t.Fatalf("existing context key = (%v, %v), want (%v, true)", second, ok, first)
	}
	if keys.contexts.Len() != 1 {
		t.Fatalf("existing upsert appended context; contexts = %d", keys.contexts.Len())
	}

	keys.contexts.callKeys[ref] = summary.SummaryKey{Entry: summary.EntryKey{Facts: 99}}
	third, ok := keys.upsertCallContext(nil, ref, base, fn, state.State{}, nil, nil)
	if !ok {
		t.Fatal("upsertCallContext did not recover stale ref")
	}
	if keys.contexts.Len() != 2 {
		t.Fatalf("stale ref recovery contexts = %d, want 2", keys.contexts.Len())
	}
	if key, ok := keys.contexts.CallContextKey(owner, ref.expr); !ok || key != third {
		t.Fatal("stale ref recovery did not replace index entry")
	}
}

func TestProgramKeysUpsertFunctionExpressionContextOwnsIndexMaps(t *testing.T) {
	fn := &ast.FunctionExpr{}
	owner := summary.SummaryKey{Entry: summary.EntryKey{Facts: 7}}
	base := summary.SummaryKey{Entry: summary.EntryKey{Facts: 1}}
	ref := functionExpressionRef{owner: canonicalContextOwner(owner), expr: factflow.ExprRef(9)}
	keys := programKeys{
		functionKeys:  map[symbol.ID]summary.SummaryKey{symbol.ID(3): base},
		functionByKey: make(map[summary.SummaryKey]*ast.FunctionExpr),
		contexts: contextIndex{
			functionExpressionKeys: map[functionExpressionRef]summary.SummaryKey{ref: {Entry: summary.EntryKey{Facts: 99}}},
			nextID:                 20,
		},
	}

	first, ok := keys.upsertFunctionExpressionContext(nil, owner, factflow.ExprRef(9), symbol.ID(3), fn, state.State{}, nil, nil)
	if !ok {
		t.Fatal("upsertFunctionExpressionContext did not recover stale ref")
	}
	if keys.contexts.Len() != 1 {
		t.Fatalf("contexts = %d, want 1", keys.contexts.Len())
	}
	if got := keys.contexts.FunctionExpressionKeysForOwner(owner)[ref.expr]; got != first {
		t.Fatal("function expression index was not replaced")
	}
	if keys.functionByKey[first] != fn {
		t.Fatal("function expression context did not bind functionByKey")
	}

	second, ok := keys.upsertFunctionExpressionContext(nil, owner, factflow.ExprRef(9), symbol.ID(3), fn, state.State{}, nil, nil)
	if !ok || second != first {
		t.Fatalf("existing function-expression context key = (%v, %v), want (%v, true)", second, ok, first)
	}
	if keys.contexts.Len() != 1 {
		t.Fatalf("existing upsert appended context; contexts = %d", keys.contexts.Len())
	}
}

func TestProgramKeysUpsertFunctionExpressionContextUpgradesFunctionType(t *testing.T) {
	fn := &ast.FunctionExpr{}
	owner := summary.SummaryKey{Entry: summary.EntryKey{Facts: 7}}
	base := summary.SummaryKey{Entry: summary.EntryKey{Facts: 1}}
	keys := programKeys{
		functionKeys:  map[symbol.ID]summary.SummaryKey{symbol.ID(3): base},
		functionByKey: make(map[summary.SummaryKey]*ast.FunctionExpr),
		functionTypes: make(map[summary.SummaryKey]*typ.Function),
		contexts:      newContextIndex(20),
	}

	firstType := typ.Func().Param("value", typ.Any).Build()
	key, ok := keys.upsertFunctionExpressionContext(nil, owner, factflow.ExprRef(9), symbol.ID(3), fn, state.State{}, nil, firstType)
	if !ok {
		t.Fatal("initial upsert returned unchanged")
	}

	upgradedType := typ.Func().Param("value", typ.String).Returns(typ.String).Build()
	gotKey, changed := keys.upsertFunctionExpressionContext(nil, owner, factflow.ExprRef(9), symbol.ID(3), fn, state.State{}, nil, upgradedType)
	if !changed || gotKey != key {
		t.Fatalf("upgrade = (%v, %v), want (%v, true)", gotKey, changed, key)
	}
	if got := keys.functionTypes[key]; !typ.SameNodeOrAcyclicEqual(got, upgradedType) {
		t.Fatalf("functionTypes[%v] = %v, want %v", key, got, upgradedType)
	}
	if keys.contexts.Len() != 1 {
		t.Fatalf("upgrade appended context; contexts = %d, want 1", keys.contexts.Len())
	}
}

func TestContextIndexEntriesReturnsSnapshot(t *testing.T) {
	fn := &ast.FunctionExpr{}
	base := summary.SummaryKey{Entry: summary.EntryKey{Facts: 1}}
	keys := programKeys{contexts: newContextIndex(30)}
	ref := callContextRef{owner: canonicalContextOwner(summary.SummaryKey{}), expr: factflow.ExprRef(4)}

	key, ok := keys.upsertCallContext(nil, ref, base, fn, state.State{}, nil, nil)
	if !ok {
		t.Fatal("upsertCallContext returned false for new context")
	}
	entries := keys.contexts.Entries()
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	entries[0].key = summary.SummaryKey{Entry: summary.EntryKey{Facts: 99}}
	if got := keys.contexts.Entry(0).key; got != key {
		t.Fatal("Entries returned mutable context storage")
	}
	if got, ok := keys.contexts.CallContextKey(summary.SummaryKey{}, ref.expr); !ok || got != key {
		t.Fatal("context key lookup changed after mutating snapshot")
	}
}

func TestContextIndexHasFunctionExpressionRequiresLiveContext(t *testing.T) {
	owner := summary.SummaryKey{Entry: summary.EntryKey{Facts: 7}}
	ref := functionExpressionRef{owner: canonicalContextOwner(owner), expr: factflow.ExprRef(9)}
	keys := programKeys{
		contexts: contextIndex{
			functionExpressionKeys: map[functionExpressionRef]summary.SummaryKey{
				ref: {Entry: summary.EntryKey{Facts: 99}},
			},
		},
	}

	if keys.contexts.HasFunctionExpression(owner, ref.expr) {
		t.Fatal("stale function expression ref reported as live")
	}
}

func TestProgramKeysRefreshContextForKeyJoinsExistingEntryState(t *testing.T) {
	reg := standard.Registry()
	fn := &ast.FunctionExpr{}
	contextKey := summary.SummaryKey{Entry: summary.EntryKey{Facts: 50}}
	existingSlot := statekey.SymbolValue(symbol.ID(10))
	refreshedSlot := statekey.SymbolValue(symbol.ID(11))
	staleSlot := statekey.SymbolValue(symbol.ID(12))
	existing := state.State{}.WriteValue(reg, existingSlot, typevalue.FromType(reg, typ.String))
	existing = existing.WriteValue(reg, staleSlot, typevalue.FromType(reg, typ.Any))
	refreshed := state.State{}.
		WriteValue(reg, refreshedSlot, typevalue.FromType(reg, typ.Number)).
		WriteValue(reg, staleSlot, typevalue.FromType(reg, typ.String))
	keys := programKeys{contexts: newContextIndex(60)}
	keys.contexts.appendContext(fn, contextKey, existing, nil)

	if !keys.refreshContextForKey(reg, contextKey, fn, nil, refreshed) {
		t.Fatal("refreshContextForKey returned unchanged for new entry facts")
	}
	if keys.contexts.Len() != 1 {
		t.Fatalf("refresh appended contexts = %d, want 1", keys.contexts.Len())
	}
	entry := keys.contexts.Entry(0)
	if got := entry.entryState.ReadValue(reg, existingSlot); product.Equal(reg, got, product.Bottom(reg)) {
		t.Fatal("refresh dropped existing context entry facts")
	}
	if got := entry.entryState.ReadValue(reg, refreshedSlot); product.Equal(reg, got, product.Bottom(reg)) {
		t.Fatal("refresh did not add refreshed context entry facts")
	}
	staleValue := entry.entryState.ReadValue(reg, staleSlot)
	staleType, staleOK := typevalue.TypeOf(reg, staleValue)
	if !staleOK || staleType != typ.String {
		t.Fatalf("refresh did not make recomputed entry slot authoritative: got %v/%v, want string", staleType, staleOK)
	}
	if keys.refreshContextForKey(reg, contextKey, fn, nil, refreshed) {
		t.Fatal("refreshContextForKey reported changed after fixed point")
	}
}

func TestContextIndexTransformEntriesPreservesKeys(t *testing.T) {
	fn := &ast.FunctionExpr{}
	base := summary.SummaryKey{Entry: summary.EntryKey{Facts: 1}}
	keys := programKeys{contexts: newContextIndex(40)}
	ref := callContextRef{owner: canonicalContextOwner(summary.SummaryKey{}), expr: factflow.ExprRef(5)}

	key, ok := keys.upsertCallContext(nil, ref, base, fn, state.State{}, nil, nil)
	if !ok {
		t.Fatal("upsertCallContext returned false for new context")
	}
	keys.contexts.TransformEntries(func(entry keyedFunction) keyedFunction {
		entry.key = summary.SummaryKey{Entry: summary.EntryKey{Facts: 99}}
		return entry
	})

	if got := keys.contexts.Entry(0).key; got != key {
		t.Fatal("context transform changed context identity")
	}
	if got, ok := keys.contexts.CallContextKey(summary.SummaryKey{}, ref.expr); !ok || got != key {
		t.Fatal("context lookup changed after identity-changing transform")
	}
}
