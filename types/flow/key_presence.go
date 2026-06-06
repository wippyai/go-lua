package flow

import (
	"fmt"
	"strings"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
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

// EmptyKeyArrayFact is a point-local must-fact proving that Array is currently
// empty, therefore its elements are keys of every table by vacuity. It is kept
// separate from concrete KeyArrayFact entries so constructors do not enumerate a
// cross-product of empty arrays and sibling tables. Joins specialize it only when
// another predecessor has observed a concrete table-specific key-array fact.
type EmptyKeyArrayFact struct {
	Array constraint.PathKey
}

// KeyArrayValueFact is a point-local must-fact proving that every current
// element of Array is a key of Table and that Table[element] has Value. It is
// the value-carrying companion of KeyArrayFact, consumed by indexed iteration to
// materialize symbolic readback for the loop value key.
type KeyArrayValueFact struct {
	Array constraint.PathKey
	Table constraint.PathKey
	Value product.AbstractValue
}

// AppendedKeyFact is a point-local event fact proving that Key was appended to
// Array on the current path. It does not by itself prove Array is a key-array
// for any table; boundary projection and append reducers compose it with
// table/key proofs and caller pre-state.
type AppendedKeyFact struct {
	Array constraint.PathKey
	Key   constraint.PathKey
}

// PendingKeyArrayFact is a delayed reduced-product proof for append-before-write
// flows. It records that the current Array would be a key-array for Table once
// Key is proven present in that Table. An empty Table means the array was freshly
// formed by this append, so any later table proven to contain Key may materialize
// the key-array relation. Consumers never read this fact directly.
type PendingKeyArrayFact struct {
	Array constraint.PathKey
	Table constraint.PathKey
	Key   constraint.PathKey
}

// AppendHistoryBaseFact proves Array's current contents are tracked from an
// empty base through finite append events. It is a must fact: if any predecessor
// loses the tracked base, append-history coverage for that array is unusable.
type AppendHistoryBaseFact struct {
	Array constraint.PathKey
}

// AppendHistoryEventFact records one possible key appended to a tracked array
// since its empty base. It is a may-history fact: joins union possible append
// keys so coverage can prove every possible current element.
type AppendHistoryEventFact struct {
	Array constraint.PathKey
	Key   constraint.PathKey
}

// AppendHistoryCoverageFact proves that a tracked append event's key is present
// in Table with Value. An append-history coverage query succeeds only when every
// tracked append event for Array has a matching coverage fact for Table.
type AppendHistoryCoverageFact struct {
	Array constraint.PathKey
	Key   constraint.PathKey
	Table constraint.PathKey
	Value product.AbstractValue
}

// AppendElementFieldOriginFact records that a tracked append to Array may have
// sourced the element-relative Field from Source. It is consumed only by
// backward demand routing: a later demand for Array element field Field can be
// routed to every recorded Source while AppendHistoryBase still proves the array
// contents are tracked from an empty base.
type AppendElementFieldOriginFact struct {
	Array       constraint.PathKey
	Field       constraint.PathKey
	Source      constraint.PathKey
	SourceField constraint.PathKey
}

// AppendElementFieldOriginUse is a field-origin fact covering a demanded
// element field path. Remainder is the suffix under Origin.Field.
type AppendElementFieldOriginUse struct {
	Origin         AppendElementFieldOriginFact
	SourceField    []constraint.Segment
	FieldRemainder []constraint.Segment
}

// SourcePath returns the symbol-rooted source path carried by this origin use.
func (u AppendElementFieldOriginUse) SourcePath() (constraint.Path, bool) {
	addr, ok := StableAddressFromKey(u.Origin.Source)
	if !ok {
		return constraint.Path{}, false
	}
	return addr.Path()
}

// KeyPresenceFacts is a finite must-set lattice over KeyOf provenance. Bottom is
// unreachable, Top is the empty fact set, and finite states are sorted
// table/key, table/key/value, key-array, key-array/value, and delayed key-array
// facts. Join keeps only facts proven by every predecessor.
type KeyPresenceFacts struct {
	bottom         bool
	entries        []KeyPresenceFact
	values         []KeyValueFact
	arrays         []KeyArrayFact
	emptyArrays    []EmptyKeyArrayFact
	arrayValues    []KeyArrayValueFact
	appends        []AppendedKeyFact
	pending        []PendingKeyArrayFact
	appendBases    []AppendHistoryBaseFact
	appendEvents   []AppendHistoryEventFact
	appendCoverage []AppendHistoryCoverageFact
	appendOrigins  []AppendElementFieldOriginFact
}

type keyPresenceFactSet struct {
	entries        []KeyPresenceFact
	values         []KeyValueFact
	arrays         []KeyArrayFact
	emptyArrays    []EmptyKeyArrayFact
	arrayValues    []KeyArrayValueFact
	appends        []AppendedKeyFact
	pending        []PendingKeyArrayFact
	appendBases    []AppendHistoryBaseFact
	appendEvents   []AppendHistoryEventFact
	appendCoverage []AppendHistoryCoverageFact
	appendOrigins  []AppendElementFieldOriginFact
}

type keyPresenceFactFilter struct {
	entries        func(KeyPresenceFact) bool
	values         func(KeyValueFact) bool
	arrays         func(KeyArrayFact) bool
	emptyArrays    func(EmptyKeyArrayFact) bool
	arrayValues    func(KeyArrayValueFact) bool
	appends        func(AppendedKeyFact) bool
	pending        func(PendingKeyArrayFact) bool
	appendBases    func(AppendHistoryBaseFact) bool
	appendEvents   func(AppendHistoryEventFact) bool
	appendCoverage func(AppendHistoryCoverageFact) bool
	appendOrigins  func(AppendElementFieldOriginFact) bool
}

func filterKeyPresenceFacts[T any](facts []T, keep func(T) bool) []T {
	if len(facts) == 0 {
		return nil
	}
	if keep == nil {
		return facts
	}
	out := make([]T, 0, len(facts))
	for _, fact := range facts {
		if keep(fact) {
			out = append(out, fact)
		}
	}
	return out
}

func (set keyPresenceFactSet) filter(filter keyPresenceFactFilter) keyPresenceFactSet {
	return keyPresenceFactSet{
		entries:        filterKeyPresenceFacts(set.entries, filter.entries),
		values:         filterKeyPresenceFacts(set.values, filter.values),
		arrays:         filterKeyPresenceFacts(set.arrays, filter.arrays),
		emptyArrays:    filterKeyPresenceFacts(set.emptyArrays, filter.emptyArrays),
		arrayValues:    filterKeyPresenceFacts(set.arrayValues, filter.arrayValues),
		appends:        filterKeyPresenceFacts(set.appends, filter.appends),
		pending:        filterKeyPresenceFacts(set.pending, filter.pending),
		appendBases:    filterKeyPresenceFacts(set.appendBases, filter.appendBases),
		appendEvents:   filterKeyPresenceFacts(set.appendEvents, filter.appendEvents),
		appendCoverage: filterKeyPresenceFacts(set.appendCoverage, filter.appendCoverage),
		appendOrigins:  filterKeyPresenceFacts(set.appendOrigins, filter.appendOrigins),
	}
}

func keyPresenceFullAddressFilter(affected func(constraint.PathKey) bool) keyPresenceFactFilter {
	return keyPresenceFactFilter{
		entries: func(e KeyPresenceFact) bool {
			return !affected(e.Table) && !affected(e.Key)
		},
		values: func(e KeyValueFact) bool {
			return !affected(e.Table) && !affected(e.Key) && !affected(e.Value)
		},
		arrays: func(e KeyArrayFact) bool {
			return !affected(e.Array) && !affected(e.Table)
		},
		emptyArrays: func(e EmptyKeyArrayFact) bool {
			return !affected(e.Array)
		},
		arrayValues: func(e KeyArrayValueFact) bool {
			return !affected(e.Array) && !affected(e.Table)
		},
		appends: func(e AppendedKeyFact) bool {
			return !affected(e.Array) && !affected(e.Key)
		},
		pending: func(e PendingKeyArrayFact) bool {
			return !affected(e.Array) && !affected(e.Key) && (e.Table == "" || !affected(e.Table))
		},
		appendBases: func(e AppendHistoryBaseFact) bool {
			return !affected(e.Array)
		},
		appendEvents: func(e AppendHistoryEventFact) bool {
			return !affected(e.Array) && !affected(e.Key)
		},
		appendCoverage: func(e AppendHistoryCoverageFact) bool {
			return !affected(e.Array) && !affected(e.Key) && !affected(e.Table)
		},
		appendOrigins: func(e AppendElementFieldOriginFact) bool {
			return !affected(e.Array) && !affected(e.Source)
		},
	}
}

func keyPresencePresentElementAddressFilter(affected func(constraint.PathKey) bool) keyPresenceFactFilter {
	return keyPresenceFactFilter{
		entries: func(e KeyPresenceFact) bool {
			return !affected(e.Key)
		},
		values: func(e KeyValueFact) bool {
			return !affected(e.Table) && !affected(e.Key) && !affected(e.Value)
		},
		arrays: func(e KeyArrayFact) bool {
			return !affected(e.Array)
		},
		emptyArrays: func(e EmptyKeyArrayFact) bool {
			return !affected(e.Array)
		},
		arrayValues: func(e KeyArrayValueFact) bool {
			return !affected(e.Array)
		},
		appends: func(e AppendedKeyFact) bool {
			return !affected(e.Array) && !affected(e.Key)
		},
		pending: func(e PendingKeyArrayFact) bool {
			return !affected(e.Array) && !affected(e.Key)
		},
		appendBases: func(e AppendHistoryBaseFact) bool {
			return !affected(e.Array)
		},
		appendEvents: func(e AppendHistoryEventFact) bool {
			return !affected(e.Array) && !affected(e.Key)
		},
		appendCoverage: func(e AppendHistoryCoverageFact) bool {
			return !affected(e.Array) && !affected(e.Key)
		},
		appendOrigins: func(e AppendElementFieldOriginFact) bool {
			return !affected(e.Array) && !affected(e.Source)
		},
	}
}

func (set keyPresenceFactSet) isEmpty() bool {
	return len(set.entries) == 0 && len(set.values) == 0 && len(set.arrays) == 0 &&
		len(set.emptyArrays) == 0 && len(set.arrayValues) == 0 && len(set.appends) == 0 &&
		len(set.pending) == 0 && len(set.appendBases) == 0 && len(set.appendEvents) == 0 &&
		len(set.appendCoverage) == 0 && len(set.appendOrigins) == 0
}

func (set keyPresenceFactSet) canonical() KeyPresenceFacts {
	return canonicalKeyPresenceFactSet(set)
}

func (f KeyPresenceFacts) factSet() keyPresenceFactSet {
	if f.bottom {
		return keyPresenceFactSet{}
	}
	return keyPresenceFactSet{
		entries:        f.entries,
		values:         f.values,
		arrays:         f.arrays,
		emptyArrays:    f.emptyArrays,
		arrayValues:    f.arrayValues,
		appends:        f.appends,
		pending:        f.pending,
		appendBases:    f.appendBases,
		appendEvents:   f.appendEvents,
		appendCoverage: f.appendCoverage,
		appendOrigins:  f.appendOrigins,
	}
}

func (f KeyPresenceFacts) hasFacts() bool {
	return !f.factSet().isEmpty()
}

// KeyPresenceFactsOf constructs a canonical finite fact set.
func KeyPresenceFactsOf(entries []KeyPresenceFact) KeyPresenceFacts {
	return canonicalKeyPresenceFacts(entries)
}

// KeyPresencePathKey returns the typed structural key used by key-presence facts.
// Symbol paths use the stable symbol/segment key; root-only fallback is accepted
// only for boundary paths that have not been symbolized.
func KeyPresencePathKey(path constraint.Path) constraint.PathKey {
	return StablePathKey(path)
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

func (f KeyPresenceFacts) HasAddresses(table, key StableAddress) bool {
	return f.Has(table.Key(), key.Key())
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
	set := f.factSet()
	set.entries = next
	return set.canonical()
}

func (f KeyPresenceFacts) WithAddresses(table, key StableAddress) KeyPresenceFacts {
	return f.With(table.Key(), key.Key())
}

func (f KeyPresenceFacts) HasValue(table, key, value constraint.PathKey) bool {
	if f.bottom || table == "" || key == "" || value == "" || len(f.values) == 0 {
		return false
	}
	_, ok := findKeyValueFact(f.values, KeyValueFact{Table: table, Key: key, Value: value})
	return ok
}

func (f KeyPresenceFacts) HasValueAddresses(table, key, value StableAddress) bool {
	return f.HasValue(table.Key(), key.Key(), value.Key())
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
	set := f.factSet()
	set.values = next
	return set.canonical()
}

func (f KeyPresenceFacts) WithValueAddresses(table, key, value StableAddress) KeyPresenceFacts {
	return f.WithValue(table.Key(), key.Key(), value.Key())
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
	set := f.factSet()
	set.arrays = next
	return set.canonical()
}

func (f KeyPresenceFacts) WithKeyArrayAddresses(array, table StableAddress) KeyPresenceFacts {
	return f.WithKeyArray(array.Key(), table.Key())
}

func (f KeyPresenceFacts) KeyArrayTables(array constraint.PathKey) []constraint.PathKey {
	if f.bottom || array == "" {
		return nil
	}
	var out []constraint.PathKey
	for _, fact := range f.arrays {
		if fact.Array == array {
			out = append(out, fact.Table)
		}
	}
	for _, table := range f.appendHistoryCoveredTables(array) {
		if _, ok := findPathKeyLinear(out, table); !ok {
			out = append(out, table)
		}
	}
	return out
}

func (f KeyPresenceFacts) KeyArrayEntries() []KeyArrayFact {
	if f.bottom {
		return nil
	}
	out := append([]KeyArrayFact(nil), f.arrays...)
	for _, base := range f.appendBases {
		for _, table := range f.appendHistoryCoveredTables(base.Array) {
			fact := KeyArrayFact{Array: base.Array, Table: table}
			if _, ok := findKeyArrayFact(out, fact); !ok {
				out = append(out, fact)
			}
		}
	}
	sortKeyArrayFacts(out)
	return out
}

func (f KeyPresenceFacts) WithEmptyKeyArray(array constraint.PathKey) KeyPresenceFacts {
	if array == "" {
		return f
	}
	if f.bottom {
		f = KeyPresenceFacts{}
	}
	next := f.EmptyKeyArrayEntries()
	fact := EmptyKeyArrayFact{Array: array}
	if _, ok := findEmptyKeyArrayFact(next, fact); !ok {
		next = append(next, fact)
	}
	base := f.AppendHistoryBaseEntries()
	if _, ok := findAppendHistoryBaseFact(base, AppendHistoryBaseFact{Array: array}); !ok {
		base = append(base, AppendHistoryBaseFact{Array: array})
	}
	set := f.factSet()
	set.emptyArrays = next
	set.appendBases = base
	return set.canonical()
}

func (f KeyPresenceFacts) WithEmptyKeyArrayAddress(array StableAddress) KeyPresenceFacts {
	return f.WithEmptyKeyArray(array.Key())
}

func (f KeyPresenceFacts) HasEmptyKeyArray(array constraint.PathKey) bool {
	if f.bottom || array == "" || len(f.emptyArrays) == 0 {
		return false
	}
	_, ok := findEmptyKeyArrayFact(f.emptyArrays, EmptyKeyArrayFact{Array: array})
	return ok
}

func (f KeyPresenceFacts) EmptyKeyArrayEntries() []EmptyKeyArrayFact {
	if f.bottom || len(f.emptyArrays) == 0 {
		return nil
	}
	return append([]EmptyKeyArrayFact(nil), f.emptyArrays...)
}

func (f KeyPresenceFacts) WithKeyArrayValue(array, table constraint.PathKey, value product.AbstractValue) KeyPresenceFacts {
	if array == "" || table == "" || value.IsZero() {
		return f
	}
	if f.bottom {
		f = KeyPresenceFacts{}
	}
	f = f.WithKeyArray(array, table)
	next := f.KeyArrayValueEntries()
	fact := KeyArrayValueFact{Array: array, Table: table, Value: value}
	next = append(next, fact)
	set := f.factSet()
	set.arrayValues = next
	return set.canonical()
}

func (f KeyPresenceFacts) WithKeyArrayValueAddresses(array, table StableAddress, value product.AbstractValue) KeyPresenceFacts {
	return f.WithKeyArrayValue(array.Key(), table.Key(), value)
}

func (f KeyPresenceFacts) KeyArrayValueEntries() []KeyArrayValueFact {
	if f.bottom {
		return nil
	}
	out := append([]KeyArrayValueFact(nil), f.arrayValues...)
	for _, base := range f.appendBases {
		for _, table := range f.appendHistoryCoveredTables(base.Array) {
			value, ok := f.AppendHistoryCoverageValue(base.Array, table)
			if !ok || value.IsZero() {
				continue
			}
			fact := KeyArrayValueFact{Array: base.Array, Table: table, Value: value}
			if idx, ok := findKeyArrayValueFact(out, fact); ok {
				out[idx].Value = product.Domain.Join(out[idx].Value, value)
				continue
			}
			out = append(out, fact)
		}
	}
	sortKeyArrayValueFacts(out)
	return out
}

func (f KeyPresenceFacts) WithAppendedKey(array, key constraint.PathKey) KeyPresenceFacts {
	if array == "" || key == "" {
		return f
	}
	if f.bottom {
		f = KeyPresenceFacts{}
	}
	next := f.AppendedKeyEntries()
	fact := AppendedKeyFact{Array: array, Key: key}
	if _, ok := findAppendedKeyFact(next, fact); !ok {
		next = append(next, fact)
	}
	set := f.factSet()
	set.appends = next
	return set.canonical()
}

func (f KeyPresenceFacts) WithAppendedKeyAddresses(array, key StableAddress) KeyPresenceFacts {
	return f.WithAppendedKey(array.Key(), key.Key())
}

func (f KeyPresenceFacts) AppendedKeyEntries() []AppendedKeyFact {
	if f.bottom || len(f.appends) == 0 {
		return nil
	}
	return append([]AppendedKeyFact(nil), f.appends...)
}

func (f KeyPresenceFacts) WithPendingKeyArray(array, table, key constraint.PathKey) KeyPresenceFacts {
	if array == "" || key == "" {
		return f
	}
	if f.bottom {
		f = KeyPresenceFacts{}
	}
	next := f.PendingKeyArrayEntries()
	fact := PendingKeyArrayFact{Array: array, Table: table, Key: key}
	if _, ok := findPendingKeyArrayFact(next, fact); !ok {
		next = append(next, fact)
	}
	set := f.factSet()
	set.pending = next
	return set.canonical()
}

func (f KeyPresenceFacts) PendingKeyArrayEntries() []PendingKeyArrayFact {
	if f.bottom || len(f.pending) == 0 {
		return nil
	}
	return append([]PendingKeyArrayFact(nil), f.pending...)
}

func (f KeyPresenceFacts) PendingKeyArraysFor(table, key constraint.PathKey) []constraint.PathKey {
	if f.bottom || key == "" || len(f.pending) == 0 {
		return nil
	}
	var out []constraint.PathKey
	for _, fact := range f.pending {
		if fact.Key != key {
			continue
		}
		if fact.Table != "" && fact.Table != table {
			continue
		}
		out = append(out, fact.Array)
	}
	return out
}

func (f KeyPresenceFacts) WithAppendHistoryBase(array constraint.PathKey) KeyPresenceFacts {
	if array == "" {
		return f
	}
	if f.bottom {
		f = KeyPresenceFacts{}
	}
	next := f.AppendHistoryBaseEntries()
	fact := AppendHistoryBaseFact{Array: array}
	if _, ok := findAppendHistoryBaseFact(next, fact); !ok {
		next = append(next, fact)
	}
	set := f.factSet()
	set.appendBases = next
	return set.canonical()
}

func (f KeyPresenceFacts) WithAppendHistoryBaseAddress(array StableAddress) KeyPresenceFacts {
	return f.WithAppendHistoryBase(array.Key())
}

func (f KeyPresenceFacts) HasAppendHistoryBase(array constraint.PathKey) bool {
	if f.bottom || array == "" || len(f.appendBases) == 0 {
		return false
	}
	_, ok := findAppendHistoryBaseFact(f.appendBases, AppendHistoryBaseFact{Array: array})
	return ok
}

func (f KeyPresenceFacts) AppendHistoryBaseEntries() []AppendHistoryBaseFact {
	if f.bottom || len(f.appendBases) == 0 {
		return nil
	}
	return append([]AppendHistoryBaseFact(nil), f.appendBases...)
}

func (f KeyPresenceFacts) WithAppendHistoryEvent(array, key constraint.PathKey) KeyPresenceFacts {
	if array == "" || key == "" || !f.HasAppendHistoryBase(array) {
		return f
	}
	next := f.AppendHistoryEventEntries()
	fact := AppendHistoryEventFact{Array: array, Key: key}
	if _, ok := findAppendHistoryEventFact(next, fact); !ok {
		next = append(next, fact)
	}
	set := f.factSet()
	set.appendEvents = next
	return set.canonical()
}

func (f KeyPresenceFacts) AppendHistoryEventEntries() []AppendHistoryEventFact {
	if f.bottom || len(f.appendEvents) == 0 {
		return nil
	}
	return append([]AppendHistoryEventFact(nil), f.appendEvents...)
}

func (f KeyPresenceFacts) WithAppendHistoryCoverage(array, key, table constraint.PathKey, value product.AbstractValue) KeyPresenceFacts {
	if array == "" || key == "" || table == "" || value.IsZero() || !f.HasAppendHistoryBase(array) {
		return f
	}
	f = f.WithAppendHistoryEvent(array, key)
	next := f.AppendHistoryCoverageEntries()
	next = append(next, AppendHistoryCoverageFact{Array: array, Key: key, Table: table, Value: value})
	set := f.factSet()
	set.appendCoverage = next
	return set.canonical()
}

func (f KeyPresenceFacts) AppendHistoryCoverageEntries() []AppendHistoryCoverageFact {
	if f.bottom || len(f.appendCoverage) == 0 {
		return nil
	}
	return append([]AppendHistoryCoverageFact(nil), f.appendCoverage...)
}

const appendElementFieldRoot = "__element"

func AppendElementFieldPathKey(segments []constraint.Segment) constraint.PathKey {
	if len(segments) == 0 {
		return ""
	}
	return constraint.Path{
		Root:     appendElementFieldRoot,
		Segments: append([]constraint.Segment(nil), segments...),
	}.Key()
}

func AppendElementFieldSegments(key constraint.PathKey) ([]constraint.Segment, bool) {
	addr, ok := StableAddressFromKey(key)
	if !ok {
		return nil, false
	}
	root, ok := addr.Root()
	if !ok || root != appendElementFieldRoot {
		return nil, false
	}
	segs := addr.Segments()
	return segs, len(segs) > 0
}

func (f KeyPresenceFacts) WithAppendElementFieldOrigin(array, field, source constraint.PathKey) KeyPresenceFacts {
	return f.WithAppendElementFieldOriginFromSource(array, field, source, "")
}

func (f KeyPresenceFacts) WithAppendElementFieldOriginFromSource(array, field, source, sourceField constraint.PathKey) KeyPresenceFacts {
	if array == "" || field == "" || source == "" || !f.HasAppendHistoryBase(array) {
		return f
	}
	if f.bottom {
		f = KeyPresenceFacts{}
	}
	next := f.AppendElementFieldOriginEntries()
	fact := AppendElementFieldOriginFact{Array: array, Field: field, Source: source, SourceField: sourceField}
	if _, ok := findAppendElementFieldOriginFact(next, fact); !ok {
		next = append(next, fact)
	}
	set := f.factSet()
	set.appendOrigins = next
	return set.canonical()
}

func (f KeyPresenceFacts) WithAppendElementFieldOriginAddresses(array StableAddress, field []constraint.Segment, source StableAddress) KeyPresenceFacts {
	return f.WithAppendElementFieldOriginFromSource(array.Key(), AppendElementFieldPathKey(field), source.Key(), "")
}

func (f KeyPresenceFacts) WithAppendElementFieldOriginFromAddresses(array StableAddress, field []constraint.Segment, source StableAddress, sourceField []constraint.Segment) KeyPresenceFacts {
	return f.WithAppendElementFieldOriginFromSource(array.Key(), AppendElementFieldPathKey(field), source.Key(), AppendElementFieldPathKey(sourceField))
}

func (f KeyPresenceFacts) AppendElementFieldOriginEntries() []AppendElementFieldOriginFact {
	if f.bottom || len(f.appendOrigins) == 0 {
		return nil
	}
	return append([]AppendElementFieldOriginFact(nil), f.appendOrigins...)
}

func (f KeyPresenceFacts) AppendElementFieldSources(array constraint.PathKey, field []constraint.Segment) []AppendElementFieldOriginUse {
	fieldKey := AppendElementFieldPathKey(field)
	if f.bottom || array == "" || fieldKey == "" || !f.HasAppendHistoryBase(array) || len(f.appendOrigins) == 0 {
		return nil
	}
	fieldSegs, ok := AppendElementFieldSegments(fieldKey)
	if !ok {
		return nil
	}
	var out []AppendElementFieldOriginUse
	for _, fact := range f.appendOrigins {
		if fact.Array != array {
			continue
		}
		originSegs, ok := AppendElementFieldSegments(fact.Field)
		if !ok || len(originSegs) > len(fieldSegs) || !segmentsPrefix(originSegs, fieldSegs) {
			continue
		}
		remainder := append([]constraint.Segment(nil), fieldSegs[len(originSegs):]...)
		sourceField, _ := AppendElementFieldSegments(fact.SourceField)
		out = append(out, AppendElementFieldOriginUse{
			Origin:         fact,
			SourceField:    append([]constraint.Segment(nil), sourceField...),
			FieldRemainder: append([]constraint.Segment(nil), remainder...),
		})
	}
	return out
}

func (f KeyPresenceFacts) KeyArrayValues(array, table constraint.PathKey) []product.AbstractValue {
	if f.bottom || array == "" || table == "" {
		return nil
	}
	var out product.AbstractValue
	for _, fact := range f.arrayValues {
		if fact.Array == array && fact.Table == table {
			if out.IsZero() {
				out = fact.Value
			} else {
				out = product.Domain.Join(out, fact.Value)
			}
		}
	}
	if value, ok := f.AppendHistoryCoverageValue(array, table); ok {
		if out.IsZero() {
			out = value
		} else {
			out = product.Domain.Join(out, value)
		}
	}
	if out.IsZero() {
		return nil
	}
	return []product.AbstractValue{out}
}

func (f KeyPresenceFacts) AppendHistoryCoverageValue(array, table constraint.PathKey) (product.AbstractValue, bool) {
	if f.bottom || array == "" || table == "" || !f.HasAppendHistoryBase(array) {
		return product.AbstractValue{}, false
	}
	events := f.appendHistoryEventsFor(array)
	if len(events) == 0 {
		return product.AbstractValue{}, false
	}
	var out product.AbstractValue
	for _, event := range events {
		value, ok := f.appendHistoryCoverageFor(array, event.Key, table)
		if !ok || value.IsZero() {
			return product.AbstractValue{}, false
		}
		if out.IsZero() {
			out = value
		} else {
			out = product.Domain.Join(out, value)
		}
	}
	return out, !out.IsZero()
}

func (f KeyPresenceFacts) appendHistoryEventsFor(array constraint.PathKey) []AppendHistoryEventFact {
	var out []AppendHistoryEventFact
	for _, event := range f.appendEvents {
		if event.Array == array {
			out = append(out, event)
		}
	}
	return out
}

func (f KeyPresenceFacts) appendHistoryCoverageFor(array, key, table constraint.PathKey) (product.AbstractValue, bool) {
	var out product.AbstractValue
	for _, coverage := range f.appendCoverage {
		if coverage.Array != array || coverage.Key != key || coverage.Table != table || coverage.Value.IsZero() {
			continue
		}
		if out.IsZero() {
			out = coverage.Value
		} else {
			out = product.Domain.Join(out, coverage.Value)
		}
	}
	return out, !out.IsZero()
}

func (f KeyPresenceFacts) appendHistoryCoveredTables(array constraint.PathKey) []constraint.PathKey {
	if f.bottom || array == "" || !f.HasAppendHistoryBase(array) {
		return nil
	}
	events := f.appendHistoryEventsFor(array)
	if len(events) == 0 {
		return nil
	}
	var candidates []constraint.PathKey
	for _, coverage := range f.appendCoverage {
		if coverage.Array != array || coverage.Table == "" {
			continue
		}
		if _, ok := findPathKeyLinear(candidates, coverage.Table); !ok {
			candidates = append(candidates, coverage.Table)
		}
	}
	var out []constraint.PathKey
	for _, table := range candidates {
		if _, ok := f.AppendHistoryCoverageValue(array, table); ok {
			out = append(out, table)
		}
	}
	return out
}

func findPathKeyLinear(xs []constraint.PathKey, want constraint.PathKey) (int, bool) {
	for i, x := range xs {
		if x == want {
			return i, true
		}
	}
	return -1, false
}

// KillSubtreeAddress removes every presence, value-origin, or key-array fact
// whose table, key, value, or array path is root or a descendant of root.
func (f KeyPresenceFacts) KillSubtreeAddress(root StableAddress) KeyPresenceFacts {
	if f.bottom || root.Key() == "" || !f.hasFacts() {
		return f
	}
	affected := func(key constraint.PathKey) bool {
		return keyPresencePathAffectedByAddress(key, root)
	}
	return f.factSet().filter(keyPresenceFullAddressFilter(affected)).canonical()
}

// KillAffectedByWriteAddress removes facts that are no longer must-facts after
// a write to write.
func (f KeyPresenceFacts) KillAffectedByWriteAddress(write StableAddress) KeyPresenceFacts {
	if f.bottom || write.Key() == "" || !f.hasFacts() {
		return f
	}
	overlaps := func(key constraint.PathKey) bool {
		return keyPresencePathOverlapsAddress(key, write)
	}
	return f.factSet().filter(keyPresenceFullAddressFilter(overlaps)).canonical()
}

// KillAffectedByPresentElementWriteAddress removes facts invalidated by a write
// of a definitely-present value to a table element or field.
func (f KeyPresenceFacts) KillAffectedByPresentElementWriteAddress(write StableAddress) KeyPresenceFacts {
	if f.bottom || write.Key() == "" || !f.hasFacts() {
		return f
	}
	overlaps := func(key constraint.PathKey) bool {
		return keyPresencePathOverlapsAddress(key, write)
	}
	return f.factSet().filter(keyPresencePresentElementAddressFilter(overlaps)).canonical()
}

// KillAffectedByPresentElementMemberWriteAddress invalidates value-specific
// proofs for a write below an already-present array element, such as
// a[i].field = v. The write can stale facts about the element member, but it
// does not change the array's key set or append history.
func (f KeyPresenceFacts) KillAffectedByPresentElementMemberWriteAddress(array StableAddress, member []constraint.Segment) KeyPresenceFacts {
	killed := f.KillAffectedByPresentElementWriteAddress(array)
	arrayKey := array.Key()
	if f.bottom || arrayKey == "" {
		return killed
	}
	return preservePresentElementMemberWriteFacts(killed, f, arrayKey, member)
}

func preservePresentElementMemberWriteFacts(killed, original KeyPresenceFacts, arrayKey constraint.PathKey, member []constraint.Segment) KeyPresenceFacts {
	return keyPresenceFactSet{
		entries:        killed.Entries(),
		values:         killed.ValueEntries(),
		arrays:         append(killed.KeyArrayEntries(), keyArrayFactsForArray(original.arrays, arrayKey)...),
		emptyArrays:    append(killed.EmptyKeyArrayEntries(), emptyKeyArrayFactsForArray(original.emptyArrays, arrayKey)...),
		arrayValues:    append(killed.KeyArrayValueEntries(), keyArrayValueFactsForArray(original.arrayValues, arrayKey)...),
		appends:        append(killed.AppendedKeyEntries(), appendedKeyFactsForArray(original.appends, arrayKey)...),
		pending:        append(killed.PendingKeyArrayEntries(), pendingKeyArrayFactsForArray(original.pending, arrayKey)...),
		appendBases:    append(killed.AppendHistoryBaseEntries(), appendHistoryBaseFactsForArray(original.appendBases, arrayKey)...),
		appendEvents:   append(killed.AppendHistoryEventEntries(), appendHistoryEventFactsForArray(original.appendEvents, arrayKey)...),
		appendCoverage: append(killed.AppendHistoryCoverageEntries(), appendHistoryCoverageFactsForArray(original.appendCoverage, arrayKey)...),
		appendOrigins:  append(killed.AppendElementFieldOriginEntries(), appendElementFieldOriginFactsPreservedByMemberWrite(original.appendOrigins, arrayKey, member)...),
	}.canonical()
}

func (f KeyPresenceFacts) Format() string {
	if f.bottom {
		return "⊥"
	}
	if len(f.entries) == 0 && len(f.values) == 0 && len(f.arrays) == 0 &&
		len(f.emptyArrays) == 0 && len(f.arrayValues) == 0 && len(f.appends) == 0 &&
		len(f.pending) == 0 && len(f.appendBases) == 0 && len(f.appendEvents) == 0 &&
		len(f.appendCoverage) == 0 && len(f.appendOrigins) == 0 {
		return "⊤"
	}
	parts := make([]string, 0,
		len(f.entries)+len(f.values)+len(f.arrays)+len(f.emptyArrays)+len(f.arrayValues)+
			len(f.appends)+len(f.pending)+len(f.appendBases)+len(f.appendEvents)+
			len(f.appendCoverage)+len(f.appendOrigins))
	for _, e := range f.entries {
		parts = append(parts, fmt.Sprintf("%s[%s]", e.Table, e.Key))
	}
	for _, e := range f.values {
		parts = append(parts, fmt.Sprintf("%s[%s]=%s", e.Table, e.Key, e.Value))
	}
	for _, e := range f.arrays {
		parts = append(parts, fmt.Sprintf("keys(%s)->%s", e.Array, e.Table))
	}
	for _, e := range f.emptyArrays {
		parts = append(parts, fmt.Sprintf("keys(%s)->*", e.Array))
	}
	for _, e := range f.arrayValues {
		parts = append(parts, fmt.Sprintf("keys(%s)->%s=%s", e.Array, e.Table, e.Value.ProjectValue()))
	}
	for _, e := range f.appends {
		parts = append(parts, fmt.Sprintf("append_key(%s,%s)", e.Array, e.Key))
	}
	for _, e := range f.pending {
		table := e.Table
		if table == "" {
			table = "*"
		}
		parts = append(parts, fmt.Sprintf("pending_keys(%s,%s)->%s", e.Array, e.Key, table))
	}
	for _, e := range f.appendBases {
		parts = append(parts, fmt.Sprintf("append_base(%s)", e.Array))
	}
	for _, e := range f.appendEvents {
		parts = append(parts, fmt.Sprintf("append_event(%s,%s)", e.Array, e.Key))
	}
	for _, e := range f.appendCoverage {
		parts = append(parts, fmt.Sprintf("append_cover(%s,%s)->%s=%s", e.Array, e.Key, e.Table, e.Value.ProjectValue()))
	}
	for _, e := range f.appendOrigins {
		parts = append(parts, fmt.Sprintf("append_origin(%s,%s)->%s", e.Array, e.Field, e.Source))
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
		if len(a.emptyArrays) != len(b.emptyArrays) {
			return false
		}
		for i := range a.emptyArrays {
			if a.emptyArrays[i] != b.emptyArrays[i] {
				return false
			}
		}
		if len(a.arrayValues) != len(b.arrayValues) {
			return false
		}
		for i := range a.arrayValues {
			if a.arrayValues[i].Array != b.arrayValues[i].Array ||
				a.arrayValues[i].Table != b.arrayValues[i].Table ||
				!product.Domain.Equal(a.arrayValues[i].Value, b.arrayValues[i].Value) {
				return false
			}
		}
		if len(a.appends) != len(b.appends) {
			return false
		}
		for i := range a.appends {
			if a.appends[i] != b.appends[i] {
				return false
			}
		}
		if len(a.pending) != len(b.pending) {
			return false
		}
		for i := range a.pending {
			if a.pending[i] != b.pending[i] {
				return false
			}
		}
		if len(a.appendBases) != len(b.appendBases) {
			return false
		}
		for i := range a.appendBases {
			if a.appendBases[i] != b.appendBases[i] {
				return false
			}
		}
		if len(a.appendEvents) != len(b.appendEvents) {
			return false
		}
		for i := range a.appendEvents {
			if a.appendEvents[i] != b.appendEvents[i] {
				return false
			}
		}
		if len(a.appendCoverage) != len(b.appendCoverage) {
			return false
		}
		for i := range a.appendCoverage {
			if a.appendCoverage[i].Array != b.appendCoverage[i].Array ||
				a.appendCoverage[i].Key != b.appendCoverage[i].Key ||
				a.appendCoverage[i].Table != b.appendCoverage[i].Table ||
				!product.Domain.Equal(a.appendCoverage[i].Value, b.appendCoverage[i].Value) {
				return false
			}
		}
		if len(a.appendOrigins) != len(b.appendOrigins) {
			return false
		}
		for i := range a.appendOrigins {
			if a.appendOrigins[i] != b.appendOrigins[i] {
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
			keyArrayFactsContainAllWithEmpty(a.arrays, a.emptyArrays, b.arrays) &&
			emptyKeyArrayFactsContainAll(a.emptyArrays, b.emptyArrays) &&
			keyArrayValueFactsContainAllWithEmpty(a.arrayValues, a.emptyArrays, b.arrayValues) &&
			appendedKeyFactsContainAll(a.appends, b.appends) &&
			pendingKeyArrayFactsContainAll(a.pending, b.pending) &&
			appendHistoryBaseFactsContainAllWithEmpty(a.appendBases, a.emptyArrays, b.appendBases) &&
			appendHistoryEventFactsContainAll(b.appendEvents, a.appendEvents) &&
			appendHistoryCoverageFactsContainAll(b.appendCoverage, a.appendCoverage) &&
			appendElementFieldOriginFactsContainAll(b.appendOrigins, a.appendOrigins)
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
		return intersectKeyPresenceFactsWiden(prev, next)
	},
}

func canonicalKeyPresenceFacts(entries []KeyPresenceFact, values ...[]KeyValueFact) KeyPresenceFacts {
	var rawValues []KeyValueFact
	if len(values) > 0 {
		rawValues = values[0]
	}
	return keyPresenceFactSet{
		entries: entries,
		values:  rawValues,
	}.canonical()
}

func canonicalKeyPresenceFactSet(set keyPresenceFactSet) KeyPresenceFacts {
	if set.isEmpty() {
		return KeyPresenceFacts{}
	}
	out := append([]KeyPresenceFact(nil), set.entries...)
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
	valueOut := append([]KeyValueFact(nil), set.values...)
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
	arrayOut := append([]KeyArrayFact(nil), set.arrays...)
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
	emptyOut := append([]EmptyKeyArrayFact(nil), set.emptyArrays...)
	sortEmptyKeyArrayFacts(emptyOut)
	emptyDst := emptyOut[:0]
	for _, e := range emptyOut {
		if e.Array == "" {
			continue
		}
		if len(emptyDst) > 0 && emptyDst[len(emptyDst)-1] == e {
			continue
		}
		emptyDst = append(emptyDst, e)
	}
	arrayValueOut := append([]KeyArrayValueFact(nil), set.arrayValues...)
	sortKeyArrayValueFacts(arrayValueOut)
	arrayValueDst := arrayValueOut[:0]
	for _, e := range arrayValueOut {
		if e.Array == "" || e.Table == "" || e.Value.IsZero() {
			continue
		}
		if len(arrayValueDst) > 0 &&
			arrayValueDst[len(arrayValueDst)-1].Array == e.Array &&
			arrayValueDst[len(arrayValueDst)-1].Table == e.Table {
			arrayValueDst[len(arrayValueDst)-1].Value = product.Domain.Join(arrayValueDst[len(arrayValueDst)-1].Value, e.Value)
			continue
		}
		arrayValueDst = append(arrayValueDst, e)
	}
	appendOut := append([]AppendedKeyFact(nil), set.appends...)
	sortAppendedKeyFacts(appendOut)
	appendDst := appendOut[:0]
	for _, e := range appendOut {
		if e.Array == "" || e.Key == "" {
			continue
		}
		if len(appendDst) > 0 && appendDst[len(appendDst)-1] == e {
			continue
		}
		appendDst = append(appendDst, e)
	}
	pendingOut := append([]PendingKeyArrayFact(nil), set.pending...)
	sortPendingKeyArrayFacts(pendingOut)
	pendingDst := pendingOut[:0]
	for _, e := range pendingOut {
		if e.Array == "" || e.Key == "" {
			continue
		}
		if len(pendingDst) > 0 && pendingDst[len(pendingDst)-1] == e {
			continue
		}
		pendingDst = append(pendingDst, e)
	}
	appendBaseOut := append([]AppendHistoryBaseFact(nil), set.appendBases...)
	sortAppendHistoryBaseFacts(appendBaseOut)
	appendBaseDst := appendBaseOut[:0]
	for _, e := range appendBaseOut {
		if e.Array == "" {
			continue
		}
		if len(appendBaseDst) > 0 && appendBaseDst[len(appendBaseDst)-1] == e {
			continue
		}
		appendBaseDst = append(appendBaseDst, e)
	}
	appendEventOut := append([]AppendHistoryEventFact(nil), set.appendEvents...)
	sortAppendHistoryEventFacts(appendEventOut)
	appendEventDst := appendEventOut[:0]
	for _, e := range appendEventOut {
		if e.Array == "" || e.Key == "" {
			continue
		}
		if _, ok := findAppendHistoryBaseFact(appendBaseDst, AppendHistoryBaseFact{Array: e.Array}); !ok {
			continue
		}
		if len(appendEventDst) > 0 && appendEventDst[len(appendEventDst)-1] == e {
			continue
		}
		appendEventDst = append(appendEventDst, e)
	}
	appendCoverageOut := append([]AppendHistoryCoverageFact(nil), set.appendCoverage...)
	sortAppendHistoryCoverageFacts(appendCoverageOut)
	appendCoverageDst := appendCoverageOut[:0]
	for _, e := range appendCoverageOut {
		if e.Array == "" || e.Key == "" || e.Table == "" || e.Value.IsZero() {
			continue
		}
		if _, ok := findAppendHistoryBaseFact(appendBaseDst, AppendHistoryBaseFact{Array: e.Array}); !ok {
			continue
		}
		if _, ok := findAppendHistoryEventFact(appendEventDst, AppendHistoryEventFact{Array: e.Array, Key: e.Key}); !ok {
			continue
		}
		if len(appendCoverageDst) > 0 &&
			appendCoverageDst[len(appendCoverageDst)-1].Array == e.Array &&
			appendCoverageDst[len(appendCoverageDst)-1].Key == e.Key &&
			appendCoverageDst[len(appendCoverageDst)-1].Table == e.Table {
			appendCoverageDst[len(appendCoverageDst)-1].Value = product.Domain.Join(appendCoverageDst[len(appendCoverageDst)-1].Value, e.Value)
			continue
		}
		appendCoverageDst = append(appendCoverageDst, e)
	}
	appendOriginOut := append([]AppendElementFieldOriginFact(nil), set.appendOrigins...)
	sortAppendElementFieldOriginFacts(appendOriginOut)
	appendOriginDst := appendOriginOut[:0]
	for _, e := range appendOriginOut {
		if e.Array == "" || e.Field == "" || e.Source == "" {
			continue
		}
		if e.SourceField != "" {
			if _, ok := AppendElementFieldSegments(e.SourceField); !ok {
				continue
			}
		}
		if _, ok := findAppendHistoryBaseFact(appendBaseDst, AppendHistoryBaseFact{Array: e.Array}); !ok {
			continue
		}
		if _, ok := AppendElementFieldSegments(e.Field); !ok {
			continue
		}
		if len(appendOriginDst) > 0 && appendOriginDst[len(appendOriginDst)-1] == e {
			continue
		}
		appendOriginDst = append(appendOriginDst, e)
	}
	return KeyPresenceFacts{
		entries:        append([]KeyPresenceFact(nil), dst...),
		values:         append([]KeyValueFact(nil), valueDst...),
		arrays:         append([]KeyArrayFact(nil), arrayDst...),
		emptyArrays:    append([]EmptyKeyArrayFact(nil), emptyDst...),
		arrayValues:    append([]KeyArrayValueFact(nil), arrayValueDst...),
		appends:        append([]AppendedKeyFact(nil), appendDst...),
		pending:        append([]PendingKeyArrayFact(nil), pendingDst...),
		appendBases:    append([]AppendHistoryBaseFact(nil), appendBaseDst...),
		appendEvents:   append([]AppendHistoryEventFact(nil), appendEventDst...),
		appendCoverage: append([]AppendHistoryCoverageFact(nil), appendCoverageDst...),
		appendOrigins:  append([]AppendElementFieldOriginFact(nil), appendOriginDst...),
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

func sortEmptyKeyArrayFacts(entries []EmptyKeyArrayFact) {
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && emptyKeyArrayLess(entries[j], entries[j-1]); j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}
}

func emptyKeyArrayLess(a, b EmptyKeyArrayFact) bool {
	return a.Array < b.Array
}

func sortKeyArrayValueFacts(entries []KeyArrayValueFact) {
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && keyArrayValueLess(entries[j], entries[j-1]); j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}
}

func keyArrayValueLess(a, b KeyArrayValueFact) bool {
	if a.Array != b.Array {
		return a.Array < b.Array
	}
	return a.Table < b.Table
}

func sortAppendedKeyFacts(entries []AppendedKeyFact) {
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && appendedKeyLess(entries[j], entries[j-1]); j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}
}

func appendedKeyLess(a, b AppendedKeyFact) bool {
	if a.Array != b.Array {
		return a.Array < b.Array
	}
	return a.Key < b.Key
}

func sortPendingKeyArrayFacts(entries []PendingKeyArrayFact) {
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && pendingKeyArrayLess(entries[j], entries[j-1]); j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}
}

func pendingKeyArrayLess(a, b PendingKeyArrayFact) bool {
	if a.Array != b.Array {
		return a.Array < b.Array
	}
	if a.Key != b.Key {
		return a.Key < b.Key
	}
	return a.Table < b.Table
}

func sortAppendHistoryBaseFacts(entries []AppendHistoryBaseFact) {
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && appendHistoryBaseLess(entries[j], entries[j-1]); j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}
}

func appendHistoryBaseLess(a, b AppendHistoryBaseFact) bool {
	return a.Array < b.Array
}

func sortAppendHistoryEventFacts(entries []AppendHistoryEventFact) {
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && appendHistoryEventLess(entries[j], entries[j-1]); j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}
}

func appendHistoryEventLess(a, b AppendHistoryEventFact) bool {
	if a.Array != b.Array {
		return a.Array < b.Array
	}
	return a.Key < b.Key
}

func sortAppendHistoryCoverageFacts(entries []AppendHistoryCoverageFact) {
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && appendHistoryCoverageLess(entries[j], entries[j-1]); j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}
}

func appendHistoryCoverageLess(a, b AppendHistoryCoverageFact) bool {
	if a.Array != b.Array {
		return a.Array < b.Array
	}
	if a.Key != b.Key {
		return a.Key < b.Key
	}
	return a.Table < b.Table
}

func sortAppendElementFieldOriginFacts(entries []AppendElementFieldOriginFact) {
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && appendElementFieldOriginLess(entries[j], entries[j-1]); j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}
}

func appendElementFieldOriginLess(a, b AppendElementFieldOriginFact) bool {
	if a.Array != b.Array {
		return a.Array < b.Array
	}
	if a.Field != b.Field {
		return a.Field < b.Field
	}
	if a.Source != b.Source {
		return a.Source < b.Source
	}
	return a.SourceField < b.SourceField
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

func findEmptyKeyArrayFact(entries []EmptyKeyArrayFact, fact EmptyKeyArrayFact) (int, bool) {
	lo, hi := 0, len(entries)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if emptyKeyArrayLess(entries[mid], fact) {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo, lo < len(entries) && entries[lo] == fact
}

func findKeyArrayValueFact(entries []KeyArrayValueFact, fact KeyArrayValueFact) (int, bool) {
	lo, hi := 0, len(entries)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if keyArrayValueLess(entries[mid], fact) {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo, lo < len(entries) &&
		entries[lo].Array == fact.Array &&
		entries[lo].Table == fact.Table
}

func findAppendedKeyFact(entries []AppendedKeyFact, fact AppendedKeyFact) (int, bool) {
	lo, hi := 0, len(entries)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if appendedKeyLess(entries[mid], fact) {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo, lo < len(entries) && entries[lo] == fact
}

func findPendingKeyArrayFact(entries []PendingKeyArrayFact, fact PendingKeyArrayFact) (int, bool) {
	lo, hi := 0, len(entries)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if pendingKeyArrayLess(entries[mid], fact) {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo, lo < len(entries) && entries[lo] == fact
}

func findAppendHistoryBaseFact(entries []AppendHistoryBaseFact, fact AppendHistoryBaseFact) (int, bool) {
	lo, hi := 0, len(entries)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if appendHistoryBaseLess(entries[mid], fact) {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo, lo < len(entries) && entries[lo] == fact
}

func findAppendHistoryEventFact(entries []AppendHistoryEventFact, fact AppendHistoryEventFact) (int, bool) {
	lo, hi := 0, len(entries)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if appendHistoryEventLess(entries[mid], fact) {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo, lo < len(entries) && entries[lo] == fact
}

func findAppendHistoryCoverageFact(entries []AppendHistoryCoverageFact, fact AppendHistoryCoverageFact) (int, bool) {
	lo, hi := 0, len(entries)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if appendHistoryCoverageLess(entries[mid], fact) {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo, lo < len(entries) &&
		entries[lo].Array == fact.Array &&
		entries[lo].Key == fact.Key &&
		entries[lo].Table == fact.Table
}

func findAppendElementFieldOriginFact(entries []AppendElementFieldOriginFact, fact AppendElementFieldOriginFact) (int, bool) {
	lo, hi := 0, len(entries)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if appendElementFieldOriginLess(entries[mid], fact) {
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

func keyArrayFactsContainAllWithEmpty(have []KeyArrayFact, empty []EmptyKeyArrayFact, want []KeyArrayFact) bool {
	for _, w := range want {
		if _, ok := findKeyArrayFact(have, w); ok {
			continue
		}
		if _, ok := findEmptyKeyArrayFact(empty, EmptyKeyArrayFact{Array: w.Array}); ok {
			continue
		}
		return false
	}
	return true
}

func emptyKeyArrayFactsContainAll(have, want []EmptyKeyArrayFact) bool {
	for _, w := range want {
		if _, ok := findEmptyKeyArrayFact(have, w); !ok {
			return false
		}
	}
	return true
}

func keyArrayValueFactsContainAll(have, want []KeyArrayValueFact) bool {
	for _, w := range want {
		i, ok := findKeyArrayValueFact(have, w)
		if !ok || !product.Domain.LessOrEq(have[i].Value, w.Value) {
			return false
		}
	}
	return true
}

func keyArrayValueFactsContainAllWithEmpty(have []KeyArrayValueFact, empty []EmptyKeyArrayFact, want []KeyArrayValueFact) bool {
	for _, w := range want {
		i, ok := findKeyArrayValueFact(have, w)
		if ok && product.Domain.LessOrEq(have[i].Value, w.Value) {
			continue
		}
		if _, ok := findEmptyKeyArrayFact(empty, EmptyKeyArrayFact{Array: w.Array}); ok {
			continue
		}
		return false
	}
	return true
}

func appendedKeyFactsContainAll(have, want []AppendedKeyFact) bool {
	for _, w := range want {
		if _, ok := findAppendedKeyFact(have, w); !ok {
			return false
		}
	}
	return true
}

func pendingKeyArrayFactsContainAll(have, want []PendingKeyArrayFact) bool {
	for _, w := range want {
		if _, ok := findPendingKeyArrayFact(have, w); !ok {
			return false
		}
	}
	return true
}

func appendHistoryBaseFactsContainAllWithEmpty(have []AppendHistoryBaseFact, empty []EmptyKeyArrayFact, want []AppendHistoryBaseFact) bool {
	for _, w := range want {
		if _, ok := findAppendHistoryBaseFact(have, w); ok {
			continue
		}
		if _, ok := findEmptyKeyArrayFact(empty, EmptyKeyArrayFact{Array: w.Array}); ok {
			continue
		}
		return false
	}
	return true
}

func appendHistoryEventFactsContainAll(have, want []AppendHistoryEventFact) bool {
	for _, w := range want {
		if _, ok := findAppendHistoryEventFact(have, w); !ok {
			return false
		}
	}
	return true
}

func appendHistoryCoverageFactsContainAll(have, want []AppendHistoryCoverageFact) bool {
	for _, w := range want {
		i, ok := findAppendHistoryCoverageFact(have, w)
		if !ok || !product.Domain.LessOrEq(have[i].Value, w.Value) {
			return false
		}
	}
	return true
}

func appendElementFieldOriginFactsContainAll(have, want []AppendElementFieldOriginFact) bool {
	for _, w := range want {
		if _, ok := findAppendElementFieldOriginFact(have, w); !ok {
			return false
		}
	}
	return true
}

func intersectKeyPresenceFacts(a, b KeyPresenceFacts) KeyPresenceFacts {
	return intersectKeyPresenceFactsWithPayload(a, b, false)
}

func intersectKeyPresenceFactsWiden(prev, next KeyPresenceFacts) KeyPresenceFacts {
	return intersectKeyPresenceFactsWithPayload(prev, next, true)
}

func intersectKeyPresenceFactsWithPayload(a, b KeyPresenceFacts, widenPayload bool) KeyPresenceFacts {
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
	arrays = append(arrays, keyArrayFactsSpecializedByEmpty(a.emptyArrays, b.arrays)...)
	arrays = append(arrays, keyArrayFactsSpecializedByEmpty(b.emptyArrays, a.arrays)...)
	var emptyArrays []EmptyKeyArrayFact
	i, j = 0, 0
	for i < len(a.emptyArrays) && j < len(b.emptyArrays) {
		switch {
		case emptyKeyArrayLess(a.emptyArrays[i], b.emptyArrays[j]):
			i++
		case emptyKeyArrayLess(b.emptyArrays[j], a.emptyArrays[i]):
			j++
		default:
			emptyArrays = append(emptyArrays, a.emptyArrays[i])
			i++
			j++
		}
	}
	var arrayValues []KeyArrayValueFact
	i, j = 0, 0
	for i < len(a.arrayValues) && j < len(b.arrayValues) {
		switch {
		case keyArrayValueLess(a.arrayValues[i], b.arrayValues[j]):
			i++
		case keyArrayValueLess(b.arrayValues[j], a.arrayValues[i]):
			j++
		default:
			value := product.Domain.Join(a.arrayValues[i].Value, b.arrayValues[j].Value)
			if widenPayload {
				value = product.Domain.Widen(a.arrayValues[i].Value, b.arrayValues[j].Value)
			}
			arrayValues = append(arrayValues, KeyArrayValueFact{
				Array: a.arrayValues[i].Array,
				Table: a.arrayValues[i].Table,
				Value: value,
			})
			i++
			j++
		}
	}
	arrayValues = append(arrayValues, keyArrayValueFactsSpecializedByEmpty(a.emptyArrays, b.arrayValues)...)
	arrayValues = append(arrayValues, keyArrayValueFactsSpecializedByEmpty(b.emptyArrays, a.arrayValues)...)
	var pending []PendingKeyArrayFact
	var appends []AppendedKeyFact
	i, j = 0, 0
	for i < len(a.appends) && j < len(b.appends) {
		switch {
		case appendedKeyLess(a.appends[i], b.appends[j]):
			i++
		case appendedKeyLess(b.appends[j], a.appends[i]):
			j++
		default:
			appends = append(appends, a.appends[i])
			i++
			j++
		}
	}
	i, j = 0, 0
	for i < len(a.pending) && j < len(b.pending) {
		switch {
		case pendingKeyArrayLess(a.pending[i], b.pending[j]):
			i++
		case pendingKeyArrayLess(b.pending[j], a.pending[i]):
			j++
		default:
			pending = append(pending, a.pending[i])
			i++
			j++
		}
	}
	var appendBases []AppendHistoryBaseFact
	i, j = 0, 0
	for i < len(a.appendBases) && j < len(b.appendBases) {
		switch {
		case appendHistoryBaseLess(a.appendBases[i], b.appendBases[j]):
			i++
		case appendHistoryBaseLess(b.appendBases[j], a.appendBases[i]):
			j++
		default:
			appendBases = append(appendBases, a.appendBases[i])
			i++
			j++
		}
	}
	appendBases = append(appendBases, appendHistoryBasesSpecializedByEmpty(a.emptyArrays, b.appendBases)...)
	appendBases = append(appendBases, appendHistoryBasesSpecializedByEmpty(b.emptyArrays, a.appendBases)...)
	appendBases = compactAppendHistoryBases(appendBases)
	appendEvents := appendHistoryEventsForBases(append(a.AppendHistoryEventEntries(), b.AppendHistoryEventEntries()...), appendBases)
	appendCoverage := appendHistoryCoverageForBases(append(a.AppendHistoryCoverageEntries(), b.AppendHistoryCoverageEntries()...), appendBases, appendEvents, widenPayload)
	appendOrigins := appendElementFieldOriginsForBases(append(a.AppendElementFieldOriginEntries(), b.AppendElementFieldOriginEntries()...), appendBases)
	return keyPresenceFactSet{
		entries:        out,
		values:         values,
		arrays:         arrays,
		emptyArrays:    emptyArrays,
		arrayValues:    arrayValues,
		appends:        appends,
		pending:        pending,
		appendBases:    appendBases,
		appendEvents:   appendEvents,
		appendCoverage: appendCoverage,
		appendOrigins:  appendOrigins,
	}.canonical()
}

func keyArrayFactsSpecializedByEmpty(empty []EmptyKeyArrayFact, concrete []KeyArrayFact) []KeyArrayFact {
	if len(empty) == 0 || len(concrete) == 0 {
		return nil
	}
	var out []KeyArrayFact
	for _, fact := range concrete {
		if _, ok := findEmptyKeyArrayFact(empty, EmptyKeyArrayFact{Array: fact.Array}); ok {
			out = append(out, fact)
		}
	}
	return out
}

func keyArrayValueFactsSpecializedByEmpty(empty []EmptyKeyArrayFact, concrete []KeyArrayValueFact) []KeyArrayValueFact {
	if len(empty) == 0 || len(concrete) == 0 {
		return nil
	}
	var out []KeyArrayValueFact
	for _, fact := range concrete {
		if _, ok := findEmptyKeyArrayFact(empty, EmptyKeyArrayFact{Array: fact.Array}); ok {
			out = append(out, fact)
		}
	}
	return out
}

func appendHistoryBasesSpecializedByEmpty(empty []EmptyKeyArrayFact, concrete []AppendHistoryBaseFact) []AppendHistoryBaseFact {
	if len(empty) == 0 || len(concrete) == 0 {
		return nil
	}
	var out []AppendHistoryBaseFact
	for _, fact := range concrete {
		if _, ok := findEmptyKeyArrayFact(empty, EmptyKeyArrayFact{Array: fact.Array}); ok {
			out = append(out, fact)
		}
	}
	return out
}

func compactAppendHistoryBases(entries []AppendHistoryBaseFact) []AppendHistoryBaseFact {
	if len(entries) == 0 {
		return nil
	}
	sortAppendHistoryBaseFacts(entries)
	out := entries[:0]
	for _, entry := range entries {
		if entry.Array == "" {
			continue
		}
		if len(out) > 0 && out[len(out)-1] == entry {
			continue
		}
		out = append(out, entry)
	}
	return append([]AppendHistoryBaseFact(nil), out...)
}

func appendHistoryEventsForBases(events []AppendHistoryEventFact, bases []AppendHistoryBaseFact) []AppendHistoryEventFact {
	if len(events) == 0 || len(bases) == 0 {
		return nil
	}
	sortAppendHistoryEventFacts(events)
	var out []AppendHistoryEventFact
	for _, event := range events {
		if event.Array == "" || event.Key == "" {
			continue
		}
		if _, ok := findAppendHistoryBaseFact(bases, AppendHistoryBaseFact{Array: event.Array}); !ok {
			continue
		}
		if len(out) > 0 && out[len(out)-1] == event {
			continue
		}
		out = append(out, event)
	}
	return append([]AppendHistoryEventFact(nil), out...)
}

func appendHistoryCoverageForBases(
	coverage []AppendHistoryCoverageFact,
	bases []AppendHistoryBaseFact,
	events []AppendHistoryEventFact,
	widenPayload bool,
) []AppendHistoryCoverageFact {
	if len(coverage) == 0 || len(bases) == 0 || len(events) == 0 {
		return nil
	}
	sortAppendHistoryCoverageFacts(coverage)
	var out []AppendHistoryCoverageFact
	for _, fact := range coverage {
		if fact.Array == "" || fact.Key == "" || fact.Table == "" || fact.Value.IsZero() {
			continue
		}
		if _, ok := findAppendHistoryBaseFact(bases, AppendHistoryBaseFact{Array: fact.Array}); !ok {
			continue
		}
		if _, ok := findAppendHistoryEventFact(events, AppendHistoryEventFact{Array: fact.Array, Key: fact.Key}); !ok {
			continue
		}
		if len(out) > 0 &&
			out[len(out)-1].Array == fact.Array &&
			out[len(out)-1].Key == fact.Key &&
			out[len(out)-1].Table == fact.Table {
			if widenPayload {
				out[len(out)-1].Value = product.Domain.Widen(out[len(out)-1].Value, fact.Value)
			} else {
				out[len(out)-1].Value = product.Domain.Join(out[len(out)-1].Value, fact.Value)
			}
			continue
		}
		out = append(out, fact)
	}
	return append([]AppendHistoryCoverageFact(nil), out...)
}

func appendElementFieldOriginsForBases(
	origins []AppendElementFieldOriginFact,
	bases []AppendHistoryBaseFact,
) []AppendElementFieldOriginFact {
	if len(origins) == 0 || len(bases) == 0 {
		return nil
	}
	sortAppendElementFieldOriginFacts(origins)
	var out []AppendElementFieldOriginFact
	for _, origin := range origins {
		if origin.Array == "" || origin.Field == "" || origin.Source == "" {
			continue
		}
		if origin.SourceField != "" {
			if _, ok := AppendElementFieldSegments(origin.SourceField); !ok {
				continue
			}
		}
		if _, ok := findAppendHistoryBaseFact(bases, AppendHistoryBaseFact{Array: origin.Array}); !ok {
			continue
		}
		if _, ok := AppendElementFieldSegments(origin.Field); !ok {
			continue
		}
		if len(out) > 0 && out[len(out)-1] == origin {
			continue
		}
		out = append(out, origin)
	}
	return append([]AppendElementFieldOriginFact(nil), out...)
}

func keyPresencePathAffectedByAddress(path constraint.PathKey, root StableAddress) bool {
	pathAddr, ok := StableAddressFromKey(path)
	return ok && pathAddr.HasPrefix(root)
}

func keyPresencePathOverlapsAddress(path constraint.PathKey, addr StableAddress) bool {
	pathAddr, ok := StableAddressFromKey(path)
	return ok && pathAddr.Overlaps(addr)
}

func keyArrayFactsForArray(facts []KeyArrayFact, array constraint.PathKey) []KeyArrayFact {
	var out []KeyArrayFact
	for _, fact := range facts {
		if fact.Array == array {
			out = append(out, fact)
		}
	}
	return out
}

func keyArrayValueFactsForArray(facts []KeyArrayValueFact, array constraint.PathKey) []KeyArrayValueFact {
	var out []KeyArrayValueFact
	for _, fact := range facts {
		if fact.Array == array {
			out = append(out, fact)
		}
	}
	return out
}

func pendingKeyArrayFactsForArray(facts []PendingKeyArrayFact, array constraint.PathKey) []PendingKeyArrayFact {
	var out []PendingKeyArrayFact
	for _, fact := range facts {
		if fact.Array == array {
			out = append(out, fact)
		}
	}
	return out
}

func emptyKeyArrayFactsForArray(facts []EmptyKeyArrayFact, array constraint.PathKey) []EmptyKeyArrayFact {
	var out []EmptyKeyArrayFact
	for _, fact := range facts {
		if fact.Array == array {
			out = append(out, fact)
		}
	}
	return out
}

func appendedKeyFactsForArray(facts []AppendedKeyFact, array constraint.PathKey) []AppendedKeyFact {
	var out []AppendedKeyFact
	for _, fact := range facts {
		if fact.Array == array {
			out = append(out, fact)
		}
	}
	return out
}

func appendHistoryBaseFactsForArray(facts []AppendHistoryBaseFact, array constraint.PathKey) []AppendHistoryBaseFact {
	var out []AppendHistoryBaseFact
	for _, fact := range facts {
		if fact.Array == array {
			out = append(out, fact)
		}
	}
	return out
}

func appendHistoryEventFactsForArray(facts []AppendHistoryEventFact, array constraint.PathKey) []AppendHistoryEventFact {
	var out []AppendHistoryEventFact
	for _, fact := range facts {
		if fact.Array == array {
			out = append(out, fact)
		}
	}
	return out
}

func appendHistoryCoverageFactsForArray(facts []AppendHistoryCoverageFact, array constraint.PathKey) []AppendHistoryCoverageFact {
	var out []AppendHistoryCoverageFact
	for _, fact := range facts {
		if fact.Array == array {
			out = append(out, fact)
		}
	}
	return out
}

func appendElementFieldOriginFactsPreservedByMemberWrite(
	facts []AppendElementFieldOriginFact,
	array constraint.PathKey,
	member []constraint.Segment,
) []AppendElementFieldOriginFact {
	var out []AppendElementFieldOriginFact
	for _, fact := range facts {
		if fact.Array != array {
			continue
		}
		if appendElementFieldOriginOverlapsMember(fact, member) {
			continue
		}
		out = append(out, fact)
	}
	return out
}

func appendElementFieldOriginOverlapsMember(fact AppendElementFieldOriginFact, member []constraint.Segment) bool {
	field, ok := AppendElementFieldSegments(fact.Field)
	if !ok || len(member) == 0 {
		return true
	}
	return segmentsOverlap(field, member)
}

func segmentsOverlap(a, b []constraint.Segment) bool {
	if len(a) > len(b) {
		a, b = b, a
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
