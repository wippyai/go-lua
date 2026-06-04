package flow

import (
	"fmt"
	"strings"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/lattice"
)

// KeyPresenceFact is a point-local must-fact proving that Key is present in
// Table. It is the product-state carrier for KeyOf(table, key) provenance, used
// by keyed iteration and exact dynamic-index reads.
type KeyPresenceFact struct {
	Table constraint.PathKey
	Key   constraint.PathKey
}

// KeyValueFact is a point-local must-fact proving that Value currently denotes
// Table[Key]. It is seeded by keyed iteration (`for k, v in pairs(table)`) and is
// killed by writes to table, key, or value.
type KeyValueFact struct {
	Table constraint.PathKey
	Key   constraint.PathKey
	Value constraint.PathKey
}

// KeyArrayFact is a point-local must-fact proving that every current element of
// Array is a key of Table. It is seeded by keys-collector call assignments and
// killed by writes to the array or table before indexed iteration consumes it.
type KeyArrayFact struct {
	Array constraint.PathKey
	Table constraint.PathKey
}

// KeyPresenceFacts is a finite must-set lattice over KeyOf provenance. Bottom is
// unreachable, Top is the empty fact set, and finite states are sorted
// table/key, table/key/value, and key-array facts. Join keeps only facts proven
// by every predecessor.
type KeyPresenceFacts struct {
	bottom  bool
	entries []KeyPresenceFact
	values  []KeyValueFact
	arrays  []KeyArrayFact
}

// KeyPresenceFactsOf constructs a canonical finite fact set.
func KeyPresenceFactsOf(entries []KeyPresenceFact) KeyPresenceFacts {
	return canonicalKeyPresenceFacts(entries)
}

// KeyPresenceFactFromPaths lowers source paths to the version-insensitive
// structural keys used by canonical point-state components.
func KeyPresenceFactFromPaths(table, key constraint.Path) (KeyPresenceFact, bool) {
	tableKey := KeyPresencePathKey(table)
	keyKey := KeyPresencePathKey(key)
	if tableKey == "" || keyKey == "" {
		return KeyPresenceFact{}, false
	}
	return KeyPresenceFact{Table: tableKey, Key: keyKey}, true
}

// KeyPresencePathKey returns the typed structural key used by key-presence facts.
// Symbol paths use the stable symbol/segment key; root-only fallback is accepted
// only for boundary paths that have not been symbolized.
func KeyPresencePathKey(path constraint.Path) constraint.PathKey {
	if path.Symbol != 0 {
		return SymbolPathKey(path.Symbol, path.Segments)
	}
	path.Version = 0
	return path.Key()
}

func (f KeyPresenceFacts) IsBottom() bool { return f.bottom }

func (f KeyPresenceFacts) Entries() []KeyPresenceFact {
	if f.bottom || len(f.entries) == 0 {
		return nil
	}
	return append([]KeyPresenceFact(nil), f.entries...)
}

func (f KeyPresenceFacts) Has(table, key constraint.PathKey) bool {
	if f.bottom || table == "" || key == "" || len(f.entries) == 0 {
		return false
	}
	_, ok := findKeyPresenceFact(f.entries, KeyPresenceFact{Table: table, Key: key})
	return ok
}

func (f KeyPresenceFacts) HasPaths(table, key constraint.Path) bool {
	fact, ok := KeyPresenceFactFromPaths(table, key)
	if !ok {
		return false
	}
	return f.Has(fact.Table, fact.Key)
}

func (f KeyPresenceFacts) With(table, key constraint.PathKey) KeyPresenceFacts {
	if table == "" || key == "" {
		return f
	}
	if f.bottom {
		f = KeyPresenceFacts{}
	}
	next := f.Entries()
	fact := KeyPresenceFact{Table: table, Key: key}
	if _, ok := findKeyPresenceFact(next, fact); !ok {
		next = append(next, fact)
	}
	return canonicalKeyPresenceFactsFull(next, f.values, f.arrays)
}

func (f KeyPresenceFacts) WithPaths(table, key constraint.Path) KeyPresenceFacts {
	fact, ok := KeyPresenceFactFromPaths(table, key)
	if !ok {
		return f
	}
	return f.With(fact.Table, fact.Key)
}

func (f KeyPresenceFacts) HasValue(table, key, value constraint.PathKey) bool {
	if f.bottom || table == "" || key == "" || value == "" || len(f.values) == 0 {
		return false
	}
	_, ok := findKeyValueFact(f.values, KeyValueFact{Table: table, Key: key, Value: value})
	return ok
}

func (f KeyPresenceFacts) HasValuePaths(table, key, value constraint.Path) bool {
	tableKey := KeyPresencePathKey(table)
	keyKey := KeyPresencePathKey(key)
	valueKey := KeyPresencePathKey(value)
	return f.HasValue(tableKey, keyKey, valueKey)
}

func (f KeyPresenceFacts) WithValue(table, key, value constraint.PathKey) KeyPresenceFacts {
	if table == "" || key == "" || value == "" {
		return f
	}
	if f.bottom {
		f = KeyPresenceFacts{}
	}
	f = f.With(table, key)
	next := f.ValueEntries()
	fact := KeyValueFact{Table: table, Key: key, Value: value}
	if _, ok := findKeyValueFact(next, fact); !ok {
		next = append(next, fact)
	}
	return canonicalKeyPresenceFactsFull(f.entries, next, f.arrays)
}

func (f KeyPresenceFacts) WithValuePaths(table, key, value constraint.Path) KeyPresenceFacts {
	tableKey := KeyPresencePathKey(table)
	keyKey := KeyPresencePathKey(key)
	valueKey := KeyPresencePathKey(value)
	return f.WithValue(tableKey, keyKey, valueKey)
}

func (f KeyPresenceFacts) ValueEntries() []KeyValueFact {
	if f.bottom || len(f.values) == 0 {
		return nil
	}
	return append([]KeyValueFact(nil), f.values...)
}

func (f KeyPresenceFacts) WithKeyArray(array, table constraint.PathKey) KeyPresenceFacts {
	if array == "" || table == "" {
		return f
	}
	if f.bottom {
		f = KeyPresenceFacts{}
	}
	next := f.KeyArrayEntries()
	fact := KeyArrayFact{Array: array, Table: table}
	if _, ok := findKeyArrayFact(next, fact); !ok {
		next = append(next, fact)
	}
	return canonicalKeyPresenceFactsFull(f.entries, f.values, next)
}

func (f KeyPresenceFacts) WithKeyArrayPaths(array, table constraint.Path) KeyPresenceFacts {
	arrayKey := KeyPresencePathKey(array)
	tableKey := KeyPresencePathKey(table)
	return f.WithKeyArray(arrayKey, tableKey)
}

func (f KeyPresenceFacts) KeyArrayTables(array constraint.PathKey) []constraint.PathKey {
	if f.bottom || array == "" || len(f.arrays) == 0 {
		return nil
	}
	var out []constraint.PathKey
	for _, fact := range f.arrays {
		if fact.Array == array {
			out = append(out, fact.Table)
		}
	}
	return out
}

func (f KeyPresenceFacts) KeyArrayEntries() []KeyArrayFact {
	if f.bottom || len(f.arrays) == 0 {
		return nil
	}
	return append([]KeyArrayFact(nil), f.arrays...)
}

// KillSubtree removes every presence, value-origin, or key-array fact whose
// table, key, value, or array path is root or a descendant of root.
func (f KeyPresenceFacts) KillSubtree(root constraint.PathKey) KeyPresenceFacts {
	if f.bottom || root == "" || (len(f.entries) == 0 && len(f.values) == 0 && len(f.arrays) == 0) {
		return f
	}
	entries := make([]KeyPresenceFact, 0, len(f.entries))
	for _, e := range f.entries {
		if keyPresencePathAffected(e.Table, root) || keyPresencePathAffected(e.Key, root) {
			continue
		}
		entries = append(entries, e)
	}
	values := make([]KeyValueFact, 0, len(f.values))
	for _, e := range f.values {
		if keyPresencePathAffected(e.Table, root) ||
			keyPresencePathAffected(e.Key, root) ||
			keyPresencePathAffected(e.Value, root) {
			continue
		}
		values = append(values, e)
	}
	arrays := make([]KeyArrayFact, 0, len(f.arrays))
	for _, e := range f.arrays {
		if keyPresencePathAffected(e.Array, root) ||
			keyPresencePathAffected(e.Table, root) {
			continue
		}
		arrays = append(arrays, e)
	}
	return canonicalKeyPresenceFactsFull(entries, values, arrays)
}

// KillAffectedByWrite removes facts that are no longer must-facts after a write
// to writePath. Unlike KillSubtree, table writes invalidate facts rooted above
// the written member too: a write to t.x can change the value of t[k] when k may
// be "x", so any table/key/value fact whose table overlaps the written path is
// dropped.
func (f KeyPresenceFacts) KillAffectedByWrite(writePath constraint.PathKey) KeyPresenceFacts {
	if f.bottom || writePath == "" || (len(f.entries) == 0 && len(f.values) == 0 && len(f.arrays) == 0) {
		return f
	}
	entries := make([]KeyPresenceFact, 0, len(f.entries))
	for _, e := range f.entries {
		if keyPresencePathsOverlap(e.Table, writePath) ||
			keyPresencePathsOverlap(e.Key, writePath) {
			continue
		}
		entries = append(entries, e)
	}
	values := make([]KeyValueFact, 0, len(f.values))
	for _, e := range f.values {
		if keyPresencePathsOverlap(e.Table, writePath) ||
			keyPresencePathsOverlap(e.Key, writePath) ||
			keyPresencePathsOverlap(e.Value, writePath) {
			continue
		}
		values = append(values, e)
	}
	arrays := make([]KeyArrayFact, 0, len(f.arrays))
	for _, e := range f.arrays {
		if keyPresencePathsOverlap(e.Array, writePath) ||
			keyPresencePathsOverlap(e.Table, writePath) {
			continue
		}
		arrays = append(arrays, e)
	}
	return canonicalKeyPresenceFactsFull(entries, values, arrays)
}

// KillAffectedByPresentElementWrite removes facts invalidated by a write of a
// definitely-present value to a table element or field. Such a write can replace
// value-specific readback facts, but it cannot make an already-present key of the
// table absent: if the written key aliases an existing proven key, the new
// non-nil value still leaves that key present.
func (f KeyPresenceFacts) KillAffectedByPresentElementWrite(writePath constraint.PathKey) KeyPresenceFacts {
	if f.bottom || writePath == "" || (len(f.entries) == 0 && len(f.values) == 0 && len(f.arrays) == 0) {
		return f
	}
	entries := make([]KeyPresenceFact, 0, len(f.entries))
	for _, e := range f.entries {
		if keyPresencePathsOverlap(e.Key, writePath) {
			continue
		}
		entries = append(entries, e)
	}
	values := make([]KeyValueFact, 0, len(f.values))
	for _, e := range f.values {
		if keyPresencePathsOverlap(e.Table, writePath) ||
			keyPresencePathsOverlap(e.Key, writePath) ||
			keyPresencePathsOverlap(e.Value, writePath) {
			continue
		}
		values = append(values, e)
	}
	arrays := make([]KeyArrayFact, 0, len(f.arrays))
	for _, e := range f.arrays {
		if keyPresencePathsOverlap(e.Array, writePath) {
			continue
		}
		arrays = append(arrays, e)
	}
	return canonicalKeyPresenceFactsFull(entries, values, arrays)
}

func (f KeyPresenceFacts) Format() string {
	if f.bottom {
		return "⊥"
	}
	if len(f.entries) == 0 && len(f.values) == 0 && len(f.arrays) == 0 {
		return "⊤"
	}
	parts := make([]string, 0, len(f.entries)+len(f.values)+len(f.arrays))
	for _, e := range f.entries {
		parts = append(parts, fmt.Sprintf("%s[%s]", e.Table, e.Key))
	}
	for _, e := range f.values {
		parts = append(parts, fmt.Sprintf("%s[%s]=%s", e.Table, e.Key, e.Value))
	}
	for _, e := range f.arrays {
		parts = append(parts, fmt.Sprintf("keys(%s)->%s", e.Array, e.Table))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

// KeyPresenceFactsDomain is the finite must-set lattice over KeyOf provenance.
// Join/Widen are intersection: a key-presence proof survives only when every
// incoming path proves the same table/key pair.
var KeyPresenceFactsDomain = lattice.Lattice[KeyPresenceFacts]{
	Bottom: func() KeyPresenceFacts {
		return KeyPresenceFacts{bottom: true}
	},
	Top: func() KeyPresenceFacts {
		return KeyPresenceFacts{}
	},
	Equal: func(a, b KeyPresenceFacts) bool {
		if a.bottom || b.bottom {
			return a.bottom == b.bottom
		}
		if len(a.entries) != len(b.entries) {
			return false
		}
		for i := range a.entries {
			if a.entries[i] != b.entries[i] {
				return false
			}
		}
		if len(a.values) != len(b.values) {
			return false
		}
		for i := range a.values {
			if a.values[i] != b.values[i] {
				return false
			}
		}
		if len(a.arrays) != len(b.arrays) {
			return false
		}
		for i := range a.arrays {
			if a.arrays[i] != b.arrays[i] {
				return false
			}
		}
		return true
	},
	LessOrEq: func(a, b KeyPresenceFacts) bool {
		if a.bottom {
			return true
		}
		if b.bottom {
			return false
		}
		return keyPresenceFactsContainAll(a.entries, b.entries) &&
			keyValueFactsContainAll(a.values, b.values) &&
			keyArrayFactsContainAll(a.arrays, b.arrays)
	},
	Join: func(a, b KeyPresenceFacts) KeyPresenceFacts {
		if a.bottom {
			return b
		}
		if b.bottom {
			return a
		}
		return intersectKeyPresenceFacts(a, b)
	},
	Meet: nil,
	Widen: func(prev, next KeyPresenceFacts) KeyPresenceFacts {
		if prev.bottom {
			return next
		}
		if next.bottom {
			return prev
		}
		return intersectKeyPresenceFacts(prev, next)
	},
}

func canonicalKeyPresenceFacts(entries []KeyPresenceFact, values ...[]KeyValueFact) KeyPresenceFacts {
	var rawValues []KeyValueFact
	if len(values) > 0 {
		rawValues = values[0]
	}
	return canonicalKeyPresenceFactsFull(entries, rawValues, nil)
}

func canonicalKeyPresenceFactsFull(entries []KeyPresenceFact, values []KeyValueFact, arrays []KeyArrayFact) KeyPresenceFacts {
	if len(entries) == 0 && len(values) == 0 && len(arrays) == 0 {
		return KeyPresenceFacts{}
	}
	out := append([]KeyPresenceFact(nil), entries...)
	sortKeyPresenceFacts(out)
	dst := out[:0]
	for _, e := range out {
		if e.Table == "" || e.Key == "" {
			continue
		}
		if len(dst) > 0 && dst[len(dst)-1] == e {
			continue
		}
		dst = append(dst, e)
	}
	valueOut := append([]KeyValueFact(nil), values...)
	sortKeyValueFacts(valueOut)
	valueDst := valueOut[:0]
	for _, e := range valueOut {
		if e.Table == "" || e.Key == "" || e.Value == "" {
			continue
		}
		if len(valueDst) > 0 && valueDst[len(valueDst)-1] == e {
			continue
		}
		valueDst = append(valueDst, e)
	}
	arrayOut := append([]KeyArrayFact(nil), arrays...)
	sortKeyArrayFacts(arrayOut)
	arrayDst := arrayOut[:0]
	for _, e := range arrayOut {
		if e.Array == "" || e.Table == "" {
			continue
		}
		if len(arrayDst) > 0 && arrayDst[len(arrayDst)-1] == e {
			continue
		}
		arrayDst = append(arrayDst, e)
	}
	return KeyPresenceFacts{
		entries: append([]KeyPresenceFact(nil), dst...),
		values:  append([]KeyValueFact(nil), valueDst...),
		arrays:  append([]KeyArrayFact(nil), arrayDst...),
	}
}

func sortKeyPresenceFacts(entries []KeyPresenceFact) {
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && keyPresenceLess(entries[j], entries[j-1]); j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}
}

func keyPresenceLess(a, b KeyPresenceFact) bool {
	if a.Table != b.Table {
		return a.Table < b.Table
	}
	return a.Key < b.Key
}

func sortKeyValueFacts(entries []KeyValueFact) {
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && keyValueLess(entries[j], entries[j-1]); j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}
}

func keyValueLess(a, b KeyValueFact) bool {
	if a.Table != b.Table {
		return a.Table < b.Table
	}
	if a.Key != b.Key {
		return a.Key < b.Key
	}
	return a.Value < b.Value
}

func sortKeyArrayFacts(entries []KeyArrayFact) {
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && keyArrayLess(entries[j], entries[j-1]); j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}
}

func keyArrayLess(a, b KeyArrayFact) bool {
	if a.Array != b.Array {
		return a.Array < b.Array
	}
	return a.Table < b.Table
}

func findKeyPresenceFact(entries []KeyPresenceFact, fact KeyPresenceFact) (int, bool) {
	lo, hi := 0, len(entries)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if keyPresenceLess(entries[mid], fact) {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo, lo < len(entries) && entries[lo] == fact
}

func findKeyValueFact(entries []KeyValueFact, fact KeyValueFact) (int, bool) {
	lo, hi := 0, len(entries)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if keyValueLess(entries[mid], fact) {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo, lo < len(entries) && entries[lo] == fact
}

func findKeyArrayFact(entries []KeyArrayFact, fact KeyArrayFact) (int, bool) {
	lo, hi := 0, len(entries)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if keyArrayLess(entries[mid], fact) {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo, lo < len(entries) && entries[lo] == fact
}

func keyPresenceFactsContainAll(have, want []KeyPresenceFact) bool {
	for _, w := range want {
		if _, ok := findKeyPresenceFact(have, w); !ok {
			return false
		}
	}
	return true
}

func keyValueFactsContainAll(have, want []KeyValueFact) bool {
	for _, w := range want {
		if _, ok := findKeyValueFact(have, w); !ok {
			return false
		}
	}
	return true
}

func keyArrayFactsContainAll(have, want []KeyArrayFact) bool {
	for _, w := range want {
		if _, ok := findKeyArrayFact(have, w); !ok {
			return false
		}
	}
	return true
}

func intersectKeyPresenceFacts(a, b KeyPresenceFacts) KeyPresenceFacts {
	var out []KeyPresenceFact
	i, j := 0, 0
	for i < len(a.entries) && j < len(b.entries) {
		switch {
		case keyPresenceLess(a.entries[i], b.entries[j]):
			i++
		case keyPresenceLess(b.entries[j], a.entries[i]):
			j++
		default:
			out = append(out, a.entries[i])
			i++
			j++
		}
	}
	var values []KeyValueFact
	i, j = 0, 0
	for i < len(a.values) && j < len(b.values) {
		switch {
		case keyValueLess(a.values[i], b.values[j]):
			i++
		case keyValueLess(b.values[j], a.values[i]):
			j++
		default:
			values = append(values, a.values[i])
			i++
			j++
		}
	}
	var arrays []KeyArrayFact
	i, j = 0, 0
	for i < len(a.arrays) && j < len(b.arrays) {
		switch {
		case keyArrayLess(a.arrays[i], b.arrays[j]):
			i++
		case keyArrayLess(b.arrays[j], a.arrays[i]):
			j++
		default:
			arrays = append(arrays, a.arrays[i])
			i++
			j++
		}
	}
	return canonicalKeyPresenceFactsFull(out, values, arrays)
}

func keyPresencePathAffected(path, root constraint.PathKey) bool {
	p := string(path)
	r := string(root)
	return p == r || strings.HasPrefix(p, r+".") || strings.HasPrefix(p, r+"[")
}

func keyPresencePathsOverlap(a, b constraint.PathKey) bool {
	return keyPresencePathAffected(a, b) || keyPresencePathAffected(b, a)
}
