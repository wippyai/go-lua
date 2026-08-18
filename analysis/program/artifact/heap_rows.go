package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
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
	return len(artifact.heapAllocations)
}

func (artifact *Artifact) HeapAllocationAt(index int) (HeapAllocationRow, bool) {
	if !artifact.Available() || index < 0 || index >= len(artifact.heapAllocations) {
		return HeapAllocationRow{}, false
	}
	return artifact.heapAllocations[index], true
}

// HeapIndexCount and HeapIndexAt expose the exact reusable access geometry
// for Link-local Heap substitution.
func (artifact *Artifact) HeapIndexCount() int {
	if !artifact.Available() {
		return 0
	}
	return len(artifact.heapIndexes)
}

func (artifact *Artifact) HeapIndexAt(index int) (HeapIndexRow, bool) {
	if !artifact.Available() || index < 0 || index >= len(artifact.heapIndexes) {
		return HeapIndexRow{}, false
	}
	return artifact.heapIndexes[index], true
}
