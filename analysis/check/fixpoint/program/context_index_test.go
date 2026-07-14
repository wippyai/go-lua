package program

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/ref"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func mustUpsertCallContext(t *testing.T, keys *programKeys, reg *axis.Registry, ref callContextRef, base summary.SummaryKey, fn *ast.FunctionExpr, entry state.State, entryKeys *keyspace.KeySpace, fnType *typ.Function, bodyDigest ...uint64) (summary.SummaryKey, bool) {
	t.Helper()
	key, changed, err := keys.upsertCallContext(reg, ref, base, fn, entry, entryKeys, fnType, bodyDigest...)
	if err != nil {
		t.Fatalf("upsert call context: %v", err)
	}
	return key, changed
}

func mustUpsertFunctionExpressionContext(t *testing.T, keys *programKeys, reg *axis.Registry, owner summary.SummaryKey, expr factflow.ExprRef, callbackSymbol symbol.ID, fn *ast.FunctionExpr, entry state.State, entryKeys *keyspace.KeySpace, fnType *typ.Function, bodyDigest ...uint64) (summary.SummaryKey, bool) {
	t.Helper()
	key, changed, err := keys.upsertFunctionExpressionContext(reg, owner, expr, callbackSymbol, fn, entry, entryKeys, fnType, bodyDigest...)
	if err != nil {
		t.Fatalf("upsert function-expression context: %v", err)
	}
	return key, changed
}

func mustRefreshContextForKey(t *testing.T, keys *programKeys, reg *axis.Registry, key summary.SummaryKey, fn *ast.FunctionExpr, entryKeys *keyspace.KeySpace, entry state.State) bool {
	t.Helper()
	changed, err := keys.refreshContextForKey(reg, key, fn, entryKeys, entry)
	if err != nil {
		t.Fatalf("refresh context: %v", err)
	}
	return changed
}

func TestProgramKeysUpsertCallContextOwnsMergeAndStaleRefRecovery(t *testing.T) {
	fn := &ast.FunctionExpr{}
	owner := summary.SummaryKey{}
	base := summary.SummaryKey{Entry: summary.EntryKey{Facts: 1}}
	ref := callContextRef{owner: canonicalContextOwner(owner), expr: factflow.ExprRef(42)}
	keys := programKeys{contexts: newContextIndex()}

	first, ok := mustUpsertCallContext(t, &keys, nil, ref, base, fn, state.State{}, nil, nil)
	if !ok {
		t.Fatal("upsertCallContext returned false for new context")
	}
	if keys.contexts.Len() != 1 {
		t.Fatalf("contexts = %d, want 1", keys.contexts.Len())
	}
	if got := first.Entry; got != (summary.EntryKey{}) {
		t.Fatalf("empty semantic entry key = %#v, want zero content digests", got)
	}

	second, changed := mustUpsertCallContext(t, &keys, nil, ref, base, fn, state.State{}, nil, nil)
	if !changed || second != first {
		t.Fatalf("existing context key = (%v, changed:%v), want (%v, true)", second, changed, first)
	}
	if keys.contexts.Len() != 1 {
		t.Fatalf("existing upsert appended context; contexts = %d", keys.contexts.Len())
	}

	keys.contexts.callKeys[ref] = summary.SummaryKey{Entry: summary.EntryKey{Facts: 99}}
	third, ok := mustUpsertCallContext(t, &keys, nil, ref, base, fn, state.State{}, nil, nil)
	if !ok {
		t.Fatal("upsertCallContext did not recover stale ref")
	}
	if keys.contexts.Len() != 1 {
		t.Fatalf("stale ref recovery semantic variants = %d, want 1", keys.contexts.Len())
	}
	if key, ok := keys.contexts.CallContextKey(owner, ref.expr); !ok || key != third {
		t.Fatal("stale ref recovery did not replace index entry")
	}
}

func TestContextKeysAreStableAcrossDiscoveryOrder(t *testing.T) {
	fn := &ast.FunctionExpr{}
	base := summary.SummaryKey{Entry: summary.EntryKey{Facts: 1}}
	firstRef := callContextRef{owner: summary.SummaryKey{Entry: summary.EntryKey{Values: 7}}, expr: factflow.ExprRef(3)}
	secondRef := callContextRef{owner: summary.SummaryKey{Entry: summary.EntryKey{Values: 9}}, expr: factflow.ExprRef(1)}

	forward := programKeys{contexts: newContextIndex()}
	forwardFirst, changed := mustUpsertCallContext(t, &forward, nil, firstRef, base, fn, state.State{}, nil, nil)
	if !changed {
		t.Fatal("forward first context unchanged")
	}
	forwardSecond, changed := mustUpsertCallContext(t, &forward, nil, secondRef, base, fn, state.State{}, nil, nil)
	if !changed {
		t.Fatal("forward second context unchanged")
	}

	reverse := programKeys{contexts: newContextIndex()}
	reverseSecond, changed := mustUpsertCallContext(t, &reverse, nil, secondRef, base, fn, state.State{}, nil, nil)
	if !changed {
		t.Fatal("reverse second context unchanged")
	}
	reverseFirst, changed := mustUpsertCallContext(t, &reverse, nil, firstRef, base, fn, state.State{}, nil, nil)
	if !changed {
		t.Fatal("reverse first context unchanged")
	}

	if forwardFirst != reverseFirst || forwardSecond != reverseSecond {
		t.Fatalf("context keys depend on discovery order\nforward: %v, %v\nreverse: %v, %v", forwardFirst, forwardSecond, reverseFirst, reverseSecond)
	}
}

func TestDomainEqualCallSitesShareOneSemanticVariant(t *testing.T) {
	reg := standard.Registry()
	fn := &ast.FunctionExpr{}
	base := summary.DefaultSummaryKey(ref.Root())
	// The slot/value pair is deliberately rebuilt for each site. The states are
	// Domain-equal despite being distinct Go values.
	entry := state.State{}.WriteValue(reg, statekey.SymbolValue(77), typevalue.FromType(reg, typ.String))
	keys := programKeys{contexts: newContextIndex()}
	left, changed := mustUpsertCallContext(t, &keys, reg, callContextRef{expr: 10}, base, fn, entry, nil, nil)
	if !changed {
		t.Fatal("first semantic context was not created")
	}
	rightEntry := state.State{}.WriteValue(reg, statekey.SymbolValue(77), typevalue.FromType(reg, typ.String))
	right, changed := mustUpsertCallContext(t, &keys, reg, callContextRef{expr: 11}, base, fn, rightEntry, nil, nil)
	if !changed {
		t.Fatal("second call-site routing was not recorded")
	}
	if left != right {
		t.Fatalf("Domain-equal call sites got variants %v and %v, want one", left, right)
	}
	if got := keys.contexts.Len(); got != 1 {
		t.Fatalf("semantic variants = %d, want 1", got)
	}
	if got := keys.contexts.CallRefCount(); got != 2 {
		t.Fatalf("site provenance entries = %d, want 2", got)
	}
}

func TestMergeContextEntryRekeysExactStateIntoCanonicalKeySpace(t *testing.T) {
	reg := standard.Registry()
	from, to := keyspace.New(), keyspace.New()
	path := pathdom.NewPath(symbol.ID(71), "arg").Field("name")
	fromKey := from.FromPath(path)
	toKey, ok := to.ImportKey(from, fromKey)
	if !ok {
		t.Fatal("failed to import test key")
	}
	want := typevalue.FromType(reg, typ.String)
	incoming := state.Domain(reg).Bottom().WriteLocalPathStaticMember(fromKey, want)
	context := keyedFunction{entryState: state.Domain(reg).Bottom(), entryKeys: to, hasEntryState: true}

	changed, err := mergeContextEntry(reg, &context, from, incoming)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("exact rekeyed entry was not merged")
	}
	got, ok := context.entryState.ReadLocalPathStaticMember(toKey)
	if !ok || !product.Equal(reg, got, want) {
		t.Fatalf("rekeyed member = (%#v, %v), want exact string evidence", got, ok)
	}
	if _, ok := context.entryState.ReadLocalPathStaticMember(fromKey); ok {
		t.Fatal("source-owned key survived canonical context merge")
	}
}

func TestMergeContextEntryRejectsForeignKeySpaceTransactionally(t *testing.T) {
	reg := standard.Registry()
	claimed, canonical, foreign := keyspace.New(), keyspace.New(), keyspace.New()
	foreignKey := foreign.FromPath(pathdom.NewPath(symbol.ID(72), "foreign").Field("name"))
	incoming := state.Domain(reg).Bottom().WriteLocalPathStaticMember(foreignKey, typevalue.FromType(reg, typ.String))
	stableSlot := statekey.SymbolValue(symbol.ID(73))
	stable := state.State{}.WriteValue(reg, stableSlot, typevalue.FromType(reg, typ.Number))
	context := keyedFunction{entryState: stable, entryKeys: canonical, hasEntryState: true}

	changed, err := mergeContextEntry(reg, &context, claimed, incoming)
	if err == nil {
		t.Fatal("foreign entry keyspace was accepted")
	}
	if changed {
		t.Fatal("failed rekey reported a context mutation")
	}
	if !state.Domain(reg).Equal(context.entryState, stable) || context.entryKeys != canonical {
		t.Fatal("failed rekey partially mutated the existing context")
	}
}

func TestMergeContextEntryRejectsForeignStoredStateBeforeMutation(t *testing.T) {
	reg := standard.Registry()
	canonical, incomingKeys, foreign := keyspace.New(), keyspace.New(), keyspace.New()
	foreignKey := foreign.FromPath(pathdom.NewPath(symbol.ID(78), "stored").Field("member"))
	stored := state.Domain(reg).Bottom().WriteLocalPathStaticMember(foreignKey, typevalue.FromType(reg, typ.String))
	context := keyedFunction{entryState: stored, entryKeys: canonical, hasEntryState: true}
	incoming := state.State{}.WriteValue(reg, statekey.SymbolValue(symbol.ID(79)), typevalue.FromType(reg, typ.Number))

	changed, err := mergeContextEntry(reg, &context, incomingKeys, incoming)
	if err == nil {
		t.Fatal("foreign stored context state was accepted")
	}
	if changed {
		t.Fatal("stored-state validation failure reported a mutation")
	}
	if context.entryKeys != canonical || !state.Domain(reg).Equal(context.entryState, stored) {
		t.Fatal("stored-state validation failure partially mutated context state or keyspace")
	}
}

func TestMergeContextEntryDoesNotPublishCanonicalKeySpaceBeforeIncomingValidation(t *testing.T) {
	reg := standard.Registry()
	claimed, foreign := keyspace.New(), keyspace.New()
	stable := state.State{}.WriteValue(reg, statekey.SymbolValue(symbol.ID(80)), typevalue.FromType(reg, typ.Number))
	context := keyedFunction{entryState: stable, entryKeys: nil, hasEntryState: true}
	foreignKey := foreign.FromPath(pathdom.NewPath(symbol.ID(81), "incoming").Field("member"))
	incoming := state.Domain(reg).Bottom().WriteLocalPathStaticMember(foreignKey, typevalue.FromType(reg, typ.String))

	changed, err := mergeContextEntry(reg, &context, claimed, incoming)
	if err == nil {
		t.Fatal("foreign incoming state was accepted")
	}
	if changed {
		t.Fatal("incoming validation failure reported a mutation")
	}
	if context.entryKeys != nil || !state.Domain(reg).Equal(context.entryState, stable) {
		t.Fatal("incoming validation failure partially published canonical metadata")
	}
}

func TestSemanticContextReuseRejectsUnrekeyableStoredCandidate(t *testing.T) {
	reg := standard.Registry()
	fn := &ast.FunctionExpr{}
	base := summary.DefaultSummaryKey(ref.Root())
	bodyDigest := uint64(0xabc)
	entry := state.State{}.WriteValue(reg, statekey.SymbolValue(symbol.ID(74)), typevalue.FromType(reg, typ.String))
	valid := keyspace.New()
	invalidValue := *keyspace.New()
	invalid := &invalidValue
	if invalid.Valid() {
		t.Fatal("shallow keyspace copy unexpectedly retained authority")
	}
	keys := programKeys{contexts: newContextIndex()}
	keys.contexts.appendSemanticContext(reg, base, bodyDigest, fn, entry, invalid)

	key, changed, err := keys.upsertCallContext(
		reg,
		callContextRef{expr: factflow.ExprRef(75)},
		base,
		fn,
		entry,
		valid,
		nil,
		bodyDigest,
	)
	if err == nil {
		t.Fatal("unrekeyable stored candidate was treated as an optional mismatch")
	}
	if changed || key != (summary.SummaryKey{}) {
		t.Fatalf("failed reuse = (%v, changed:%v), want no routing publication", key, changed)
	}
	if got := keys.contexts.Len(); got != 1 {
		t.Fatalf("semantic variants = %d, want unchanged candidate inventory", got)
	}
	if _, ok := keys.contexts.CallContextKey(summary.SummaryKey{}, factflow.ExprRef(75)); ok {
		t.Fatal("failed candidate reuse published a call route")
	}
}

func TestUpsertCallContextRejectsForeignIncomingEntryBeforePublication(t *testing.T) {
	reg := standard.Registry()
	claimed, foreign := keyspace.New(), keyspace.New()
	foreignKey := foreign.FromPath(pathdom.NewPath(symbol.ID(76), "foreign").Field("member"))
	entry := state.Domain(reg).Bottom().WriteLocalPathStaticMember(foreignKey, typevalue.FromType(reg, typ.String))
	keys := programKeys{contexts: newContextIndex()}
	callRef := callContextRef{expr: factflow.ExprRef(77)}

	key, changed, err := keys.upsertCallContext(
		reg,
		callRef,
		summary.DefaultSummaryKey(ref.Root()),
		&ast.FunctionExpr{},
		entry,
		claimed,
		nil,
	)
	if err == nil {
		t.Fatal("foreign incoming entry was accepted")
	}
	if changed || key != (summary.SummaryKey{}) || keys.contexts.Len() != 0 {
		t.Fatalf("foreign incoming entry partially published: key=%v changed=%v contexts=%d", key, changed, keys.contexts.Len())
	}
	if _, ok := keys.contexts.CallContextKey(summary.SummaryKey{}, callRef.expr); ok {
		t.Fatal("foreign incoming entry published a call route")
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
		},
	}

	first, ok := mustUpsertFunctionExpressionContext(t, &keys, nil, owner, factflow.ExprRef(9), symbol.ID(3), fn, state.State{}, nil, nil)
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

	second, changed := mustUpsertFunctionExpressionContext(t, &keys, nil, owner, factflow.ExprRef(9), symbol.ID(3), fn, state.State{}, nil, nil)
	if !changed || second != first {
		t.Fatalf("existing function-expression context key = (%v, changed:%v), want (%v, true)", second, changed, first)
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
		contexts:      newContextIndex(),
	}

	firstType := typ.Func().Param("value", typ.Any).Build()
	key, ok := mustUpsertFunctionExpressionContext(t, &keys, nil, owner, factflow.ExprRef(9), symbol.ID(3), fn, state.State{}, nil, firstType)
	if !ok {
		t.Fatal("initial upsert returned unchanged")
	}

	upgradedType := typ.Func().Param("value", typ.String).Returns(typ.String).Build()
	gotKey, changed := mustUpsertFunctionExpressionContext(t, &keys, nil, owner, factflow.ExprRef(9), symbol.ID(3), fn, state.State{}, nil, upgradedType)
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
	keys := programKeys{contexts: newContextIndex()}
	ref := callContextRef{owner: canonicalContextOwner(summary.SummaryKey{}), expr: factflow.ExprRef(4)}

	key, ok := mustUpsertCallContext(t, &keys, nil, ref, base, fn, state.State{}, nil, nil)
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
	keys := programKeys{contexts: newContextIndex()}
	keys.contexts.appendContext(fn, contextKey, existing, nil)

	if !mustRefreshContextForKey(t, &keys, reg, contextKey, fn, nil, refreshed) {
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
	if mustRefreshContextForKey(t, &keys, reg, contextKey, fn, nil, refreshed) {
		t.Fatal("refreshContextForKey reported changed after fixed point")
	}
}

func TestContextIndexTransformEntriesPreservesKeys(t *testing.T) {
	fn := &ast.FunctionExpr{}
	base := summary.SummaryKey{Entry: summary.EntryKey{Facts: 1}}
	keys := programKeys{contexts: newContextIndex()}
	ref := callContextRef{owner: canonicalContextOwner(summary.SummaryKey{}), expr: factflow.ExprRef(5)}

	key, ok := mustUpsertCallContext(t, &keys, nil, ref, base, fn, state.State{}, nil, nil)
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
