package programschema

import "github.com/wippyai/go-lua/analysis/identity"

func (row Program) OccurrenceCount() (int, bool) {
	catalog, ok := row.catalog()
	if !ok {
		return 0, false
	}
	return OccurrenceFamily().Count(&row.Frozen, catalog)
}

func (row Program) OccurrenceAt(index int) (Occurrence, bool) {
	catalog, ok := row.catalog()
	if !ok {
		return Occurrence{}, false
	}
	return OccurrenceFamily().At(&row.Frozen, catalog, index)
}

func (row Program) OccurrencePointAt(index int) (OccurrencePoint, bool) {
	catalog, ok := row.catalog()
	if !ok {
		return OccurrencePoint{}, false
	}
	return OccurrencePointFamily().At(&row.Frozen, catalog, index)
}

func (row Program) OccurrenceInputAt(index int) (OccurrenceInput, bool) {
	catalog, ok := row.catalog()
	if !ok {
		return OccurrenceInput{}, false
	}
	return OccurrenceInputFamily().At(&row.Frozen, catalog, index)
}

func (row Program) OccurrencePointFor(occurrenceIndex, childIndex int) (OccurrencePoint, bool) {
	occurrence, ok := row.OccurrenceAt(occurrenceIndex)
	if !ok || childIndex < 0 {
		return OccurrencePoint{}, false
	}
	offset, count, spanOK := occurrence.PointSpan()
	if !spanOK || uint64(childIndex) >= uint64(count) {
		return OccurrencePoint{}, false
	}
	point, held := row.OccurrencePointAt(int(offset) + childIndex)
	return point, held
}

func (row Program) OccurrenceInputFor(occurrenceIndex, childIndex int) (OccurrenceInput, bool) {
	occurrence, ok := row.OccurrenceAt(occurrenceIndex)
	if !ok || childIndex < 0 {
		return OccurrenceInput{}, false
	}
	offset, count, spanOK := occurrence.InputSpan()
	if !spanOK || uint64(childIndex) >= uint64(count) {
		return OccurrenceInput{}, false
	}
	input, held := row.OccurrenceInputAt(int(offset) + childIndex)
	return input, held
}

func (row Program) OccurrencePointID(occurrenceIndex, childIndex int) (identity.ContentID, bool) {
	point, ok := row.OccurrencePointFor(occurrenceIndex, childIndex)
	if !ok {
		return identity.ContentID{}, false
	}
	return point.PointID(), true
}

func (row Program) OccurrenceInputID(occurrenceIndex, childIndex int) (identity.ContentID, bool) {
	input, ok := row.OccurrenceInputFor(occurrenceIndex, childIndex)
	if !ok {
		return identity.ContentID{}, false
	}
	return input.InputID(), true
}

// OccurrenceKindAt walks the canonical parent family in emission order. It is
// deliberately a scan: kind indexes are compile-only helpers and are not
// another retained Program authority.
func (row Program) OccurrenceKindCount(kind OccurrenceKind) (int, bool) {
	if !kind.Valid() {
		return 0, false
	}
	count, ok := row.OccurrenceCount()
	if !ok {
		return 0, false
	}
	result := 0
	for index := 0; index < count; index++ {
		occurrence, held := row.OccurrenceAt(index)
		if held && occurrence.Kind() == kind {
			result++
		}
	}
	return result, true
}

func (row Program) OccurrenceKindAt(kind OccurrenceKind, ordinal int) (Occurrence, bool) {
	if !kind.Valid() || ordinal < 0 {
		return Occurrence{}, false
	}
	count, ok := row.OccurrenceCount()
	if !ok {
		return Occurrence{}, false
	}
	for index := 0; index < count; index++ {
		occurrence, held := row.OccurrenceAt(index)
		if !held || occurrence.Kind() != kind {
			continue
		}
		if ordinal == 0 {
			return occurrence, true
		}
		ordinal--
	}
	return Occurrence{}, false
}

func (row Program) OccurrenceForID(kind OccurrenceKind, id identity.ContentID) (Occurrence, bool) {
	if !kind.Valid() || !id.Available() {
		return Occurrence{}, false
	}
	count, ok := row.OccurrenceCount()
	if !ok {
		return Occurrence{}, false
	}
	for index := 0; index < count; index++ {
		occurrence, held := row.OccurrenceAt(index)
		if held && occurrence.Kind() == kind && occurrence.ID() == id {
			return occurrence, true
		}
	}
	return Occurrence{}, false
}

// OccurrenceOrdinalForID returns the dense parent position for one canonical
// occurrence. The ordinal is the only child-span join key; the scan is
// intentional because retained occurrence indexes belong to the compiler's
// seal-time workspace, not the published Program.
func (row Program) OccurrenceOrdinalForID(kind OccurrenceKind, id identity.ContentID) (int, bool) {
	if !kind.Valid() || !id.Available() {
		return 0, false
	}
	count, ok := row.OccurrenceCount()
	if !ok {
		return 0, false
	}
	for index := 0; index < count; index++ {
		occurrence, held := row.OccurrenceAt(index)
		if held && occurrence.Kind() == kind && occurrence.ID() == id {
			return index, true
		}
	}
	return 0, false
}

func (row Program) RuleOccurrenceCount() (int, bool) {
	catalog, ok := row.catalog()
	if !ok {
		return 0, false
	}
	return RuleOccurrenceFamily().Count(&row.Frozen, catalog)
}

func (row Program) RuleOccurrenceAt(index int) (RuleOccurrence, bool) {
	catalog, ok := row.catalog()
	if !ok {
		return RuleOccurrence{}, false
	}
	return RuleOccurrenceFamily().At(&row.Frozen, catalog, index)
}

func (row Program) RuleOccurrenceCountForKey(key string) (int, bool) {
	if key == "" {
		return 0, false
	}
	count, ok := row.RuleOccurrenceCount()
	if !ok {
		return 0, false
	}
	result := 0
	for index := 0; index < count; index++ {
		placement, held := row.RuleOccurrenceAt(index)
		if held && string(placement.Key()) == key {
			result++
		}
	}
	return result, true
}

func (row Program) RuleOccurrenceForKeyAt(key string, ordinal int) (RuleOccurrence, bool) {
	if key == "" || ordinal < 0 {
		return RuleOccurrence{}, false
	}
	count, ok := row.RuleOccurrenceCount()
	if !ok {
		return RuleOccurrence{}, false
	}
	for index := 0; index < count; index++ {
		placement, held := row.RuleOccurrenceAt(index)
		if !held || string(placement.Key()) != key {
			continue
		}
		if ordinal == 0 {
			return placement, true
		}
		ordinal--
	}
	return RuleOccurrence{}, false
}

// OccurrenceOutputSemanticID publishes the output identity of one canonical
// occurrence: the Program-issued semantic value the occurrence establishes.
// The distinction is the owner's because only the owner holds both the closed
// kind vocabulary and the sealed operand vector it is read against.
//
// A value-producing kind names its output by its own identity. A storage
// transfer, a storage write, an index read and an allocation name theirs in
// the operand vector instead, because for those families the occurrence
// identity is the operation or the reusable template, not the value it
// establishes. Every other kind establishes no value and reports false; the
// vocabulary is closed, so an unrecognised kind is not a producer.
func (row Program) OccurrenceOutputSemanticID(index int) (identity.ContentID, bool) {
	occurrence, ok := row.OccurrenceAt(index)
	if !ok {
		return identity.ContentID{}, false
	}
	if operand, named := occurrenceOutputOperand(occurrence.Kind()); named {
		return row.OccurrenceInputID(index, operand)
	}
	switch occurrence.Kind() {
	case OccurrenceValueSource, OccurrenceFormalEntry, OccurrenceGlobalEntry, OccurrenceStorageRead,
		OccurrenceBinaryEquality, OccurrenceBinaryArithmetic, OccurrenceBinaryOrder:
		return occurrence.ID(), true
	default:
		return identity.ContentID{}, false
	}
}
