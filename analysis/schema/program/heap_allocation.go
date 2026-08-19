package programschema

import "github.com/wippyai/go-lua/analysis/identity"

// Heap field ordinals are the historical Flow field-kind ordinals. They are
// kept as plain bytes here: the cold column preserves the value and shape but
// does not import or reinterpret the Program vocabulary.
const (
	HeapFieldKindList  uint8 = 1
	HeapFieldKindName  uint8 = 2
	HeapFieldKindExact uint8 = 3
	HeapFieldKindKey   uint8 = 4
)

// HeapField is one allocation-field geometry row. FieldSpan, SelectorSpan,
// ValuesSpan and ValuesID are all the exact identities copied by the
// canonical HeapFieldRow. Width and the booleans retain the historical
// constructor geometry without retaining authored terms or Flow objects.
type HeapField struct {
	id                   identity.ContentID
	kind                 uint8
	fieldSpan            identity.ContentID
	selectorSpan         identity.ContentID
	valuesSpan           identity.ContentID
	valuesID             identity.ContentID
	width                int
	finalOpen            bool
	sharesFirstValueCell bool
	normalized           uint64
	normalizedOK         bool
}

// NewHeapField copies one canonical HeapFieldRow. SelectorSpan is required
// only for the FieldKey ordinal; all other kinds must not carry one.
func NewHeapField(id identity.ContentID, kind uint8, fieldSpan, selectorSpan, valuesSpan, valuesID identity.ContentID, width int, finalOpen, sharesFirstValueCell bool, normalized uint64, normalizedOK bool) (HeapField, bool) {
	row := HeapField{
		id: id, kind: kind, fieldSpan: fieldSpan, selectorSpan: selectorSpan,
		valuesSpan: valuesSpan, valuesID: valuesID, width: width,
		finalOpen: finalOpen, sharesFirstValueCell: sharesFirstValueCell,
		normalized: normalized, normalizedOK: normalizedOK,
	}
	return row, row.Available()
}

func (row HeapField) Available() bool {
	return row.id.Available() && row.kind >= HeapFieldKindList && row.kind <= HeapFieldKindKey &&
		row.fieldSpan.Available() && row.valuesSpan.Available() && row.valuesID.Available() && row.width >= 0 &&
		(row.kind == HeapFieldKindKey) == row.selectorSpan.Available() &&
		(row.kind == HeapFieldKindKey || !row.sharesFirstValueCell)
}

func (row HeapField) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}

func (row HeapField) Kind() uint8 {
	if !row.Available() {
		return 0
	}
	return row.kind
}

func (row HeapField) FieldSpan() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.fieldSpan
}

func (row HeapField) SelectorSpan() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.selectorSpan
}

func (row HeapField) Values() (identity.ContentID, int, bool, bool) {
	if !row.Available() {
		return identity.ContentID{}, 0, false, false
	}
	return row.valuesSpan, row.width, row.finalOpen, true
}

func (row HeapField) Width() int {
	if !row.Available() {
		return 0
	}
	return row.width
}

func (row HeapField) FinalOpen() bool { return row.Available() && row.finalOpen }

func (row HeapField) ValuesID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.valuesID
}

func (row HeapField) SharesFirstValueCell() bool {
	return row.Available() && row.sharesFirstValueCell
}

func (row HeapField) NormalizedKey() (uint64, bool) {
	if !row.Available() {
		return 0, false
	}
	return row.normalized, row.normalizedOK
}

// Heap allocation roles retain the artifact's table/closure ordinals.
const (
	HeapAllocationRoleTable   uint8 = 1
	HeapAllocationRoleClosure uint8 = 2
)

// Heap allocation forms retain the artifact's empty/closed/final-open
// ordinals. Form zero is invalid.
const (
	HeapAllocationFormEmpty     uint8 = 1
	HeapAllocationFormClosed    uint8 = 2
	HeapAllocationFormFinalOpen uint8 = 3
)

// HeapAllocation is one allocation template. Its fields are a span in the
// separate HeapFieldFamily, preserving the canonical field order while making
// this row flat and copy-safe.
type HeapAllocation struct {
	id          identity.ContentID
	role        uint8
	form        uint8
	rootSpan    identity.ContentID
	fieldOffset uint32
	fieldCount  uint32
}

// NewHeapAllocation copies one canonical HeapAllocationRow and replaces its
// nested field slice with a dense HeapFieldFamily span.
func NewHeapAllocation(id identity.ContentID, role, form uint8, rootSpan identity.ContentID, fieldOffset, fieldCount uint32) (HeapAllocation, bool) {
	row := HeapAllocation{id: id, role: role, form: form, rootSpan: rootSpan, fieldOffset: fieldOffset, fieldCount: fieldCount}
	return row, row.Available()
}

func (row HeapAllocation) Available() bool {
	return row.id.Available() && row.role >= HeapAllocationRoleTable && row.role <= HeapAllocationRoleClosure &&
		row.form >= HeapAllocationFormEmpty && row.form <= HeapAllocationFormFinalOpen && row.rootSpan.Available() &&
		uint64(row.fieldOffset)+uint64(row.fieldCount) <= uint64(^uint32(0))
}

func (row HeapAllocation) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}

func (row HeapAllocation) Role() uint8 {
	if !row.Available() {
		return 0
	}
	return row.role
}

func (row HeapAllocation) Form() uint8 {
	if !row.Available() {
		return 0
	}
	return row.form
}

func (row HeapAllocation) RootSpan() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.rootSpan
}

func (row HeapAllocation) FieldSpan() (offset, count uint32, ok bool) {
	return row.fieldOffset, row.fieldCount, row.Available()
}

func (row HeapAllocation) FieldCount() int {
	if !row.Available() {
		return 0
	}
	return int(row.fieldCount)
}
