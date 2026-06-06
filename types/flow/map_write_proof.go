package flow

import (
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

// KeyPresenceProof is the canonical proof transaction for "key is a key of
// table". It owns the cross-reduction from direct key presence into delayed
// key-array facts.
type KeyPresenceProof struct {
	Table        StableAddress
	Key          StableAddress
	ValuePath    StableAddress
	HasValuePath bool
	Value        product.AbstractValue
}

// KeyPresenceAliasProof copies key-presence facts from SourceKey to TargetKey.
// It is used for local alias assignments where the target value denotes the
// same dynamic map key as the source.
type KeyPresenceAliasProof struct {
	SourceKey StableAddress
	TargetKey StableAddress
}

// KeyArrayElementKeyProof consumes key-array provenance for Array by proving
// TargetKey is present in every table whose keys are carried by Array.
type KeyArrayElementKeyProof struct {
	Array     StableAddress
	TargetKey StableAddress
	KeyValue  product.AbstractValue
}

// KeyArrayElementKeyResult reports the tables reached by a key-array element
// proof so callers can apply non-flow refinements.
type KeyArrayElementKeyResult struct {
	Tables []StableAddress
}

// KeyArrayProof is the canonical proof transaction for "array contains keys of
// table". It is separate from direct key presence because the array fact is a
// quantified provenance statement, not one table/key membership.
type KeyArrayProof struct {
	Array StableAddress
	Table StableAddress
}

// EmptyKeyArrayProof records that Array is known empty and can later be used as
// a key-array seed when keys are appended.
type EmptyKeyArrayProof struct {
	Array StableAddress
}

// KeyArrayValueProof is the value-carrying form of key-array provenance. When
// AppendKey is present it also records the append-history coverage that proves
// the appended element is backed by the same table value.
type KeyArrayValueProof struct {
	Array        StableAddress
	Table        StableAddress
	Value        product.AbstractValue
	AppendKey    StableAddress
	HasAppendKey bool
}

// PendingKeyArrayProof records that Array may become a key-array for Table
// after Key is proven present. Table is optional because some empty-array seeds
// intentionally wait for any matching table/key presence.
type PendingKeyArrayProof struct {
	Array    StableAddress
	Key      StableAddress
	Table    StableAddress
	HasTable bool
}

// AppendKeyProof records that Key was appended into Array.
type AppendKeyProof struct {
	Array StableAddress
	Key   StableAddress
}

// AppendHistoryBaseProof preserves append-history tracking for Array across a
// mutation that otherwise invalidates array element facts.
type AppendHistoryBaseProof struct {
	Array StableAddress
}

// AppendElementFieldOriginProof records that appended elements in Array carry a
// field from Source. Field and SourceField are structured suffixes, not encoded
// fact keys.
type AppendElementFieldOriginProof struct {
	Array       StableAddress
	Field       []constraint.Segment
	Source      StableAddress
	SourceField []constraint.Segment
}

// ApplyKeyPresenceProof applies a key-presence proof to point state. When Value
// is non-zero it also records value-carrying key-array consequences.
func ApplyKeyPresenceProof(out *PointState, proof KeyPresenceProof) bool {
	if out == nil {
		return false
	}
	tableKey := proof.Table.Key()
	keyKey := proof.Key.Key()
	if tableKey == "" || keyKey == "" {
		return false
	}
	before := out.KeyPresence
	out.KeyPresence = out.KeyPresence.With(tableKey, keyKey)
	if proof.HasValuePath {
		valuePath := proof.ValuePath.Key()
		if valuePath != "" {
			out.KeyPresence = out.KeyPresence.WithValue(tableKey, keyKey, valuePath)
		}
	}
	for _, array := range out.KeyPresence.PendingKeyArraysFor(tableKey, keyKey) {
		out.KeyPresence = out.KeyPresence.WithKeyArray(array, tableKey)
		if !proof.Value.IsZero() {
			out.KeyPresence = out.KeyPresence.WithKeyArrayValue(array, tableKey, proof.Value)
		}
	}
	return !KeyPresenceFactsDomain.Equal(before, out.KeyPresence)
}

// ApplyKeyPresenceAliasProof applies key-presence facts proven for SourceKey to
// TargetKey. Value-path facts are preserved with the same table/value path.
func ApplyKeyPresenceAliasProof(out *PointState, proof KeyPresenceAliasProof) bool {
	if out == nil || proof.SourceKey.Key() == "" || proof.TargetKey.Key() == "" {
		return false
	}
	sourceKey := proof.SourceKey.Key()
	targetKey := proof.TargetKey.Key()
	before := out.KeyPresence
	for _, entry := range out.KeyPresence.Entries() {
		if entry.Key != sourceKey {
			continue
		}
		out.KeyPresence = out.KeyPresence.With(entry.Table, targetKey)
	}
	for _, entry := range out.KeyPresence.ValueEntries() {
		if entry.Key != sourceKey {
			continue
		}
		out.KeyPresence = out.KeyPresence.WithValue(entry.Table, targetKey, entry.Value)
	}
	return !KeyPresenceFactsDomain.Equal(before, out.KeyPresence)
}

// ApplyKeyArrayElementKeyProof applies key-array membership to a target key. If
// value-carrying key-array facts exist, it also publishes table[target] readback
// admissions for the assigned target key.
func ApplyKeyArrayElementKeyProof(out *PointState, proof KeyArrayElementKeyProof) (KeyArrayElementKeyResult, bool) {
	if out == nil || proof.Array.Key() == "" || proof.TargetKey.Key() == "" {
		return KeyArrayElementKeyResult{}, false
	}
	result := KeyArrayElementKeyResult{}
	arrayKey := proof.Array.Key()
	targetKey := proof.TargetKey.Key()
	keyValue := proof.KeyValue
	if keyValue.IsZero() {
		keyValue = product.FromType(typ.Unknown)
	}
	beforePresence := out.KeyPresence
	beforeIndexWrites := out.IndexWrites
	tables := out.KeyPresence.KeyArrayTables(arrayKey)
	for _, tableKey := range tables {
		table, ok := StableAddressFromKey(tableKey)
		if !ok {
			continue
		}
		result.Tables = append(result.Tables, table)
		out.KeyPresence = out.KeyPresence.With(tableKey, targetKey)
		for _, value := range out.KeyPresence.KeyArrayValues(arrayKey, tableKey) {
			if value.IsZero() {
				continue
			}
			out.IndexWrites = out.IndexWrites.WithAddress(IndexWriteAdmissionAddressFact{
				Target:     table,
				KeyPath:    proof.TargetKey,
				HasKeyPath: true,
				Key:        keyValue,
				Value:      value,
			})
		}
	}
	changed := !KeyPresenceFactsDomain.Equal(beforePresence, out.KeyPresence)
	changed = !IndexWriteAdmissionFactsDomain.Equal(beforeIndexWrites, out.IndexWrites) || changed
	return result, changed
}

// ApplyKeyArrayProof applies a key-array provenance proof to point state.
func ApplyKeyArrayProof(out *PointState, proof KeyArrayProof) bool {
	if out == nil || proof.Array.Key() == "" || proof.Table.Key() == "" {
		return false
	}
	before := out.KeyPresence
	out.KeyPresence = out.KeyPresence.WithKeyArrayAddresses(proof.Array, proof.Table)
	return !KeyPresenceFactsDomain.Equal(before, out.KeyPresence)
}

// ApplyEmptyKeyArrayProof applies empty key-array provenance to point state.
func ApplyEmptyKeyArrayProof(out *PointState, proof EmptyKeyArrayProof) bool {
	if out == nil || proof.Array.Key() == "" {
		return false
	}
	before := out.KeyPresence
	out.KeyPresence = out.KeyPresence.WithEmptyKeyArrayAddress(proof.Array)
	return !KeyPresenceFactsDomain.Equal(before, out.KeyPresence)
}

// ApplyKeyArrayValueProof applies a value-carrying key-array proof to point state.
func ApplyKeyArrayValueProof(out *PointState, proof KeyArrayValueProof) bool {
	if out == nil || proof.Array.Key() == "" || proof.Table.Key() == "" || proof.Value.IsZero() {
		return false
	}
	before := out.KeyPresence
	out.KeyPresence = out.KeyPresence.WithKeyArrayValueAddresses(proof.Array, proof.Table, proof.Value)
	if proof.HasAppendKey {
		out.KeyPresence = out.KeyPresence.WithAppendHistoryCoverage(proof.Array.Key(), proof.AppendKey.Key(), proof.Table.Key(), proof.Value)
	}
	return !KeyPresenceFactsDomain.Equal(before, out.KeyPresence)
}

// ApplyPendingKeyArrayProof applies delayed key-array provenance to point state.
func ApplyPendingKeyArrayProof(out *PointState, proof PendingKeyArrayProof) bool {
	if out == nil || proof.Array.Key() == "" || proof.Key.Key() == "" {
		return false
	}
	tableKey := constraint.PathKey("")
	if proof.HasTable {
		tableKey = proof.Table.Key()
		if tableKey == "" {
			return false
		}
	}
	before := out.KeyPresence
	out.KeyPresence = out.KeyPresence.WithPendingKeyArray(proof.Array.Key(), tableKey, proof.Key.Key())
	return !KeyPresenceFactsDomain.Equal(before, out.KeyPresence)
}

// ApplyAppendKeyProof applies append-key provenance to point state.
func ApplyAppendKeyProof(out *PointState, proof AppendKeyProof) bool {
	if out == nil || proof.Array.Key() == "" || proof.Key.Key() == "" {
		return false
	}
	before := out.KeyPresence
	out.KeyPresence = out.KeyPresence.WithAppendedKeyAddresses(proof.Array, proof.Key)
	out.KeyPresence = out.KeyPresence.WithAppendHistoryEvent(proof.Array.Key(), proof.Key.Key())
	return !KeyPresenceFactsDomain.Equal(before, out.KeyPresence)
}

// ApplyAppendHistoryBaseProof applies append-history base tracking to point state.
func ApplyAppendHistoryBaseProof(out *PointState, proof AppendHistoryBaseProof) bool {
	if out == nil || proof.Array.Key() == "" {
		return false
	}
	before := out.KeyPresence
	out.KeyPresence = out.KeyPresence.WithAppendHistoryBaseAddress(proof.Array)
	return !KeyPresenceFactsDomain.Equal(before, out.KeyPresence)
}

// ApplyAppendElementFieldOriginProof applies an append element-field origin
// proof to point state.
func ApplyAppendElementFieldOriginProof(out *PointState, proof AppendElementFieldOriginProof) bool {
	if out == nil || proof.Array.Key() == "" || proof.Source.Key() == "" || len(proof.Field) == 0 {
		return false
	}
	before := out.KeyPresence
	out.KeyPresence = out.KeyPresence.
		WithAppendHistoryBaseAddress(proof.Array).
		WithAppendElementFieldOriginFromAddresses(proof.Array, proof.Field, proof.Source, proof.SourceField)
	return !KeyPresenceFactsDomain.Equal(before, out.KeyPresence)
}

// IndexedKeyArrayIterationProof consumes key-array provenance at an indexed
// iteration value binding. Its consequences are direct table/key presence and
// value-carrying readback admissions for table[key].
type IndexedKeyArrayIterationProof struct {
	Array    StableAddress
	Key      StableAddress
	KeyValue product.AbstractValue
}

// IndexedKeyArrayIterationResult reports the tables reached by an indexed
// key-array iteration proof so callers can apply non-flow refinements.
type IndexedKeyArrayIterationResult struct {
	Tables []StableAddress
}

// ApplyIndexedKeyArrayIterationProof consumes a key-array provenance proof by
// publishing direct table/key presence for every table proven for the array.
func ApplyIndexedKeyArrayIterationProof(out *PointState, proof IndexedKeyArrayIterationProof) (IndexedKeyArrayIterationResult, bool) {
	if out == nil || proof.Array.Key() == "" || proof.Key.Key() == "" {
		return IndexedKeyArrayIterationResult{}, false
	}
	result := IndexedKeyArrayIterationResult{}
	beforePresence := out.KeyPresence
	beforeIndexWrites := out.IndexWrites
	arrayKey := proof.Array.Key()
	keyKey := proof.Key.Key()
	keyValue := proof.KeyValue
	if keyValue.IsZero() {
		keyValue = product.FromType(typ.Unknown)
	}
	for _, tableKey := range out.KeyPresence.KeyArrayTables(arrayKey) {
		table, ok := StableAddressFromKey(tableKey)
		if !ok {
			continue
		}
		result.Tables = append(result.Tables, table)
		out.KeyPresence = out.KeyPresence.With(tableKey, keyKey)
		for _, value := range out.KeyPresence.KeyArrayValues(arrayKey, tableKey) {
			if value.IsZero() {
				continue
			}
			out.IndexWrites = out.IndexWrites.WithAddress(IndexWriteAdmissionAddressFact{
				Target:     table,
				KeyPath:    proof.Key,
				HasKeyPath: true,
				Key:        keyValue,
				Value:      value,
			})
		}
	}
	changed := !KeyPresenceFactsDomain.Equal(beforePresence, out.KeyPresence)
	changed = !IndexWriteAdmissionFactsDomain.Equal(beforeIndexWrites, out.IndexWrites) || changed
	return result, changed
}

// MapWriteProof is the canonical point-local proof transaction for an admitted
// dynamic map write. It has two independent consequences:
//   - lightweight key/provenance facts when the write value is definitely present
//   - optional heavy readback facts when key/value products are admissible
type MapWriteProof struct {
	Table                  StableAddress
	Key                    StableAddress
	HasKey                 bool
	ValuePath              StableAddress
	HasValuePath           bool
	KeyValue               product.AbstractValue
	Value                  product.AbstractValue
	AllowOpaqueKeyReadback bool
}

// MapWritePathProof is the structured-path publication form for a dynamic map
// write. Flow owns lowering these paths to stable addresses before applying the
// reduced-product map-write transaction.
type MapWritePathProof struct {
	TablePath              constraint.Path
	KeyPath                constraint.Path
	ValuePath              constraint.Path
	KeyValue               product.AbstractValue
	Value                  product.AbstractValue
	AllowOpaqueKeyReadback bool
}

// MapWriteProofOfPathProof lowers a path-level dynamic map write proof to the
// stable-address proof consumed by ApplyMapWriteProof.
func MapWriteProofOfPathProof(proof MapWritePathProof) (MapWriteProof, bool) {
	tableAddr, ok := StableAddressOfPath(proof.TablePath)
	if !ok {
		return MapWriteProof{}, false
	}
	out := MapWriteProof{
		Table:                  tableAddr,
		KeyValue:               proof.KeyValue,
		Value:                  proof.Value,
		AllowOpaqueKeyReadback: proof.AllowOpaqueKeyReadback,
	}
	if !proof.KeyPath.IsEmpty() {
		if keyAddr, ok := StableAddressOfPath(proof.KeyPath); ok {
			out.Key = keyAddr
			out.HasKey = true
		}
	}
	if !proof.ValuePath.IsEmpty() {
		if valueAddr, ok := StableAddressOfPath(proof.ValuePath); ok {
			out.ValuePath = valueAddr
			out.HasValuePath = true
		}
	}
	return out, true
}

// ApplyMapWritePathProof lowers and applies a path-level dynamic map write
// proof.
func ApplyMapWritePathProof(out *PointState, proof MapWritePathProof) bool {
	normalized, ok := MapWriteProofOfPathProof(proof)
	if !ok {
		return false
	}
	return ApplyMapWriteProof(out, normalized)
}

// ApplyMapWriteProof applies all reduced-product consequences of one dynamic
// map write. Key facts and readback facts are intentionally independent, so a
// readback admission failure cannot suppress lightweight provenance.
func ApplyMapWriteProof(out *PointState, proof MapWriteProof) bool {
	if out == nil || proof.Table.Key() == "" {
		return false
	}
	changed := false
	if proof.Value.DefinitelyPresent() {
		changed = ApplyTablePresentWriteValueProof(out, proof.Table, proof.Value) || changed
	}
	if proof.HasKey && proof.Value.DefinitelyPresent() {
		changed = ApplyKeyPresenceProof(out, KeyPresenceProof{
			Table: proof.Table,
			Key:   proof.Key,
			Value: proof.Value,
		}) || changed
		changed = ApplyAppendHistoryCoverageProof(out, proof.Table, proof.Key, proof.Value) || changed
	}
	if fact, ok := proof.IndexWriteAdmissionAddressFact(); ok {
		before := out.IndexWrites
		out.IndexWrites = out.IndexWrites.WithAddress(fact)
		changed = !IndexWriteAdmissionFactsDomain.Equal(before, out.IndexWrites) || changed
	}
	return changed
}

// ApplyTablePresentWriteValueProof updates value-carrying key-array facts after
// a definitely-present table element write. The key may or may not be an
// existing element of a proven key array, so the new table[element] payload is
// the old universal payload joined with the written value.
func ApplyTablePresentWriteValueProof(out *PointState, table StableAddress, written product.AbstractValue) bool {
	if out == nil || table.Key() == "" || !written.DefinitelyPresent() {
		return false
	}
	tableKey := table.Key()
	before := out.KeyPresence
	for _, fact := range out.KeyPresence.KeyArrayValueEntries() {
		if fact.Table != tableKey {
			continue
		}
		out.KeyPresence = out.KeyPresence.WithKeyArrayValue(fact.Array, tableKey, written)
	}
	for _, fact := range out.KeyPresence.AppendHistoryCoverageEntries() {
		if fact.Table != tableKey {
			continue
		}
		out.KeyPresence = out.KeyPresence.WithAppendHistoryCoverage(fact.Array, fact.Key, tableKey, written)
	}
	return !KeyPresenceFactsDomain.Equal(before, out.KeyPresence)
}

// ApplyAppendHistoryCoverageProof marks tracked append events with Key as
// covered by Table[Key]. This is the write-after-append half of the append
// coverage reducer; write-before-append is handled when the append sees the
// ordinary key-presence/readback facts.
func ApplyAppendHistoryCoverageProof(out *PointState, table StableAddress, key StableAddress, value product.AbstractValue) bool {
	if out == nil || table.Key() == "" || key.Key() == "" || value.IsZero() {
		return false
	}
	tableKey := table.Key()
	keyKey := key.Key()
	before := out.KeyPresence
	for _, event := range out.KeyPresence.AppendHistoryEventEntries() {
		if event.Key != keyKey {
			continue
		}
		out.KeyPresence = out.KeyPresence.WithAppendHistoryCoverage(event.Array, keyKey, tableKey, value)
	}
	return !KeyPresenceFactsDomain.Equal(before, out.KeyPresence)
}

// IndexWriteAdmissionAddressFact returns the optional heavy readback consequence
// of the map write proof.
func (p MapWriteProof) IndexWriteAdmissionAddressFact() (IndexWriteAdmissionAddressFact, bool) {
	if p.Table.Key() == "" || p.KeyValue.IsZero() || p.Value.IsZero() {
		return IndexWriteAdmissionAddressFact{}, false
	}
	if !p.HasKey && !AdmissibleMapWriteProofValue(p.KeyValue) {
		return IndexWriteAdmissionAddressFact{}, false
	}
	if p.HasKey {
		if p.KeyValue.DefinitelyAbsent() {
			return IndexWriteAdmissionAddressFact{}, false
		}
		if !AdmissibleMapWriteProofValue(p.KeyValue) && !p.AllowOpaqueKeyReadback {
			return IndexWriteAdmissionAddressFact{}, false
		}
	}
	if !AdmissibleMapWriteProofValue(p.Value) {
		return IndexWriteAdmissionAddressFact{}, false
	}
	fact := IndexWriteAdmissionAddressFact{
		Target: p.Table,
		Key:    p.KeyValue,
		Value:  p.Value,
	}
	if p.HasKey {
		fact.KeyPath = p.Key
		fact.HasKeyPath = true
	}
	if p.HasValuePath {
		fact.ValuePath = p.ValuePath
		fact.HasValuePath = true
	}
	return fact, true
}

// AdmissibleMapWriteProofValue reports whether a product value is finite enough
// to publish through the heavy IndexWrites readback lane.
func AdmissibleMapWriteProofValue(av product.AbstractValue) bool {
	if av.IsZero() {
		return false
	}
	t := product.ProjectValueOrUnknown(av)
	return !typ.IsAbsentOrUnknown(t) && !typ.IsAny(t)
}

// IndexedIteratorKeyArrayReadback derives a map-readback value for table[key]
// when key is a value yielded from an indexed iteration over a proven key array.
//
// The proof intentionally composes stable must-facts instead of requiring a
// point-local IndexWriteAdmissionFact to survive loop joins:
//   - ValueOriginIndexedIterator proves key <- keyArray[i].
//   - KeyPresence.KeyArrayValues proves every element of keyArray indexes table
//     and carries a product value for table[element].
//
// Assignment aliases of the key are followed through ValueOriginAssignmentAlias.
func IndexedIteratorKeyArrayReadback(
	keyPresence KeyPresenceFacts,
	origins ValueOriginFacts,
	table StableAddress,
	key StableAddress,
) (product.AbstractValue, bool) {
	tableKey := table.Key()
	keyKey := key.Key()
	if keyPresence.IsBottom() || origins.IsBottom() || tableKey == "" || keyKey == "" {
		return product.AbstractValue{}, false
	}
	var out product.AbstractValue
	seen := map[constraint.PathKey]struct{}{}
	queue := []StableAddress{key}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		curKey := cur.Key()
		if curKey == "" {
			continue
		}
		if _, ok := seen[curKey]; ok {
			continue
		}
		seen[curKey] = struct{}{}
		for _, use := range origins.OriginsCoveringAddress(cur) {
			switch use.Origin.Kind {
			case ValueOriginIndexedIterator:
				if use.Origin.VarIndex != 1 || len(use.Remainder) != 0 {
					continue
				}
				for _, value := range keyPresence.KeyArrayValues(use.Origin.Source, tableKey) {
					if value.IsZero() {
						continue
					}
					if out.IsZero() {
						out = value
					} else {
						out = product.Domain.Join(out, value)
					}
				}
			case ValueOriginAssignmentAlias:
				source, ok := StableAddressFromKey(use.Origin.Source)
				if !ok {
					continue
				}
				source, ok = source.Append(use.Remainder)
				if ok {
					queue = append(queue, source)
				}
			}
		}
	}
	return out, !out.IsZero()
}
