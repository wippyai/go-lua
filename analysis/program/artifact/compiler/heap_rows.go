package compiler

import (
	"github.com/wippyai/go-lua/analysis/identity"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// heapFieldDraft is the immutable scalar constructor geometry captured while
// the Program proof is live. Terms are cold source coordinates solely for
// Link substitution; no Program, Flow, or domain value escapes.
type heapFieldDraft struct {
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

func (row heapFieldDraft) Available() bool {
	return row.id.Available() && row.kind >= flowkind.FieldList && row.kind <= flowkind.FieldKey && row.fieldSpan.Available() && row.valuesSpan.Available() && row.valuesID.Available() && row.width >= 0 && (row.kind == flowkind.FieldKey) == row.selectorSpan.Available() && (row.kind == flowkind.FieldKey || !row.sharesFirstValueCell)
}
func (row heapFieldDraft) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}
func (row heapFieldDraft) Kind() flowkind.FieldKind {
	if !row.Available() {
		return 0
	}
	return row.kind
}
func (row heapFieldDraft) FieldSpan() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.fieldSpan
}
func (row heapFieldDraft) SelectorSpan() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.selectorSpan
}
func (row heapFieldDraft) Values() (identity.ContentID, int, bool, bool) {
	if !row.Available() {
		return identity.ContentID{}, 0, false, false
	}
	return row.valuesSpan, row.width, row.finalOpen, true
}

// Width returns the exact authored Values member count copied into this
// field's constructor geometry.
func (row heapFieldDraft) Width() int {
	if !row.Available() {
		return 0
	}
	return row.width
}

// FinalOpen reports whether this field's Values row has an open tail.
func (row heapFieldDraft) FinalOpen() bool {
	return row.Available() && row.finalOpen
}
func (row heapFieldDraft) ValuesID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.valuesID
}

// SharesFirstValueCell preserves the exact dynamic FieldKey relation needed
// by Heap's closed-constructor descriptor without retaining raw read terms.
func (row heapFieldDraft) SharesFirstValueCell() bool {
	return row.Available() && row.sharesFirstValueCell
}
func (row heapFieldDraft) NormalizedKey() (keyspace.Key, bool) {
	if !row.Available() {
		return 0, false
	}
	return row.normalized, row.normalizedOK
}

// heapAllocationDraft is one allocation template plus its ordered field
// geometry. It is neutral source data consumed by Heap at Link binding time.
type heapAllocationDraft struct {
	id       identity.ContentID
	role     allocationRole
	form     allocationForm
	rootSpan identity.ContentID
	fields   []heapFieldDraft
}

func (row heapAllocationDraft) Available() bool {
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
func (row heapAllocationDraft) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}

func (row heapAllocationDraft) Role() allocationRole {
	if !row.Available() {
		return allocationInvalid
	}
	return row.role
}

func (row heapAllocationDraft) Form() allocationForm {
	if !row.Available() {
		return allocationFormInvalid
	}
	return row.form
}
func (row heapAllocationDraft) RootSpan() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.rootSpan
}
func (row heapAllocationDraft) FieldCount() int {
	if !row.Available() {
		return 0
	}
	return len(row.fields)
}
func (row heapAllocationDraft) FieldAt(index int) (heapFieldDraft, bool) {
	if !row.Available() || index < 0 || index >= len(row.fields) {
		return heapFieldDraft{}, false
	}
	return row.fields[index], true
}

// heapIndexDraft is one scalar IndexRead/IndexWrite candidate. A false Read
// denotes a write and therefore carries Values and its exact position.
type heapIndexDraft struct {
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

func (row heapIndexDraft) Available() bool {
	if !row.id.Available() || !row.baseSpan.Available() || row.lensKind == 0 || row.lensKind > 2 {
		return false
	}
	if row.lensKind == 1 && row.exactKey == 0 || row.lensKind == 2 && !row.keySpan.Available() {
		return false
	}
	return row.read && row.resultSpan.Available() && !row.valuesSpan.Available() && !row.valuesID.Available() && row.position == -1 || !row.read && !row.resultSpan.Available() && row.valuesSpan.Available() && row.valuesID.Available() && row.position >= 0
}
func (row heapIndexDraft) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}
func (row heapIndexDraft) Read() bool { return row.Available() && row.read }
func (row heapIndexDraft) BaseSpan() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.baseSpan
}
func (row heapIndexDraft) ResultSpan() identity.ContentID {
	if !row.Available() || !row.read {
		return identity.ContentID{}
	}
	return row.resultSpan
}
func (row heapIndexDraft) DynamicKeySpan() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.keySpan
}
func (row heapIndexDraft) ExactKey() (keyspace.Key, bool) {
	if !row.Available() || row.lensKind != 1 {
		return 0, false
	}
	return row.exactKey, true
}
func (row heapIndexDraft) Values() (identity.ContentID, int, bool) {
	if !row.Available() || row.read {
		return identity.ContentID{}, 0, false
	}
	return row.valuesSpan, row.position, true
}
func (row heapIndexDraft) ValuesID() identity.ContentID {
	if !row.Available() || row.read {
		return identity.ContentID{}
	}
	return row.valuesID
}
