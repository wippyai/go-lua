package flow

import (
	"sort"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

// PointFacts is the read-only projection boundary for one PointState.
//
// It centralizes the normalized read rules for product-state axes so driver,
// observation, and transfer consumers do not each know how to combine Env,
// Cells, StaticMembers, and numeric length facts. It does not resolve syntax,
// summaries, callees, or declared types.
type PointFacts struct {
	state PointState
}

// CallableSignatureQuery asks the checker-owned callable projector for the
// signature represented by either a concrete function identity or a runtime
// value path. State carries the live axes needed to project closure captures.
type CallableSignatureQuery struct {
	Ref   FunctionRef
	Path  constraint.Path
	State PointState
}

// CallableSignatureResolver resolves callable identity facts to signatures.
type CallableSignatureResolver func(CallableSignatureQuery) (typ.Type, bool)

// PointFactsOf returns the read-only facts view for state.
func PointFactsOf(state PointState) PointFacts {
	return PointFacts{state: state}
}

// SingleChangedValueSlot reports the one logical value slot changed from before
// to after. Env symbol keys and Cells are both reported as symbol slots so
// callers can apply their own lexical storage policy without decoding key
// strings.
func SingleChangedValueSlot(before, after PointState) (ValueSlot, bool) {
	var changed ValueSlot
	setChanged := func(slot ValueSlot) bool {
		if _, ok := changed.ValueKey(); ok {
			return false
		}
		changed = slot
		return true
	}
	for key, next := range after.Env {
		prev, had := before.Env[key]
		if had && product.Domain.Equal(prev, next) {
			continue
		}
		slot, ok := ValueKeySlot(key)
		if !ok || !setChanged(slot) {
			return ValueSlot{}, false
		}
	}
	for _, cell := range after.Cells.Entries() {
		prev, _ := before.Cells.Value(cell.Symbol)
		if product.Domain.Equal(prev, cell.Value) {
			continue
		}
		slot, ok := SymbolValueSlot(cell.Symbol)
		if !ok || !setChanged(slot) {
			return ValueSlot{}, false
		}
	}
	for _, cell := range before.Cells.Entries() {
		if _, ok := after.Cells.Value(cell.Symbol); ok {
			continue
		}
		slot, ok := SymbolValueSlot(cell.Symbol)
		if !ok || !setChanged(slot) {
			return ValueSlot{}, false
		}
	}
	_, ok := changed.ValueKey()
	return changed, ok
}

// SingleChangedValueKey reports the one logical value slot changed from before
// to after as a ValueKey. Prefer SingleChangedValueSlot when the caller needs to
// distinguish symbol slots from non-symbol Env keys.
func SingleChangedValueKey(before, after PointState) (ValueKey, bool) {
	slot, ok := SingleChangedValueSlot(before, after)
	if !ok {
		return "", false
	}
	return slot.ValueKey()
}

// SymbolValue returns sym's low-level slot value, using Cells before Env when a
// cell entry exists. It is intentionally lexical-policy-free; transfer code
// that knows whether a symbol is Env-backed or cell-backed should use its
// symbol-storage boundary instead.
func (f PointFacts) SymbolValue(sym cfg.SymbolID) (product.AbstractValue, bool) {
	return SymbolValue(f.state, sym)
}

// EnvValue returns the abstract value stored directly in Env under key.
func (f PointFacts) EnvValue(key ValueKey) (product.AbstractValue, bool) {
	av, ok := f.state.Env[key]
	if !ok || av.IsZero() {
		return product.AbstractValue{}, false
	}
	return av, true
}

// EnvSymbolValue returns the Env entry for sym without consulting Cells.
func (f PointFacts) EnvSymbolValue(sym cfg.SymbolID) (product.AbstractValue, bool) {
	if sym == 0 {
		return product.AbstractValue{}, false
	}
	return f.EnvValue(SymbolValueKey(sym))
}

// EnvCaptureCells projects Env-backed symbol values into the capture-cell
// domain for the requested symbols. It deliberately reads only Env entries:
// existing Cells are already represented on the Cells axis and should be joined
// by the caller when publishing capture exports.
func (f PointFacts) EnvCaptureCells(allowed map[cfg.SymbolID]bool) CaptureCells {
	if len(f.state.Env) == 0 || len(allowed) == 0 {
		return CaptureCellsDomain.Bottom()
	}
	entries := make([]CaptureCell, 0, len(allowed))
	for key, av := range f.state.Env {
		sym, ok := ParseSymbolValueKey(key)
		if !ok || !allowed[sym] || av.IsZero() {
			continue
		}
		entries = append(entries, CaptureCell{Symbol: sym, Value: av})
	}
	return CaptureCellsOf(entries)
}

// CellValue returns the abstract value stored directly in the capture-cell axis.
func (f PointFacts) CellValue(sym cfg.SymbolID) (product.AbstractValue, bool) {
	if sym == 0 {
		return product.AbstractValue{}, false
	}
	av, ok := f.state.Cells.Value(sym)
	if !ok || av.IsZero() {
		return product.AbstractValue{}, false
	}
	return av, true
}

// ValueKeyValue returns the abstract value stored under key.
//
// Symbol keys are resolved through SymbolValue so captured Cells retain
// precedence over Env. Non-symbol keys, such as return-slot keys, are read from
// Env directly.
func (f PointFacts) ValueKeyValue(key ValueKey) (product.AbstractValue, bool) {
	if sym, ok := ParseSymbolValueKey(key); ok {
		return f.SymbolValue(sym)
	}
	return f.EnvValue(key)
}

// ReturnSlotValue returns the scratch value recorded for a non-symbol return
// expression at slot index.
func (f PointFacts) ReturnSlotValue(index int) (product.AbstractValue, bool) {
	if index < 0 {
		return product.AbstractValue{}, false
	}
	return f.EnvValue(ReturnSlotValueKey(index))
}

// ReturnSlotStoredArity reports one past the greatest non-empty scratch return
// slot present in Env. Summary projection uses it to expand a single returned
// call into the callee arity the transfer materialized.
func (f PointFacts) ReturnSlotStoredArity() int {
	maxSlot := -1
	for key, av := range f.state.Env {
		if av.IsZero() {
			continue
		}
		idx, ok := ParseReturnSlotValueKey(key)
		if !ok || idx < 0 {
			continue
		}
		if idx > maxSlot {
			maxSlot = idx
		}
	}
	return maxSlot + 1
}

// SymbolType returns sym's projected structural type when a product value is
// present and informative.
func (f PointFacts) SymbolType(sym cfg.SymbolID) (typ.Type, bool) {
	return productType(f.SymbolValue(sym))
}

// StaticMemberValue returns the point-local must fact for an exact static path.
func (f PointFacts) StaticMemberValue(path constraint.Path) (product.AbstractValue, bool) {
	if path.Symbol == 0 {
		return product.AbstractValue{}, false
	}
	addr, ok := StableAddressOfPath(path)
	if !ok {
		return product.AbstractValue{}, false
	}
	return f.state.StaticMembers.ValueAtAddress(addr)
}

// AddressValue returns a product value for a stable symbol-rooted address. It
// is the address-domain form of PathValue for callers that already operate on
// normalized fact identities.
func (f PointFacts) AddressValue(addr StableAddress) (product.AbstractValue, bool) {
	path, ok := addr.Path()
	if !ok || path.Symbol == 0 {
		return product.AbstractValue{}, false
	}
	return f.PathValue(path)
}

// PathValue returns a product value for path by applying point-local static
// member facts and structural product-member traversal from the root value.
func (f PointFacts) PathValue(path constraint.Path) (product.AbstractValue, bool) {
	if path.Symbol == 0 {
		return product.AbstractValue{}, false
	}
	if len(path.Segments) == 0 {
		return f.SymbolValue(path.Symbol)
	}
	if fact, ok := f.StaticMemberValue(path); ok {
		return fact, true
	}
	cur, ok := f.SymbolValue(path.Symbol)
	if !ok || cur.IsZero() {
		return product.AbstractValue{}, false
	}
	for i, seg := range path.Segments {
		prefix := constraint.Path{
			Root:     path.Root,
			Symbol:   path.Symbol,
			Version:  path.Version,
			Segments: path.Segments[:i+1],
		}
		if fact, ok := f.StaticMemberValue(prefix); ok {
			cur = fact
			continue
		}
		member, ok := value.MemberFromSegment(seg)
		if !ok {
			return product.AbstractValue{}, false
		}
		next, ok := product.MemberOf(cur, member)
		if !ok || next.IsZero() {
			return product.AbstractValue{}, false
		}
		cur = next
	}
	return cur, true
}

// CallablePathValue is the canonical product read for a runtime path that may
// carry function identity facts. It first reads the point-state product value,
// then overlays the callable signature proven by FunctionRefs/ClosureRefs or a
// static path resolver. Function identity does not by itself prove a nested slot
// is present, so member reads use RefineCallableRead while root values use
// RefineCallableValue.
func (f PointFacts) CallablePathValue(path constraint.Path, resolve CallableSignatureResolver) (product.AbstractValue, bool) {
	if path.Symbol == 0 {
		return product.AbstractValue{}, false
	}
	av, hasValue := f.PathValue(path)
	if !hasValue {
		av = product.AbstractValue{}
	}
	return f.CallablePathRead(path, av, resolve)
}

// CallablePathRead overlays callable identity facts onto an already-computed
// runtime read for path. Use this when the caller must preserve Lua runtime
// read semantics, for example table/member reads that distinguish strict shape
// lookup from missing-slot nil.
func (f PointFacts) CallablePathRead(path constraint.Path, read product.AbstractValue, resolve CallableSignatureResolver) (product.AbstractValue, bool) {
	if path.Symbol == 0 {
		return product.AbstractValue{}, false
	}
	sig, hasSig := f.CallablePathType(path, resolve)
	if !hasSig {
		if read.IsZero() {
			return product.AbstractValue{}, false
		}
		return read, true
	}
	if len(path.Segments) == 0 {
		return product.RefineCallableValue(read, sig), true
	}
	return product.RefineCallableRead(read, sig), true
}

// CallablePathType resolves the signature for a function-valued path without
// reading its product value.
func (f PointFacts) CallablePathType(path constraint.Path, resolve CallableSignatureResolver) (typ.Type, bool) {
	if path.Symbol == 0 || resolve == nil {
		return nil, false
	}
	state := PointState{
		Cells:        f.state.Cells,
		FunctionRefs: f.state.FunctionRefs,
		ClosureRefs:  f.state.ClosureRefs,
	}
	if refs, ok := FunctionRefAtPath(f.state.FunctionRefs, path); ok {
		if ref, singleton := refs.Singleton(); singleton {
			if sig, ok := resolve(CallableSignatureQuery{Ref: ref, State: state}); ok && !typ.IsAbsentOrUnknown(sig) {
				return sig, true
			}
		}
	}
	if sig, ok := resolve(CallableSignatureQuery{Path: path, State: state}); ok && !typ.IsAbsentOrUnknown(sig) {
		return sig, true
	}
	return nil, false
}

// PathType returns path's projected structural type when a product value is
// present and informative.
func (f PointFacts) PathType(path constraint.Path) (typ.Type, bool) {
	return productType(f.PathValue(path))
}

// ChildPathFacts returns direct child path facts that are already materialized
// below parent in this point state. It enumerates finite StaticMembers only and
// asks PathType for each direct child, so nested facts are folded through the
// same normalized point-state read law as ordinary path observations.
func (f PointFacts) ChildPathFacts(parent constraint.Path) []PathFact {
	if parent.Symbol == 0 {
		return nil
	}
	parentAddr, ok := StableAddressOfPath(parent)
	if !ok {
		return nil
	}
	children := make(map[string]PathFact)
	for _, childAddr := range f.state.StaticMembers.DirectChildAddressesUnder(parentAddr) {
		childPath, ok := childAddr.Path()
		if !ok || childPath.Symbol != parent.Symbol {
			continue
		}
		remainder, ok := childAddr.RemainderAfterPrefix(parentAddr)
		if !ok || len(remainder) != 1 {
			continue
		}
		childKey := constraint.FormatSegments(remainder)
		if _, seen := children[childKey]; seen {
			continue
		}
		t, ok := f.PathType(childPath)
		if !ok || typ.IsAbsentOrUnknown(t) {
			continue
		}
		children[childKey] = PathFact{Path: childPath, Type: t}
	}
	return sortedPathFacts(children)
}

// LengthLowerBound returns the numeric lower-bound proof for #path when known.
func (f PointFacts) LengthLowerBound(path constraint.Path) (int64, bool) {
	if path.Symbol == 0 || f.state.Num == nil {
		return 0, false
	}
	lower, _, ok := f.state.Num.LenBoundsFor(SymbolPathKey(path.Symbol, path.Segments))
	return lower, ok
}

// HasKeyPresence reports whether the key-presence axis proves table[key] is
// present for structured paths.
func (f PointFacts) HasKeyPresence(table, key constraint.Path) bool {
	tableAddr, tableOK := StableAddressOfPath(table)
	keyAddr, keyOK := StableAddressOfPath(key)
	return tableOK && keyOK && f.state.KeyPresence.HasAddresses(tableAddr, keyAddr)
}

func (f PointFacts) hasKeyValuePresence(table, key, value constraint.Path) bool {
	tableAddr, tableOK := StableAddressOfPath(table)
	keyAddr, keyOK := StableAddressOfPath(key)
	valueAddr, valueOK := StableAddressOfPath(value)
	return tableOK && keyOK && valueOK && f.state.KeyPresence.HasValueAddresses(tableAddr, keyAddr, valueAddr)
}

// KeyValueReadbackSourceQuery asks whether ValuePath is proven to be the current
// readback value of TablePath[KeyPath].
type KeyValueReadbackSourceQuery struct {
	TablePath constraint.Path
	KeyPath   constraint.Path
	ValuePath constraint.Path
}

// HasKeyValueReadbackSource reports whether the reduced product proves
// ValuePath was read from TablePath[KeyPath]. Producers use this as an identity
// proof for self-derived dynamic writes; the raw key-presence storage stays
// hidden behind this semantic query.
func (f PointFacts) HasKeyValueReadbackSource(q KeyValueReadbackSourceQuery) bool {
	return f.hasKeyValuePresence(q.TablePath, q.KeyPath, q.ValuePath)
}

func (f PointFacts) HasEmptyKeyArray(array constraint.Path) bool {
	arrayAddr, ok := StableAddressOfPath(array)
	return ok && f.state.KeyPresence.HasEmptyKeyArray(arrayAddr.Key())
}

func (f PointFacts) HasAppendHistoryBase(array constraint.Path) bool {
	arrayAddr, ok := StableAddressOfPath(array)
	return ok && f.state.KeyPresence.HasAppendHistoryBase(arrayAddr.Key())
}

// IdentityAliasClosurePaths returns root plus every assignment/path alias
// reachable through the identity-alias relation, normalized back to paths.
func (f PointFacts) IdentityAliasClosurePaths(root constraint.Path) []constraint.Path {
	rootAddr, ok := StableAddressOfPath(root)
	if !ok {
		return nil
	}
	addrs := IdentityAliasClosure(f.state, rootAddr)
	out := make([]constraint.Path, 0, len(addrs))
	for _, addr := range addrs {
		path, ok := addr.Path()
		if ok && !path.IsEmpty() {
			out = append(out, path)
		}
	}
	return out
}

// IdentityAliasSourcePaths returns alias-source paths selected by policy.
// PointFacts owns this normalization so AST-facing transfer code can stay in
// structured paths while the flow relation remains address-native internally.
func (f PointFacts) IdentityAliasSourcePaths(root constraint.Path, policy IdentityAliasRoutePolicy) []constraint.Path {
	rootAddr, ok := StableAddressOfPath(root)
	if !ok {
		return nil
	}
	addrs := IdentityAliasSourcesWithPolicy(f.state, rootAddr, policy)
	out := make([]constraint.Path, 0, len(addrs))
	for _, addr := range addrs {
		path, ok := addr.Path()
		if ok && !path.IsEmpty() {
			out = append(out, path)
		}
	}
	return out
}

// ValueOriginUsesCoveringPath returns value-origin uses that cover path.
// The relation is address-native internally because it must compare structural
// prefixes, but consumers that reason over AST paths should not repeat that
// lowering at each call site.
func (f PointFacts) ValueOriginUsesCoveringPath(path constraint.Path) []ValueOriginUse {
	addr, ok := StableAddressOfPath(path)
	if !ok {
		return nil
	}
	return f.state.ValueOrigins.OriginsCoveringAddress(addr)
}

// IndexWritePathQuery is the structured-path query form for admitted dynamic
// index writes.
type IndexWritePathQuery struct {
	Target     constraint.Path
	KeyPath    constraint.Path
	HasKeyPath bool
	// FollowKeyAliases asks the PointFacts read boundary to apply identity
	// alias closure before querying the address-native index-write facts.
	FollowKeyAliases bool
	KeyValue         product.AbstractValue
	ValuePath        constraint.Path
	HasValuePath     bool
}

// DynamicIndexReadbackQuery describes a runtime table[key] read that may be
// refined by admitted dynamic-index write facts. KeyValue is the evaluated key
// product; PointFacts owns the dynamic-key normalization and the rule that only
// literal keys may use value-only readback without a stable key path.
type DynamicIndexReadbackQuery struct {
	Target           constraint.Path
	KeyPath          constraint.Path
	KeyValue         product.AbstractValue
	FollowKeyAliases bool
}

// IndexWriteAdmissionAtAddress returns the value admitted by a dynamic index
// write proven in this point state using normalized address-domain evidence.
func (f PointFacts) IndexWriteAdmissionAtAddress(q IndexWriteAddressQuery) (typ.Type, bool) {
	av, ok := f.state.IndexWrites.AdmissionAtAddress(q)
	if !ok || av.IsZero() {
		return nil, false
	}
	t := product.ProjectValueOrUnknown(av)
	if typ.IsAbsentOrUnknown(t) {
		return nil, false
	}
	return t, true
}

// IndexWriteAdmission returns the value admitted by a dynamic index write using
// structured paths. PointFacts owns the path-to-address normalization for reads.
func (f PointFacts) IndexWriteAdmission(q IndexWritePathQuery) (product.AbstractValue, bool) {
	target, ok := StableAddressOfPath(q.Target)
	if !ok {
		return product.AbstractValue{}, false
	}
	valueAddr := StableAddress{}
	if q.HasValuePath {
		value, ok := StableAddressOfPath(q.ValuePath)
		if !ok {
			return product.AbstractValue{}, false
		}
		valueAddr = value
	}
	keyPaths := []constraint.Path{q.KeyPath}
	if !q.HasKeyPath {
		keyPaths = []constraint.Path{{}}
	} else if q.FollowKeyAliases {
		keyPaths = f.IdentityAliasClosurePaths(q.KeyPath)
	}
	for _, keyPath := range keyPaths {
		addressQuery := IndexWriteAddressQuery{
			Target:       target,
			KeyValue:     q.KeyValue,
			ValuePath:    valueAddr,
			HasValuePath: q.HasValuePath,
			HasKeyPath:   q.HasKeyPath,
		}
		if q.HasKeyPath {
			key, ok := StableAddressOfPath(keyPath)
			if !ok {
				continue
			}
			addressQuery.KeyPath = key
		}
		if admitted, ok := f.state.IndexWrites.AdmissionAtAddress(addressQuery); ok && !admitted.IsZero() {
			return admitted, true
		}
	}
	return product.AbstractValue{}, false
}

// DynamicIndexReadback returns the product value admitted for one runtime
// table[key] read. It is the product-valued counterpart of IndexWriteAdmission:
// callers provide a structural table path plus the evaluated key, and PointFacts
// decides whether path identity or literal key value is sufficient to consume
// the readback proof.
func (f PointFacts) DynamicIndexReadback(q DynamicIndexReadbackQuery) (product.AbstractValue, bool) {
	if q.Target.IsEmpty() || q.KeyValue.IsZero() {
		return product.AbstractValue{}, false
	}
	keyType := NormalizeDynamicKeyType(product.ProjectValueOrUnknown(q.KeyValue))
	keyValue := product.FromType(keyType)
	if q.KeyPath.HasSymbol() {
		if admitted, ok := f.IndexWriteAdmission(IndexWritePathQuery{
			Target:           q.Target,
			KeyPath:          q.KeyPath,
			HasKeyPath:       true,
			FollowKeyAliases: q.FollowKeyAliases,
			KeyValue:         keyValue,
		}); ok && !admitted.IsZero() {
			return admitted, true
		}
	}
	if !IndexWriteReadCanUseKeyValueOnly(keyType) {
		return product.AbstractValue{}, false
	}
	admitted, ok := f.IndexWriteAdmission(IndexWritePathQuery{
		Target:   q.Target,
		KeyValue: keyValue,
	})
	if !ok || admitted.IsZero() {
		return product.AbstractValue{}, false
	}
	return admitted, true
}

func productType(av product.AbstractValue, ok bool) (typ.Type, bool) {
	if !ok || av.IsZero() {
		return nil, false
	}
	t := product.ProjectValueOrUnknown(av)
	if typ.IsAbsentOrUnknown(t) {
		return nil, false
	}
	return t, true
}

func sortedPathFacts(facts map[string]PathFact) []PathFact {
	if len(facts) == 0 {
		return nil
	}
	keys := make([]string, 0, len(facts))
	for key := range facts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]PathFact, 0, len(keys))
	for _, key := range keys {
		out = append(out, facts[key])
	}
	return out
}
