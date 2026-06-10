package table

import (
	"github.com/wippyai/go-lua/analysis/type/annotation"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// NewFreshArray creates a fresh empty-array table seed.
func NewFreshArray() *typ.Array {
	return typ.NewFreshArray()
}

// NewFreshEmptyRecord creates a fresh empty-record table seed.
func NewFreshEmptyRecord() *typ.Record {
	return typ.NewFreshEmptyRecord()
}

// NewMap creates a map type with table-key normalization.
func NewMap(key, value typ.Type) *typ.Map {
	return typ.NewMap(NormalizeKey(key), value)
}

// NewReadonlyMap creates a read-only map type with table-key normalization.
func NewReadonlyMap(key, value typ.Type) *typ.ReadonlyMap {
	return typ.NewReadonlyMap(NormalizeKey(key), value)
}

// RecordBuilder provides a fluent API for constructing Lua table record types.
type RecordBuilder struct {
	inner *typ.RecordBuilder
}

// NewRecord starts building a record type with Lua table construction policy.
func NewRecord() *RecordBuilder {
	return &RecordBuilder{inner: typ.NewRecord()}
}

// Metatable sets the metatable type.
func (b *RecordBuilder) Metatable(t typ.Type) *RecordBuilder {
	b.inner.Metatable(t)
	return b
}

// SetOpen marks the record as open.
func (b *RecordBuilder) SetOpen(open bool) *RecordBuilder {
	b.inner.SetOpen(open)
	return b
}

// MapComponent sets the map component key and value types with table-key
// normalization.
func (b *RecordBuilder) MapComponent(key, value typ.Type) *RecordBuilder {
	b.inner.MapComponent(NormalizeKey(key), value)
	return b
}

// Field adds a required field.
func (b *RecordBuilder) Field(name string, t typ.Type) *RecordBuilder {
	b.inner.Field(name, t)
	return b
}

// OptField adds an optional field.
func (b *RecordBuilder) OptField(name string, t typ.Type) *RecordBuilder {
	b.inner.OptField(name, t)
	return b
}

// ReadonlyField adds a readonly field.
func (b *RecordBuilder) ReadonlyField(name string, t typ.Type) *RecordBuilder {
	b.inner.ReadonlyField(name, t)
	return b
}

// OptReadonlyField adds an optional readonly field.
func (b *RecordBuilder) OptReadonlyField(name string, t typ.Type) *RecordBuilder {
	b.inner.OptReadonlyField(name, t)
	return b
}

// AnnotatedField adds a field with validation annotations.
func (b *RecordBuilder) AnnotatedField(name string, t typ.Type, optional bool, annotations []annotation.Annotation) *RecordBuilder {
	b.inner.AnnotatedField(name, t, optional, annotations)
	return b
}

// StaticStringIndex adds a required bracket-string member.
func (b *RecordBuilder) StaticStringIndex(name string, t typ.Type) *RecordBuilder {
	b.inner.StaticStringIndex(name, t)
	return b
}

// StaticIntIndex adds a required bracket-integer member.
func (b *RecordBuilder) StaticIntIndex(index int64, t typ.Type) *RecordBuilder {
	b.inner.StaticIntIndex(index, t)
	return b
}

// AddStaticMember adds a pre-built exact bracket member.
func (b *RecordBuilder) AddStaticMember(member typ.StaticMember) *RecordBuilder {
	b.inner.AddStaticMember(member)
	return b
}

// Build creates the record type.
func (b *RecordBuilder) Build() *typ.Record {
	return b.inner.Build()
}

// RebuildRecord rebuilds a record with table-key normalization for its map
// component.
func RebuildRecord(parts typ.RecordParts) *typ.Record {
	return typ.RebuildRecord(RecordPartsWithMapKeyNormalization(parts))
}

// RecordPartsWithMapKeyNormalization returns a copy of parts with the map key
// normalized when a map component key is present.
func RecordPartsWithMapKeyNormalization(parts typ.RecordParts) typ.RecordParts {
	if parts.MapKey != nil {
		parts.MapKey = NormalizeKey(parts.MapKey)
	}
	return parts
}
