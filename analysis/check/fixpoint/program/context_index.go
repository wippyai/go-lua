package program

import (
	"fmt"
	"hash/fnv"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

type contextIndex struct {
	entries                []keyedFunction
	byKey                  map[summary.SummaryKey]int
	callKeys               map[callContextRef]summary.SummaryKey
	functionExpressionKeys map[functionExpressionRef]summary.SummaryKey
}

func newContextIndex() contextIndex {
	return contextIndex{
		byKey: make(map[summary.SummaryKey]int),
	}
}

func (idx *contextIndex) Len() int {
	if idx == nil {
		return 0
	}
	return len(idx.entries)
}

func (idx *contextIndex) CallRefCount() int {
	if idx == nil {
		return 0
	}
	return len(idx.callKeys)
}

func (idx *contextIndex) Entry(i int) keyedFunction {
	if idx == nil || i < 0 || i >= len(idx.entries) {
		return keyedFunction{}
	}
	return idx.entries[i]
}

func (idx *contextIndex) Entries() []keyedFunction {
	if idx == nil {
		return nil
	}
	out := make([]keyedFunction, len(idx.entries))
	copy(out, idx.entries)
	return out
}

func (idx *contextIndex) TransformEntries(transform func(keyedFunction) keyedFunction) {
	if idx == nil || transform == nil {
		return
	}
	idx.ensureKeyIndex()
	for i := range idx.entries {
		oldKey := idx.entries[i].key
		next := transform(idx.entries[i])
		next.key = oldKey
		idx.entries[i] = next
	}
}

func (idx *contextIndex) ForEach(fn func(keyedFunction)) {
	if idx == nil || fn == nil {
		return
	}
	for _, entry := range idx.entries {
		fn(entry)
	}
}

func (idx *contextIndex) ensureKeyIndex() {
	if idx == nil {
		return
	}
	if idx.byKey != nil && len(idx.byKey) >= len(idx.entries) {
		return
	}
	idx.byKey = make(map[summary.SummaryKey]int, len(idx.entries))
	for i := range idx.entries {
		idx.byKey[idx.entries[i].key] = i
	}
}

func (idx *contextIndex) contextByKey(key summary.SummaryKey) (*keyedFunction, bool) {
	if idx == nil {
		return nil, false
	}
	idx.ensureKeyIndex()
	i, ok := idx.byKey[key]
	if !ok || i < 0 || i >= len(idx.entries) || idx.entries[i].key != key {
		return nil, false
	}
	return &idx.entries[i], true
}

func (idx *contextIndex) hasContextKey(key summary.SummaryKey) bool {
	_, ok := idx.contextByKey(key)
	return ok
}

func (idx *contextIndex) mergeContextForKey(reg *axis.Registry, key summary.SummaryKey, fn *ast.FunctionExpr, entryKeys *keyspace.KeySpace, entry state.State) bool {
	context, ok := idx.contextByKey(key)
	if !ok || context.funcExpr != fn {
		return false
	}
	return mergeContextEntry(reg, context, entryKeys, entry)
}

func mergeContextEntry(reg *axis.Registry, context *keyedFunction, entryKeys *keyspace.KeySpace, entry state.State) bool {
	if context == nil {
		return false
	}
	if !context.hasEntryState || reg == nil {
		context.entryState = entry
		context.entryKeys = entryKeys
		context.hasEntryState = true
		return true
	}
	current := context.entryState.RekeyPathEvidence(context.entryKeys, entryKeys)
	joined := state.Domain(reg).Join(current, entry)
	joined = joined.RefreshValueSlotsFrom(reg, entry)
	if state.Domain(reg).Equal(current, joined) {
		return false
	}
	context.entryState = joined
	context.entryKeys = entryKeys
	context.hasEntryState = true
	return true
}

type contextKeyIdentity struct {
	kind  string
	owner summary.SummaryKey
	expr  factflow.ExprRef
}

// nextContextKey derives the context entry dimension from source-stable
// identity rather than discovery order. The callee's base key supplies the
// function reference; owner and expression reference identify the call or
// function-expression site that caused specialization.
func (idx *contextIndex) nextContextKey(baseKey summary.SummaryKey, identity contextKeyIdentity) summary.SummaryKey {
	for collision := uint64(0); ; collision++ {
		contextKey := baseKey
		contextKey.Entry.Facts = stableContextKeyDigest(baseKey, identity, collision)
		if contextKey.Entry.Facts == 0 {
			continue
		}
		if !idx.hasContextKey(contextKey) {
			return contextKey
		}
	}
}

func stableContextKeyDigest(baseKey summary.SummaryKey, identity contextKeyIdentity, collision uint64) summary.Digest {
	h := fnv.New64a()
	_, _ = fmt.Fprint(h, "call-context-key-v1:")
	writeSummaryKeyDigest(h, baseKey)
	_, _ = fmt.Fprint(h, "kind:", identity.kind, ";owner:")
	writeSummaryKeyDigest(h, identity.owner)
	_, _ = fmt.Fprint(h, "expr:", uint32(identity.expr), ";collision:", collision, ";")
	return summary.Digest(h.Sum64())
}

func (idx *contextIndex) appendContext(fn *ast.FunctionExpr, contextKey summary.SummaryKey, entry state.State, entryKeys *keyspace.KeySpace) {
	idx.ensureKeyIndex()
	idx.entries = append(idx.entries, keyedFunction{
		funcExpr:      fn,
		key:           contextKey,
		entryState:    entry,
		entryKeys:     entryKeys,
		hasEntryState: true,
	})
	idx.byKey[contextKey] = len(idx.entries) - 1
}

func (idx *contextIndex) upsertCallContext(
	reg *axis.Registry,
	ref callContextRef,
	baseKey summary.SummaryKey,
	fn *ast.FunctionExpr,
	entry state.State,
	entryKeys *keyspace.KeySpace,
) (summary.SummaryKey, bool, bool) {
	if idx == nil || fn == nil {
		return summary.SummaryKey{}, false, false
	}
	if existing, seen := idx.callKeys[ref]; seen {
		if idx.mergeContextForKey(reg, existing, fn, entryKeys, entry) {
			return existing, true, false
		}
		if _, ok := idx.contextByKey(existing); ok {
			return summary.SummaryKey{}, false, false
		}
	}
	contextKey := idx.nextContextKey(baseKey, contextKeyIdentity{kind: "call", owner: ref.owner, expr: ref.expr})
	idx.appendContext(fn, contextKey, entry, entryKeys)
	if idx.callKeys == nil {
		idx.callKeys = make(map[callContextRef]summary.SummaryKey)
	}
	idx.callKeys[ref] = contextKey
	return contextKey, true, true
}

func (idx *contextIndex) upsertFunctionExpressionContext(
	reg *axis.Registry,
	ref functionExpressionRef,
	baseKey summary.SummaryKey,
	callbackFn *ast.FunctionExpr,
	entry state.State,
	entryKeys *keyspace.KeySpace,
) (summary.SummaryKey, bool, bool) {
	if idx == nil || ref.expr == 0 || callbackFn == nil {
		return summary.SummaryKey{}, false, false
	}
	if existing, seen := idx.functionExpressionKeys[ref]; seen {
		if idx.mergeContextForKey(reg, existing, callbackFn, entryKeys, entry) {
			return existing, true, false
		}
		if _, ok := idx.contextByKey(existing); ok {
			return summary.SummaryKey{}, false, false
		}
	}
	contextKey := idx.nextContextKey(baseKey, contextKeyIdentity{kind: "function-expression", owner: ref.owner, expr: ref.expr})
	idx.appendContext(callbackFn, contextKey, entry, entryKeys)
	if idx.functionExpressionKeys == nil {
		idx.functionExpressionKeys = make(map[functionExpressionRef]summary.SummaryKey)
	}
	idx.functionExpressionKeys[ref] = contextKey
	return contextKey, true, true
}

func (idx *contextIndex) CallContextKey(owner summary.SummaryKey, expr factflow.ExprRef) (summary.SummaryKey, bool) {
	if idx == nil {
		return summary.SummaryKey{}, false
	}
	key, ok := idx.callKeys[callContextRef{owner: canonicalContextOwner(owner), expr: expr}]
	if !ok || !idx.hasContextKey(key) {
		return summary.SummaryKey{}, false
	}
	return key, true
}

func (idx *contextIndex) HasFunctionExpression(owner summary.SummaryKey, expr factflow.ExprRef) bool {
	if idx == nil {
		return false
	}
	key, ok := idx.functionExpressionKeys[functionExpressionRef{owner: canonicalContextOwner(owner), expr: expr}]
	return ok && idx.hasContextKey(key)
}

func (idx *contextIndex) FunctionExpressionKeysForOwner(owner summary.SummaryKey) map[factflow.ExprRef]summary.SummaryKey {
	if idx == nil {
		return nil
	}
	owner = canonicalContextOwner(owner)
	out := make(map[factflow.ExprRef]summary.SummaryKey)
	for ref, key := range idx.functionExpressionKeys {
		if ref.owner == owner && idx.hasContextKey(key) {
			out[ref.expr] = key
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (k *programKeys) refreshContextForKey(reg *axis.Registry, key summary.SummaryKey, fn *ast.FunctionExpr, entryKeys *keyspace.KeySpace, entry state.State) bool {
	if k == nil {
		return false
	}
	entry, entryKeys = k.seedMetatableMethodContextEntry(reg, fn, entry, entryKeys)
	return k.contexts.mergeContextForKey(reg, key, fn, entryKeys, entry)
}

func (k *programKeys) upsertCallContext(
	reg *axis.Registry,
	ref callContextRef,
	baseKey summary.SummaryKey,
	fn *ast.FunctionExpr,
	entry state.State,
	entryKeys *keyspace.KeySpace,
	fnType *typ.Function,
) (summary.SummaryKey, bool) {
	if k == nil {
		return summary.SummaryKey{}, false
	}
	entry, entryKeys = k.seedMetatableMethodContextEntry(reg, fn, entry, entryKeys)
	contextKey, changed, created := k.contexts.upsertCallContext(reg, ref, baseKey, fn, entry, entryKeys)
	if created && fnType != nil {
		k.functionTypes[contextKey] = fnType
	}
	return contextKey, changed
}

func (k *programKeys) upsertFunctionExpressionContext(
	reg *axis.Registry,
	owner summary.SummaryKey,
	expr factflow.ExprRef,
	callbackSymbol symbol.ID,
	callbackFn *ast.FunctionExpr,
	entry state.State,
	entryKeys *keyspace.KeySpace,
	fnType *typ.Function,
) (summary.SummaryKey, bool) {
	if k == nil || expr == 0 || callbackSymbol == 0 || callbackFn == nil {
		return summary.SummaryKey{}, false
	}
	baseKey, ok := k.functionKeys[callbackSymbol]
	if !ok {
		return summary.SummaryKey{}, false
	}
	ref := functionExpressionRef{owner: canonicalContextOwner(owner), expr: expr}
	entry, entryKeys = k.seedMetatableMethodContextEntry(reg, callbackFn, entry, entryKeys)
	contextKey, changed, created := k.contexts.upsertFunctionExpressionContext(reg, ref, baseKey, callbackFn, entry, entryKeys)
	if created {
		k.functionByKey[contextKey] = callbackFn
	}
	if fnType != nil {
		if existing := k.functionTypes[contextKey]; existing == nil || !typ.SameNodeOrAcyclicEqual(existing, fnType) {
			k.functionTypes[contextKey] = fnType
			changed = true
		}
	}
	return contextKey, changed
}

func (k *programKeys) seedMetatableMethodContextEntry(reg *axis.Registry, fn *ast.FunctionExpr, entry state.State, entryKeys *keyspace.KeySpace) (state.State, *keyspace.KeySpace) {
	if k == nil || fn == nil || reg == nil || len(k.metatableMethodReceivers) == 0 {
		return entry, entryKeys
	}
	functions := []keyedFunction{{
		funcExpr:      fn,
		entryState:    entry,
		entryKeys:     entryKeys,
		hasEntryState: true,
	}}
	applyMetatableMethodReceiverEntryStatesTo(functions, k.metatableMethodReceivers, k.metatableSeedReceivers, k.metatableProof, k.bindings, reg)
	return functions[0].entryState, functions[0].entryKeys
}
