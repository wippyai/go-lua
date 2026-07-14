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
	variants               map[semanticContextRef][]summary.SummaryKey
	callKeys               map[callContextRef]summary.SummaryKey
	functionExpressionKeys map[functionExpressionRef]summary.SummaryKey
	discoveryGeneration    uint64
}

// semanticContextRef is the interning identity of a solved callee variant.
// Call sites are intentionally absent; callKeys retains that provenance.
type semanticContextRef struct {
	ref        summary.SummaryKey
	bodyDigest uint64
}

func newContextIndex() contextIndex {
	return contextIndex{
		byKey:    make(map[summary.SummaryKey]int),
		variants: make(map[semanticContextRef][]summary.SummaryKey),
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

// CallSiteHistogram reports how many call sites route to each solved semantic
// variant. The key is sites-per-variant and the value is variant count.
func (idx *contextIndex) CallSiteHistogram() map[int]int {
	if idx == nil || len(idx.callKeys) == 0 {
		return nil
	}
	counts := make(map[summary.SummaryKey]int)
	for _, key := range idx.callKeys {
		if idx.hasContextKey(key) {
			counts[key]++
		}
	}
	histogram := make(map[int]int)
	for _, count := range counts {
		histogram[count]++
	}
	return histogram
}

func (idx *contextIndex) SemanticCallContextCount() int {
	count := 0
	for _, variants := range idx.CallSiteHistogram() {
		count += variants
	}
	return count
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
		// An arbitrary entry transform cannot inherit an exact-entry proof.
		next.relationContextEntry = nil
		idx.entries[i] = next
	}
}

func (idx *contextIndex) nextEntryDiscoveryGeneration() uint64 {
	if idx == nil {
		return 0
	}
	idx.discoveryGeneration++
	if idx.discoveryGeneration == 0 {
		idx.discoveryGeneration++
	}
	return idx.discoveryGeneration
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

func (idx *contextIndex) mergeContextForKey(reg *axis.Registry, key summary.SummaryKey, fn *ast.FunctionExpr, entryKeys *keyspace.KeySpace, entry state.State) (bool, error) {
	context, ok := idx.contextByKey(key)
	if !ok || context.funcExpr != fn {
		return false, nil
	}
	return mergeContextEntry(reg, context, entryKeys, entry)
}

func mergeContextEntry(reg *axis.Registry, context *keyedFunction, entryKeys *keyspace.KeySpace, entry state.State) (bool, error) {
	if context == nil {
		return false, nil
	}
	validated, err := entry.RekeyKeySpace(entryKeys, entryKeys)
	if err != nil {
		return false, err
	}
	entry = validated
	if !context.hasEntryState || reg == nil {
		context.entryState = entry
		context.entryKeys = entryKeys
		context.hasEntryState = true
		context.relationContextEntry = nil
		return true, nil
	}
	// A variant keeps one canonical callee keyspace. Re-key a refreshed caller
	// entry into it instead of changing the representation used by its digest.
	current, err := context.entryState.RekeyKeySpace(context.entryKeys, context.entryKeys)
	if err != nil {
		return false, err
	}
	targetKeys := context.entryKeys
	if targetKeys == nil {
		targetKeys = entryKeys
	}
	entry, err = entry.RekeyKeySpace(entryKeys, targetKeys)
	if err != nil {
		return false, err
	}
	joined := state.Domain(reg).Join(current, entry)
	joined = joined.RefreshValueSlotsFrom(reg, entry)
	if state.Domain(reg).Equal(current, joined) {
		return false, nil
	}
	context.entryState = joined
	context.entryKeys = targetKeys
	// Keep context.entryKeys: it is the variant's canonical representation.
	context.hasEntryState = true
	context.relationContextEntry = nil
	return true, nil
}

func (idx *contextIndex) semanticContextKey(reg *axis.Registry, baseKey summary.SummaryKey, bodyDigest uint64, fn *ast.FunctionExpr, entry state.State, entryKeys *keyspace.KeySpace) (summary.SummaryKey, bool, error) {
	if idx == nil || fn == nil {
		return summary.SummaryKey{}, false, nil
	}
	validated, err := entry.RekeyKeySpace(entryKeys, entryKeys)
	if err != nil {
		return summary.SummaryKey{}, false, err
	}
	entry = validated
	baseKey.Entry = semanticEntryKey(reg, entry, entryKeys)
	ref := semanticContextRef{ref: baseKey, bodyDigest: bodyDigest}
	for _, key := range idx.variants[ref] {
		context, ok := idx.contextByKey(key)
		if !ok || context.funcExpr != fn {
			continue
		}
		stored, err := context.entryState.RekeyKeySpace(context.entryKeys, context.entryKeys)
		if err != nil {
			return summary.SummaryKey{}, false, err
		}
		candidate, err := entry.RekeyKeySpace(entryKeys, context.entryKeys)
		if err != nil {
			return summary.SummaryKey{}, false, err
		}
		if reg == nil || state.Domain(reg).Equal(stored, candidate) {
			return key, true, nil
		}
	}
	return summary.SummaryKey{}, false, nil
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
	fmt.Fprint(h, "call-context-key-v1:")
	writeSummaryKeyDigest(h, baseKey)
	fmt.Fprint(h, "kind:", identity.kind, ";owner:")
	writeSummaryKeyDigest(h, identity.owner)
	fmt.Fprint(h, "expr:", uint32(identity.expr), ";collision:", collision, ";")
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

func (idx *contextIndex) appendSemanticContext(reg *axis.Registry, baseKey summary.SummaryKey, bodyDigest uint64, fn *ast.FunctionExpr, entry state.State, entryKeys *keyspace.KeySpace) summary.SummaryKey {
	entryKey := semanticEntryKey(reg, entry, entryKeys)
	contextKey := baseKey
	contextKey.Entry = entryKey
	// A digest collision must not merge unequal states. Keep the primary
	// content digest unchanged where possible and add a deterministic collision
	// discriminator only for the impossible-to-prove-equal case.
	for collision := uint64(0); idx.hasContextKey(contextKey); collision++ {
		contextKey.Entry.References = stableSemanticCollisionDigest(baseKey, entryKey, collision)
	}
	idx.appendContext(fn, contextKey, entry, entryKeys)
	ref := semanticContextRef{ref: baseKey, bodyDigest: bodyDigest}
	ref.ref.Entry = entryKey
	if idx.variants == nil {
		idx.variants = make(map[semanticContextRef][]summary.SummaryKey)
	}
	idx.variants[ref] = append(idx.variants[ref], contextKey)
	return contextKey
}

func stableSemanticCollisionDigest(base summary.SummaryKey, entry summary.EntryKey, collision uint64) summary.Digest {
	h := fnv.New64a()
	fmt.Fprint(h, "semantic-context-collision-v1:")
	writeSummaryKeyDigest(h, base)
	fmt.Fprintf(h, "entry:%d/%d/%d;collision:%d;", entry.Values, entry.Facts, entry.References, collision)
	return summary.Digest(h.Sum64())
}

func (idx *contextIndex) upsertCallContext(
	reg *axis.Registry,
	ref callContextRef,
	baseKey summary.SummaryKey,
	fn *ast.FunctionExpr,
	entry state.State,
	entryKeys *keyspace.KeySpace,
	bodyDigest uint64,
) (summary.SummaryKey, bool, bool, error) {
	if idx == nil || fn == nil {
		return summary.SummaryKey{}, false, false, nil
	}
	// A previously seen source site is a refresh of its existing equation
	// input, not a new specialization. Keep its routing stable and merge the
	// refreshed entry exactly as the old monotone context driver did. Semantic
	// interning applies when a distinct site first discovers its entry.
	if existing, seen := idx.callKeys[ref]; seen {
		changed, err := idx.mergeContextForKey(reg, existing, fn, entryKeys, entry)
		if err != nil {
			return summary.SummaryKey{}, false, false, err
		}
		if changed {
			return existing, true, false, nil
		}
		if _, ok := idx.contextByKey(existing); ok {
			return summary.SummaryKey{}, false, false, nil
		}
	}
	contextKey, ok, err := idx.semanticContextKey(reg, baseKey, bodyDigest, fn, entry, entryKeys)
	if err != nil {
		return summary.SummaryKey{}, false, false, err
	}
	if ok {
		if idx.callKeys == nil {
			idx.callKeys = make(map[callContextRef]summary.SummaryKey)
		}
		previous, seen := idx.callKeys[ref]
		idx.callKeys[ref] = contextKey
		return contextKey, !seen || previous != contextKey, false, nil
	}
	contextKey = idx.appendSemanticContext(reg, baseKey, bodyDigest, fn, entry, entryKeys)
	if idx.callKeys == nil {
		idx.callKeys = make(map[callContextRef]summary.SummaryKey)
	}
	idx.callKeys[ref] = contextKey
	return contextKey, true, true, nil
}

func (idx *contextIndex) upsertFunctionExpressionContext(
	reg *axis.Registry,
	ref functionExpressionRef,
	baseKey summary.SummaryKey,
	callbackFn *ast.FunctionExpr,
	entry state.State,
	entryKeys *keyspace.KeySpace,
	bodyDigest uint64,
) (summary.SummaryKey, bool, bool, error) {
	if idx == nil || ref.expr == 0 || callbackFn == nil {
		return summary.SummaryKey{}, false, false, nil
	}
	if existing, seen := idx.functionExpressionKeys[ref]; seen {
		changed, err := idx.mergeContextForKey(reg, existing, callbackFn, entryKeys, entry)
		if err != nil {
			return summary.SummaryKey{}, false, false, err
		}
		if changed {
			return existing, true, false, nil
		}
		if _, ok := idx.contextByKey(existing); ok {
			return summary.SummaryKey{}, false, false, nil
		}
	}
	contextKey, ok, err := idx.semanticContextKey(reg, baseKey, bodyDigest, callbackFn, entry, entryKeys)
	if err != nil {
		return summary.SummaryKey{}, false, false, err
	}
	if ok {
		if idx.functionExpressionKeys == nil {
			idx.functionExpressionKeys = make(map[functionExpressionRef]summary.SummaryKey)
		}
		previous, seen := idx.functionExpressionKeys[ref]
		idx.functionExpressionKeys[ref] = contextKey
		return contextKey, !seen || previous != contextKey, false, nil
	}
	contextKey = idx.appendSemanticContext(reg, baseKey, bodyDigest, callbackFn, entry, entryKeys)
	if idx.functionExpressionKeys == nil {
		idx.functionExpressionKeys = make(map[functionExpressionRef]summary.SummaryKey)
	}
	idx.functionExpressionKeys[ref] = contextKey
	return contextKey, true, true, nil
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
	_, ok := idx.FunctionExpressionKey(owner, expr)
	return ok
}

// FunctionExpressionKey returns the live context summary key for an owner-local
// callback expression without materializing the owner's complete routing map.
func (idx *contextIndex) FunctionExpressionKey(owner summary.SummaryKey, expr factflow.ExprRef) (summary.SummaryKey, bool) {
	if idx == nil {
		return summary.SummaryKey{}, false
	}
	key, ok := idx.functionExpressionKeys[functionExpressionRef{owner: canonicalContextOwner(owner), expr: expr}]
	if !ok || !idx.hasContextKey(key) {
		return summary.SummaryKey{}, false
	}
	return key, true
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

func (k *programKeys) refreshContextForKey(reg *axis.Registry, key summary.SummaryKey, fn *ast.FunctionExpr, entryKeys *keyspace.KeySpace, entry state.State) (bool, error) {
	if k == nil {
		return false, nil
	}
	entry, entryKeys = k.seedMetatableMethodContextEntry(reg, fn, entry, entryKeys)
	changed, err := k.contexts.mergeContextForKey(reg, key, fn, entryKeys, entry)
	if err != nil {
		return false, err
	}
	if changed {
		// refreshContextForKey lacks the base/prepared identity required to
		// rebuild the proof, so the merge above clears it transactionally.
		k.contexts.nextEntryDiscoveryGeneration()
	}
	return changed, nil
}

func (k *programKeys) upsertCallContext(
	reg *axis.Registry,
	ref callContextRef,
	baseKey summary.SummaryKey,
	fn *ast.FunctionExpr,
	entry state.State,
	entryKeys *keyspace.KeySpace,
	fnType *typ.Function,
	bodyDigest ...uint64,
) (summary.SummaryKey, bool, error) {
	if k == nil {
		return summary.SummaryKey{}, false, nil
	}
	entry, entryKeys = k.seedMetatableMethodContextEntry(reg, fn, entry, entryKeys)
	var digest uint64
	if len(bodyDigest) != 0 {
		digest = bodyDigest[0]
	}
	contextKey, changed, created, err := k.contexts.upsertCallContext(reg, ref, baseKey, fn, entry, entryKeys, digest)
	if err != nil {
		return summary.SummaryKey{}, false, err
	}
	if changed && k.certifyRelationContexts {
		k.contexts.nextEntryDiscoveryGeneration()
	}
	if created && fnType != nil {
		k.functionTypes[contextKey] = fnType
	}
	return contextKey, changed, nil
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
	bodyDigest ...uint64,
) (summary.SummaryKey, bool, error) {
	if k == nil || expr == 0 || callbackSymbol == 0 || callbackFn == nil {
		return summary.SummaryKey{}, false, nil
	}
	baseKey, ok := k.functionKeys[callbackSymbol]
	if !ok {
		return summary.SummaryKey{}, false, nil
	}
	ref := functionExpressionRef{owner: canonicalContextOwner(owner), expr: expr}
	entry, entryKeys = k.seedMetatableMethodContextEntry(reg, callbackFn, entry, entryKeys)
	var digest uint64
	if len(bodyDigest) != 0 {
		digest = bodyDigest[0]
	}
	contextKey, changed, created, err := k.contexts.upsertFunctionExpressionContext(reg, ref, baseKey, callbackFn, entry, entryKeys, digest)
	if err != nil {
		return summary.SummaryKey{}, false, err
	}
	if changed && k.certifyRelationContexts {
		k.contexts.nextEntryDiscoveryGeneration()
	}
	if created {
		k.functionByKey[contextKey] = callbackFn
	}
	if fnType != nil {
		if existing := k.functionTypes[contextKey]; existing == nil || !typ.SameNodeOrAcyclicEqual(existing, fnType) {
			k.functionTypes[contextKey] = fnType
			changed = true
		}
	}
	return contextKey, changed, nil
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
