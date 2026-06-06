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

// PointFactsOf returns the read-only facts view for state.
func PointFactsOf(state PointState) PointFacts {
	return PointFacts{state: state}
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
	parentSegs := parent.Segments
	children := make(map[string]PathFact)
	for _, entry := range f.state.StaticMembers.Entries() {
		sym, segs, ok := ParseSymbolPathKey(entry.Path)
		if !ok || sym != parent.Symbol {
			continue
		}
		if len(segs) <= len(parentSegs) || !segmentsPrefix(parentSegs, segs) {
			continue
		}
		childSeg := segs[len(parentSegs)]
		childPath := parent
		childPath.Segments = append(append([]constraint.Segment(nil), parentSegs...), childSeg)
		childKey := constraint.FormatSegments([]constraint.Segment{childSeg})
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

// HasKeyValuePresence reports whether key-presence also tracks the value path
// produced by table[key].
func (f PointFacts) HasKeyValuePresence(table, key, value constraint.Path) bool {
	tableAddr, tableOK := StableAddressOfPath(table)
	keyAddr, keyOK := StableAddressOfPath(key)
	valueAddr, valueOK := StableAddressOfPath(value)
	return tableOK && keyOK && valueOK && f.state.KeyPresence.HasValueAddresses(tableAddr, keyAddr, valueAddr)
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

// IndexWritePathQuery is the structured-path query form for admitted dynamic
// index writes.
type IndexWritePathQuery struct {
	Target       constraint.Path
	KeyPath      constraint.Path
	HasKeyPath   bool
	KeyValue     product.AbstractValue
	ValuePath    constraint.Path
	HasValuePath bool
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
	addressQuery := IndexWriteAddressQuery{
		Target:   target,
		KeyValue: q.KeyValue,
	}
	if q.HasKeyPath {
		key, ok := StableAddressOfPath(q.KeyPath)
		if !ok {
			return product.AbstractValue{}, false
		}
		addressQuery.KeyPath = key
		addressQuery.HasKeyPath = true
	}
	if q.HasValuePath {
		value, ok := StableAddressOfPath(q.ValuePath)
		if !ok {
			return product.AbstractValue{}, false
		}
		addressQuery.ValuePath = value
		addressQuery.HasValuePath = true
	}
	return f.state.IndexWrites.AdmissionAtAddress(addressQuery)
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
