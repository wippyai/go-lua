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
	Table StableAddress
	Key   StableAddress
	Value product.AbstractValue
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
	for _, array := range out.KeyPresence.PendingKeyArraysFor(tableKey, keyKey) {
		out.KeyPresence = out.KeyPresence.WithKeyArray(array, tableKey)
		if !proof.Value.IsZero() {
			out.KeyPresence = out.KeyPresence.WithKeyArrayValue(array, tableKey, proof.Value)
		}
	}
	return !KeyPresenceFactsDomain.Equal(before, out.KeyPresence)
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
	tablePath constraint.Path,
	keyPath constraint.Path,
) (product.AbstractValue, bool) {
	tableKey := KeyPresencePathKey(tablePath)
	keyKey := KeyPresencePathKey(keyPath)
	if keyPresence.IsBottom() || origins.IsBottom() || tableKey == "" || keyKey == "" {
		return product.AbstractValue{}, false
	}
	var out product.AbstractValue
	seen := map[constraint.PathKey]struct{}{}
	queue := []constraint.PathKey{keyKey}
	for len(queue) > 0 {
		curKey := queue[0]
		queue = queue[1:]
		if curKey == "" {
			continue
		}
		if _, ok := seen[curKey]; ok {
			continue
		}
		seen[curKey] = struct{}{}
		curPath, ok := pathFromSymbolPathKey(curKey)
		if !ok {
			continue
		}
		curAddr, ok := StableAddressOfPath(curPath)
		if !ok {
			continue
		}
		for _, use := range origins.OriginsCoveringAddress(curAddr) {
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
				sourcePath, ok := pathFromSymbolPathKey(use.Origin.Source)
				if !ok {
					continue
				}
				for _, seg := range use.Remainder {
					sourcePath = sourcePath.Append(seg)
				}
				sourceKey := KeyPresencePathKey(sourcePath)
				if sourceKey != "" {
					queue = append(queue, sourceKey)
				}
			}
		}
	}
	return out, !out.IsZero()
}

func pathFromSymbolPathKey(key constraint.PathKey) (constraint.Path, bool) {
	sym, segments, ok := ParseSymbolPathKey(key)
	if !ok || sym == 0 {
		return constraint.Path{}, false
	}
	return constraint.Path{
		Symbol:   sym,
		Segments: append([]constraint.Segment(nil), segments...),
	}, true
}
