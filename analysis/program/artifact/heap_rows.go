package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/schema/cold"
)

// HeapFieldRow is the immutable scalar constructor geometry captured while
// the Program proof is live. Terms are cold source coordinates solely for
// Link substitution; no Program, Flow, or domain value escapes.
type HeapFieldRow struct {
	id                   identity.ContentID
	kind                 flowkind.FieldKind
	fieldSpan            identity.ContentID
	selectorSpan         identity.ContentID
	valuesSpan           identity.ContentID
	valuesID             identity.ContentID
	width                int
	finalOpen            bool
	sharesFirstValueCell bool
	normalized           keyspace.Key
	normalizedOK         bool
}

func (row HeapFieldRow) Available() bool {
	return row.id.Available() && row.kind >= flowkind.FieldList && row.kind <= flowkind.FieldKey && row.fieldSpan.Available() && row.valuesSpan.Available() && row.valuesID.Available() && row.width >= 0 && (row.kind == flowkind.FieldKey) == row.selectorSpan.Available() && (row.kind == flowkind.FieldKey || !row.sharesFirstValueCell)
}
func (row HeapFieldRow) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}
func (row HeapFieldRow) Kind() flowkind.FieldKind {
	if !row.Available() {
		return 0
	}
	return row.kind
}
func (row HeapFieldRow) FieldSpan() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.fieldSpan
}
func (row HeapFieldRow) SelectorSpan() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.selectorSpan
}
func (row HeapFieldRow) Values() (identity.ContentID, int, bool, bool) {
	if !row.Available() {
		return identity.ContentID{}, 0, false, false
	}
	return row.valuesSpan, row.width, row.finalOpen, true
}

// Width returns the exact authored Values member count copied into this
// field's constructor geometry.
func (row HeapFieldRow) Width() int {
	if !row.Available() {
		return 0
	}
	return row.width
}

// FinalOpen reports whether this field's Values row has an open tail.
func (row HeapFieldRow) FinalOpen() bool {
	return row.Available() && row.finalOpen
}
func (row HeapFieldRow) ValuesID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.valuesID
}

// SharesFirstValueCell preserves the exact dynamic FieldKey relation needed
// by Heap's closed-constructor descriptor without retaining raw read terms.
func (row HeapFieldRow) SharesFirstValueCell() bool {
	return row.Available() && row.sharesFirstValueCell
}
func (row HeapFieldRow) NormalizedKey() (keyspace.Key, bool) {
	if !row.Available() {
		return 0, false
	}
	return row.normalized, row.normalizedOK
}

// HeapAllocationRow is one allocation template plus its ordered field
// geometry. It is neutral source data consumed by Heap at Link binding time.
type HeapAllocationRow struct {
	id       identity.ContentID
	role     AllocationRole
	form     AllocationForm
	rootSpan identity.ContentID
	fields   []HeapFieldRow
}

func (row HeapAllocationRow) Available() bool {
	if !row.id.Available() || !row.role.Valid() || !row.form.Valid() || !row.rootSpan.Available() {
		return false
	}
	for _, field := range row.fields {
		if !field.Available() {
			return false
		}
	}
	return true
}
func (row HeapAllocationRow) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}

func (row HeapAllocationRow) Role() AllocationRole {
	if !row.Available() {
		return AllocationInvalid
	}
	return row.role
}

func (row HeapAllocationRow) Form() AllocationForm {
	if !row.Available() {
		return AllocationFormInvalid
	}
	return row.form
}
func (row HeapAllocationRow) RootSpan() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.rootSpan
}
func (row HeapAllocationRow) FieldCount() int {
	if !row.Available() {
		return 0
	}
	return len(row.fields)
}
func (row HeapAllocationRow) FieldAt(index int) (HeapFieldRow, bool) {
	if !row.Available() || index < 0 || index >= len(row.fields) {
		return HeapFieldRow{}, false
	}
	return row.fields[index], true
}

// HeapIndexRow is one scalar IndexRead/IndexWrite candidate. A false Read
// denotes a write and therefore carries Values and its exact position.
type HeapIndexRow struct {
	id         identity.ContentID
	read       bool
	baseSpan   identity.ContentID
	resultSpan identity.ContentID
	keySpan    identity.ContentID
	lensKind   uint8
	exactKey   keyspace.Key
	valuesSpan identity.ContentID
	valuesID   identity.ContentID
	position   int
}

func (row HeapIndexRow) Available() bool {
	if !row.id.Available() || !row.baseSpan.Available() || row.lensKind == 0 || row.lensKind > 2 {
		return false
	}
	if row.lensKind == 1 && row.exactKey == 0 || row.lensKind == 2 && !row.keySpan.Available() {
		return false
	}
	return row.read && row.resultSpan.Available() && !row.valuesSpan.Available() && !row.valuesID.Available() && row.position == -1 || !row.read && !row.resultSpan.Available() && row.valuesSpan.Available() && row.valuesID.Available() && row.position >= 0
}
func (row HeapIndexRow) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}
func (row HeapIndexRow) Read() bool { return row.Available() && row.read }
func (row HeapIndexRow) BaseSpan() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.baseSpan
}
func (row HeapIndexRow) ResultSpan() identity.ContentID {
	if !row.Available() || !row.read {
		return identity.ContentID{}
	}
	return row.resultSpan
}
func (row HeapIndexRow) DynamicKeySpan() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.keySpan
}
func (row HeapIndexRow) ExactKey() (keyspace.Key, bool) {
	if !row.Available() || row.lensKind != 1 {
		return 0, false
	}
	return row.exactKey, true
}
func (row HeapIndexRow) Values() (identity.ContentID, int, bool) {
	if !row.Available() || row.read {
		return identity.ContentID{}, 0, false
	}
	return row.valuesSpan, row.position, true
}
func (row HeapIndexRow) ValuesID() identity.ContentID {
	if !row.Available() || row.read {
		return identity.ContentID{}
	}
	return row.valuesID
}

// HeapAllocationCount and HeapAllocationAt expose the exact reusable
// allocation geometry for Link-local Heap substitution.
func (artifact *Artifact) HeapAllocationCount() int {
	if !artifact.Available() {
		return 0
	}
	count, published := coldCount(artifact, cold.HeapAllocationFamily())
	if !published {
		return 0
	}
	return count
}

func (artifact *Artifact) HeapAllocationAt(index int) (HeapAllocationRow, bool) {
	if !artifact.Available() {
		return HeapAllocationRow{}, false
	}
	return artifact.heapAllocationRowAt(index)
}

// heapAllocationRowAt reads one allocation template out of the sealed
// publication and rejoins it with the field span it names. Fields are a dense
// plane there, so the ordered geometry a caller receives is assembled at the
// read site rather than retained a second time beside the publication.
func (artifact *Artifact) heapAllocationRowAt(index int) (HeapAllocationRow, bool) {
	sealed, held := coldRow(artifact, cold.HeapAllocationFamily(), index)
	offset, count, spanOK := sealed.FieldSpan()
	if !held || !spanOK {
		return HeapAllocationRow{}, false
	}
	row := HeapAllocationRow{
		id: sealed.ID(), role: AllocationRole(sealed.Role()), form: AllocationForm(sealed.Form()),
		rootSpan: sealed.RootSpan(), fields: make([]HeapFieldRow, 0, count),
	}
	for position := uint32(0); position < count; position++ {
		field, fieldHeld := artifact.heapFieldRowAt(int(offset + position))
		if !fieldHeld {
			return HeapAllocationRow{}, false
		}
		row.fields = append(row.fields, field)
	}
	return row, row.Available()
}

func (artifact *Artifact) heapFieldRowAt(index int) (HeapFieldRow, bool) {
	sealed, held := coldRow(artifact, cold.HeapFieldFamily(), index)
	valuesSpan, width, finalOpen, valuesOK := sealed.Values()
	normalized, normalizedOK := sealed.NormalizedKey()
	if !held || !valuesOK {
		return HeapFieldRow{}, false
	}
	row := HeapFieldRow{
		id: sealed.ID(), kind: flowkind.FieldKind(sealed.Kind()), fieldSpan: sealed.FieldSpan(),
		selectorSpan: sealed.SelectorSpan(), valuesSpan: valuesSpan, valuesID: sealed.ValuesID(),
		width: width, finalOpen: finalOpen, sharesFirstValueCell: sealed.SharesFirstValueCell(),
		normalized: keyspace.Key(normalized), normalizedOK: normalizedOK,
	}
	return row, row.Available()
}

// HeapIndexCount and HeapIndexAt expose the exact reusable access geometry
// for Link-local Heap substitution.
func (artifact *Artifact) HeapIndexCount() int {
	if !artifact.Available() {
		return 0
	}
	count, published := coldCount(artifact, cold.HeapIndexFamily())
	if !published {
		return 0
	}
	return count
}

func (artifact *Artifact) HeapIndexAt(index int) (HeapIndexRow, bool) {
	if !artifact.Available() {
		return HeapIndexRow{}, false
	}
	return artifact.heapIndexRowAt(index)
}

func (artifact *Artifact) heapIndexRowAt(index int) (HeapIndexRow, bool) {
	sealed, held := coldRow(artifact, cold.HeapIndexFamily(), index)
	if !held {
		return HeapIndexRow{}, false
	}
	exactKey, _ := sealed.ExactKey()
	valuesSpan, _, _ := sealed.Values()
	row := HeapIndexRow{
		id: sealed.ID(), read: sealed.Read(), baseSpan: sealed.BaseSpan(), resultSpan: sealed.ResultSpan(),
		keySpan: sealed.DynamicKeySpan(), lensKind: sealed.LensKind(), exactKey: keyspace.Key(exactKey),
		valuesSpan: valuesSpan, valuesID: sealed.ValuesID(), position: sealed.Position(),
	}
	return row, row.Available()
}
