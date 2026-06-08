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

// KeyArraySeedPathTransaction is the source-facing write-seed transaction for
// key-array facts. Flow lowers paths once, then applies address-native proofs.
type KeyArraySeedPathTransaction struct {
	ArrayPath constraint.Path
	TablePath constraint.Path
	HasTable  bool
	Empty     bool
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
	if proof.Table.Key() == "" || proof.Key.Key() == "" {
		return false
	}
	before := out.KeyPresence
	out.KeyPresence = out.KeyPresence.WithProofAddress(
		proof.Table,
		proof.Key,
		proof.Value,
		proof.ValuePath,
		proof.HasValuePath,
	)
	return !KeyPresenceFactsDomain.Equal(before, out.KeyPresence)
}

// ApplyKeyPresenceAliasProof applies key-presence facts proven for SourceKey to
// TargetKey. Value-path facts are preserved with the same table/value path.
func ApplyKeyPresenceAliasProof(out *PointState, proof KeyPresenceAliasProof) bool {
	if out == nil || proof.SourceKey.Key() == "" || proof.TargetKey.Key() == "" {
		return false
	}
	before := out.KeyPresence
	out.KeyPresence = out.KeyPresence.WithKeyAliasAddress(proof.SourceKey, proof.TargetKey)
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
	keyValue := proof.KeyValue
	if keyValue.IsZero() {
		keyValue = product.FromType(typ.Unknown)
	}
	beforePresence := out.KeyPresence
	beforeIndexWrites := out.IndexWrites
	for _, tableUse := range out.KeyPresence.KeyArrayTableAddresses(proof.Array) {
		table := tableUse.Address
		result.Tables = append(result.Tables, table)
		out.KeyPresence = out.KeyPresence.WithAddresses(table, proof.TargetKey)
		for _, value := range out.KeyPresence.KeyArrayValuesAddresses(proof.Array, table) {
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

// ApplyKeyArraySeedPathTransaction normalizes and applies source-level
// key-array seed facts.
func ApplyKeyArraySeedPathTransaction(out *PointState, tx KeyArraySeedPathTransaction) bool {
	if tx.ArrayPath.IsEmpty() {
		return false
	}
	array, arrayOK := StableAddressOfPath(tx.ArrayPath)
	if !arrayOK {
		return false
	}
	changed := false
	if tx.HasTable {
		table, tableOK := StableAddressOfPath(tx.TablePath)
		if !tableOK {
			return false
		}
		changed = ApplyKeyArrayProof(out, KeyArrayProof{
			Array: array,
			Table: table,
		}) || changed
	}
	if tx.Empty {
		changed = ApplyEmptyKeyArrayProof(out, EmptyKeyArrayProof{Array: array}) || changed
	}
	return changed
}

// ApplyKeyArrayValueProof applies a value-carrying key-array proof to point state.
func ApplyKeyArrayValueProof(out *PointState, proof KeyArrayValueProof) bool {
	if out == nil || proof.Array.Key() == "" || proof.Table.Key() == "" || proof.Value.IsZero() {
		return false
	}
	before := out.KeyPresence
	out.KeyPresence = out.KeyPresence.WithKeyArrayValueAddresses(proof.Array, proof.Table, proof.Value)
	if proof.HasAppendKey {
		out.KeyPresence = out.KeyPresence.WithAppendHistoryCoverageAddresses(proof.Array, proof.AppendKey, proof.Table, proof.Value)
	}
	return !KeyPresenceFactsDomain.Equal(before, out.KeyPresence)
}

// ApplyPendingKeyArrayProof applies delayed key-array provenance to point state.
func ApplyPendingKeyArrayProof(out *PointState, proof PendingKeyArrayProof) bool {
	if out == nil || proof.Array.Key() == "" || proof.Key.Key() == "" {
		return false
	}
	if proof.HasTable {
		if proof.Table.Key() == "" {
			return false
		}
	}
	before := out.KeyPresence
	out.KeyPresence = out.KeyPresence.WithPendingKeyArrayAddresses(proof.Array, proof.Table, proof.HasTable, proof.Key)
	return !KeyPresenceFactsDomain.Equal(before, out.KeyPresence)
}

// ApplyAppendKeyProof applies append-key provenance to point state.
func ApplyAppendKeyProof(out *PointState, proof AppendKeyProof) bool {
	if out == nil || proof.Array.Key() == "" || proof.Key.Key() == "" {
		return false
	}
	before := out.KeyPresence
	out.KeyPresence = out.KeyPresence.WithAppendedKeyAddresses(proof.Array, proof.Key)
	out.KeyPresence = out.KeyPresence.WithAppendHistoryEventAddresses(proof.Array, proof.Key)
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
	keyValue := proof.KeyValue
	if keyValue.IsZero() {
		keyValue = product.FromType(typ.Unknown)
	}
	for _, tableUse := range out.KeyPresence.KeyArrayTableAddresses(proof.Array) {
		table := tableUse.Address
		result.Tables = append(result.Tables, table)
		out.KeyPresence = out.KeyPresence.WithAddresses(table, proof.Key)
		for _, value := range out.KeyPresence.KeyArrayValuesAddresses(proof.Array, table) {
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

// DynamicIndexWriteProof is the canonical point-local proof transaction for one
// dynamic table[index] write. WrittenValue is the value that actually flows into
// key-presence/provenance consequences; ReadbackValue is the optional value safe
// to publish through the heavier dynamic-readback lane.
type DynamicIndexWriteProof struct {
	Table         StableAddress
	Key           StableAddress
	HasKey        bool
	ValuePath     StableAddress
	HasValuePath  bool
	KeyValue      product.AbstractValue
	WrittenValue  product.AbstractValue
	ReadbackValue product.AbstractValue
}

// DynamicIndexWritePathTransaction is the source-facing publication form for a
// dynamic table[index] write. Flow lowers paths once before applying the address
// transaction.
type DynamicIndexWritePathTransaction struct {
	TablePath     constraint.Path
	KeyPath       constraint.Path
	ValuePath     constraint.Path
	KeyValue      product.AbstractValue
	WrittenValue  product.AbstractValue
	ReadbackValue product.AbstractValue
}

// DynamicIndexWriteProofOfPath lowers a source-level dynamic-index transaction
// to the stable-address proof consumed by ApplyDynamicIndexWriteProof.
func DynamicIndexWriteProofOfPath(tx DynamicIndexWritePathTransaction) (DynamicIndexWriteProof, bool) {
	tableAddr, ok := StableAddressOfPath(tx.TablePath)
	if !ok {
		return DynamicIndexWriteProof{}, false
	}
	out := DynamicIndexWriteProof{
		Table:         tableAddr,
		KeyValue:      tx.KeyValue,
		WrittenValue:  tx.WrittenValue,
		ReadbackValue: tx.ReadbackValue,
	}
	if !tx.KeyPath.IsEmpty() {
		if keyAddr, ok := StableAddressOfPath(tx.KeyPath); ok {
			out.Key = keyAddr
			out.HasKey = true
		}
	}
	if !tx.ValuePath.IsEmpty() {
		if valueAddr, ok := StableAddressOfPath(tx.ValuePath); ok {
			out.ValuePath = valueAddr
			out.HasValuePath = true
		}
	}
	return out, true
}

// ApplyDynamicIndexWritePathTransaction lowers and applies a source-level
// dynamic-index write transaction.
func ApplyDynamicIndexWritePathTransaction(out *PointState, tx DynamicIndexWritePathTransaction) bool {
	normalized, ok := DynamicIndexWriteProofOfPath(tx)
	if !ok {
		return false
	}
	return ApplyDynamicIndexWriteProof(out, normalized)
}

// ApplyDynamicIndexWriteProof applies all reduced-product consequences of one
// dynamic-index write. Key facts and readback facts are intentionally
// independent, so a readback admission failure cannot suppress lightweight
// provenance.
func ApplyDynamicIndexWriteProof(out *PointState, proof DynamicIndexWriteProof) bool {
	if out == nil || proof.Table.Key() == "" {
		return false
	}
	changed := false
	if proof.WrittenValue.DefinitelyPresent() {
		changed = ApplyTablePresentWriteValueProof(out, proof.Table, proof.WrittenValue) || changed
	}
	if proof.HasKey && proof.WrittenValue.DefinitelyPresent() {
		changed = ApplyKeyPresenceProof(out, KeyPresenceProof{
			Table: proof.Table,
			Key:   proof.Key,
			Value: proof.WrittenValue,
		}) || changed
		changed = ApplyAppendHistoryCoverageProof(out, proof.Table, proof.Key, proof.WrittenValue) || changed
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
	before := out.KeyPresence
	out.KeyPresence = out.KeyPresence.WithTablePresentWriteValueAddress(table, written)
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
	before := out.KeyPresence
	out.KeyPresence = out.KeyPresence.WithAppendHistoryCoverageForKeyAddress(table, key, value)
	return !KeyPresenceFactsDomain.Equal(before, out.KeyPresence)
}

// IndexWriteAdmissionAddressFact returns the optional heavy readback
// consequence of the dynamic-index write proof.
func (p DynamicIndexWriteProof) IndexWriteAdmissionAddressFact() (IndexWriteAdmissionAddressFact, bool) {
	if p.Table.Key() == "" || p.KeyValue.IsZero() || p.ReadbackValue.IsZero() {
		return IndexWriteAdmissionAddressFact{}, false
	}
	if !p.HasKey && !AdmissibleDynamicIndexWriteProofValue(p.KeyValue) {
		return IndexWriteAdmissionAddressFact{}, false
	}
	if p.HasKey {
		if p.KeyValue.DefinitelyAbsent() {
			return IndexWriteAdmissionAddressFact{}, false
		}
	}
	if !AdmissibleDynamicIndexWriteProofValue(p.ReadbackValue) {
		return IndexWriteAdmissionAddressFact{}, false
	}
	fact := IndexWriteAdmissionAddressFact{
		Target: p.Table,
		Key:    p.KeyValue,
		Value:  p.ReadbackValue,
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

// AdmissibleDynamicIndexWriteProofValue reports whether a product value is finite enough
// to publish through the heavy IndexWrites readback lane.
func AdmissibleDynamicIndexWriteProofValue(av product.AbstractValue) bool {
	if av.IsZero() {
		return false
	}
	t := product.ProjectValueOrUnknown(av)
	return !typ.IsAbsentOrUnknown(t) && !typ.IsAny(t)
}

// DynamicIndexWriteReadbackValue returns the value safe to expose through the
// heavy readback lane after a product-domain index write. When the post-write
// container has a more precise table[key] slot than the RHS, readback uses that
// slot; otherwise it falls back to the written value.
func DynamicIndexWriteReadbackValue(container, key, written product.AbstractValue) product.AbstractValue {
	if container.IsZero() || key.IsZero() {
		return written
	}
	read, ok := product.RuntimeIndexOf(container, key)
	if !ok || read.IsZero() {
		return written
	}
	if written.DefinitelyPresent() {
		present := product.NarrowPresent(read)
		if !present.IsZero() {
			return present
		}
	}
	return read
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
	relations := relationIndexForOrigins(origins)
	var seen stableAddressSet
	queue := []StableAddress{key}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if !seen.Add(cur) {
			continue
		}
		for _, route := range relations.SourceRoutes(relationSourceQuery{
			Target:   cur,
			Kind:     relationSourceExactIndexedIteratorValue,
			VarIndex: 1,
		}) {
			for _, value := range keyPresence.KeyArrayValuesAddresses(route.source, table) {
				if value.IsZero() {
					continue
				}
				if out.IsZero() {
					out = value
				} else {
					out = product.Domain.Join(out, value)
				}
			}
		}
		for _, alias := range relations.SourceRoutes(relationSourceQuery{
			Target: cur,
			Kind:   relationSourceAssignmentAlias,
		}) {
			source, ok := alias.source.Append(alias.remainder)
			if ok {
				queue = append(queue, source)
			}
		}
	}
	return out, !out.IsZero()
}
