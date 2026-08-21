// Package heapallocation owns the immutable heap allocation and field planes
// of a sealed Program publication.
package heapallocation

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/analysis/identity"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	programfamily "github.com/wippyai/go-lua/analysis/schema/program/family"
	"github.com/wippyai/go-lua/internal/framing"
)

// FieldKind is the stable ordinal of one table field geometry shape.
type FieldKind uint8

const (
	FieldKindList  FieldKind = 1
	FieldKindName  FieldKind = 2
	FieldKindExact FieldKind = 3
	FieldKindKey   FieldKind = 4
)

// Valid reports whether kind names a declared field geometry shape.
func (kind FieldKind) Valid() bool {
	return kind >= FieldKindList && kind <= FieldKindKey
}

// Role is the stable ordinal of an allocation owner.
type Role uint8

const (
	RoleTable   Role = 1
	RoleClosure Role = 2
)

// Valid reports whether role names a declared allocation owner.
func (role Role) Valid() bool {
	return role == RoleTable || role == RoleClosure
}

// Form is the stable ordinal of an allocation's constructor shape.
type Form uint8

const (
	FormEmpty     Form = 1
	FormClosed    Form = 2
	FormFinalOpen Form = 3
)

// Valid reports whether form names a declared constructor shape.
func (form Form) Valid() bool {
	return form >= FormEmpty && form <= FormFinalOpen
}

// Field is one immutable allocation-field geometry row.
type Field struct {
	id                   identity.ContentID
	kind                 FieldKind
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

// NewField constructs one valid immutable field row. Only key fields carry a
// selector span and may share their first value cell.
func NewField(id identity.ContentID, kind FieldKind, fieldSpan, selectorSpan, valuesSpan, valuesID identity.ContentID, width int, finalOpen, sharesFirstValueCell bool, normalized uint64, normalizedOK bool) (Field, bool) {
	row := Field{
		id: id, kind: kind, fieldSpan: fieldSpan, selectorSpan: selectorSpan,
		valuesSpan: valuesSpan, valuesID: valuesID, width: width,
		finalOpen: finalOpen, sharesFirstValueCell: sharesFirstValueCell,
		normalized: normalized, normalizedOK: normalizedOK,
	}
	return row, row.Available()
}

// Available reports whether row is a complete field proof.
func (row Field) Available() bool {
	return row.id.Available() && row.kind.Valid() && row.fieldSpan.Available() &&
		row.valuesSpan.Available() && row.valuesID.Available() && row.width >= 0 &&
		(row.kind == FieldKindKey) == row.selectorSpan.Available() &&
		(row.kind == FieldKindKey || !row.sharesFirstValueCell)
}

func (row Field) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}
func (row Field) Kind() FieldKind {
	if !row.Available() {
		return 0
	}
	return row.kind
}
func (row Field) FieldSpan() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.fieldSpan
}
func (row Field) SelectorSpan() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.selectorSpan
}
func (row Field) Values() (identity.ContentID, int, bool, bool) {
	if !row.Available() {
		return identity.ContentID{}, 0, false, false
	}
	return row.valuesSpan, row.width, row.finalOpen, true
}
func (row Field) Width() int {
	if !row.Available() {
		return 0
	}
	return row.width
}
func (row Field) FinalOpen() bool { return row.Available() && row.finalOpen }
func (row Field) ValuesID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.valuesID
}
func (row Field) SharesFirstValueCell() bool { return row.Available() && row.sharesFirstValueCell }
func (row Field) NormalizedKey() (uint64, bool) {
	if !row.Available() {
		return 0, false
	}
	return row.normalized, row.normalizedOK
}

// Allocation names one dense Field plane span.
type Allocation struct {
	id          identity.ContentID
	role        Role
	form        Form
	rootSpan    identity.ContentID
	fieldOffset uint32
	fieldCount  uint32
}

// NewAllocation constructs one valid immutable allocation row. Closures are
// always empty; tables are empty exactly when their field span is empty.
func NewAllocation(id identity.ContentID, role Role, form Form, rootSpan identity.ContentID, fieldOffset, fieldCount uint32) (Allocation, bool) {
	row := Allocation{id: id, role: role, form: form, rootSpan: rootSpan, fieldOffset: fieldOffset, fieldCount: fieldCount}
	return row, row.Available()
}

// Available reports whether row is a complete allocation proof.
func (row Allocation) Available() bool {
	if !row.id.Available() || !row.role.Valid() || !row.form.Valid() || !row.rootSpan.Available() || uint64(row.fieldOffset)+uint64(row.fieldCount) > uint64(^uint32(0)) {
		return false
	}
	if row.role == RoleClosure {
		return row.form == FormEmpty && row.fieldCount == 0
	}
	return row.form == FormEmpty && row.fieldCount == 0 || row.form != FormEmpty && row.fieldCount > 0
}

func (row Allocation) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}
func (row Allocation) Role() Role {
	if !row.Available() {
		return 0
	}
	return row.role
}
func (row Allocation) Form() Form {
	if !row.Available() {
		return 0
	}
	return row.form
}
func (row Allocation) RootSpan() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.rootSpan
}
func (row Allocation) FieldSpan() (uint32, uint32, bool) {
	return row.fieldOffset, row.fieldCount, row.Available()
}
func (row Allocation) FieldCount() int {
	if !row.Available() {
		return 0
	}
	return int(row.fieldCount)
}

// AllocationFamily binds the allocation plane to its manifest slot.
func AllocationFamily() programfamily.Family[Allocation] {
	return programfamily.New[Allocation](programcatalog.HeapAllocation())
}

// FieldFamily binds the field plane to its manifest slot.
func FieldFamily() programfamily.Family[Field] {
	return programfamily.New[Field](programcatalog.HeapField())
}

// TemplateID derives the exact historical allocation-template identity from
// canonical field geometry. Field spans, field identities, value identities,
// selectors, and share flags intentionally do not enter this preimage.
func TemplateID(occurrence identity.ContentID, role Role, form Form, fields []Field) identity.ContentID {
	if !occurrence.Available() || !templateShapeValid(role, form, fields) {
		return identity.ContentID{}
	}
	hash := sha256.New()
	var writer framing.Writer
	if writer.Reset(hash, "program/allocation-template", 2) != nil || writer.Record(1) != nil || writer.Bytes(occurrence[:]) != nil || writer.Uint(uint64(role)) != nil || writer.Uint(uint64(form)) != nil {
		return identity.ContentID{}
	}
	if role == RoleTable {
		if writer.Count(uint64(len(fields))) != nil {
			return identity.ContentID{}
		}
		for _, field := range fields {
			normalized, normalizedOK := field.NormalizedKey()
			if writer.Record(1) != nil || writer.Uint(uint64(field.Kind())) != nil || writer.Bool(normalizedOK) != nil {
				return identity.ContentID{}
			}
			if normalizedOK && writer.Uint(normalized) != nil {
				return identity.ContentID{}
			}
			if writer.Uint(uint64(field.Width())) != nil || writer.Bool(field.FinalOpen()) != nil {
				return identity.ContentID{}
			}
		}
	}
	if writer.Finish() != nil {
		return identity.ContentID{}
	}
	var id identity.ContentID
	copy(id[:], hash.Sum(nil))
	return id
}

// templateShapeValid binds the declared form to the canonical field sequence.
// A closure has no fields. An empty table has no fields. A non-empty table is
// final-open exactly when at least one field carries an open values tail.
func templateShapeValid(role Role, form Form, fields []Field) bool {
	if !role.Valid() || !form.Valid() {
		return false
	}
	if role == RoleClosure {
		return form == FormEmpty && len(fields) == 0
	}
	if len(fields) == 0 {
		return form == FormEmpty
	}
	if form == FormEmpty {
		return false
	}
	open := false
	for _, field := range fields {
		if !field.Available() {
			return false
		}
		open = open || field.FinalOpen()
	}
	return (form == FormFinalOpen) == open
}

// FieldID derives the exact historical allocation-field identity preimage.
func FieldID(programID, fieldProof identity.ContentID) identity.ContentID {
	if !programID.Available() || !fieldProof.Available() {
		return identity.ContentID{}
	}
	const prefix = "program-allocation-field-v1"
	var payload [len(prefix) + sha256.Size + sha256.Size]byte
	copy(payload[:len(prefix)], prefix)
	copy(payload[len(prefix):len(prefix)+sha256.Size], programID[:])
	copy(payload[len(prefix)+sha256.Size:], fieldProof[:])
	return sha256.Sum256(payload[:])
}
