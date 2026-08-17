package table

import (
	"github.com/wippyai/go-lua/domain/type/annotation"
	"github.com/wippyai/go-lua/domain/type/typ"
)

// RecordBuilder provides a fluent API for constructing Lua table record types.
type RecordBuilder struct {
	parts typ.RecordParts
}

// NewRecord starts building a record type with Lua table construction policy.
func NewRecord() *RecordBuilder {
	return &RecordBuilder{}
}

// Metatable sets the metatable type.
func (b *RecordBuilder) Metatable(t typ.Type) *RecordBuilder {
	b.parts.Metatable = t
	return b
}

// SetOpen marks the record as open.
func (b *RecordBuilder) SetOpen(open bool) *RecordBuilder {
	b.parts.Open = open
	return b
}

// MapComponent sets the map component key and value types with table-key
// normalization.
func (b *RecordBuilder) MapComponent(key, value typ.Type) *RecordBuilder {
	b.parts.MapKey = NormalizeKey(key)
	b.parts.MapValue = value
	return b
}

// Field adds a required field.
func (b *RecordBuilder) Field(name string, t typ.Type) *RecordBuilder {
	b.parts.Fields = append(b.parts.Fields, typ.Field{Name: name, Type: t})
	return b
}

// OptField adds an optional field.
func (b *RecordBuilder) OptField(name string, t typ.Type) *RecordBuilder {
	b.parts.Fields = append(b.parts.Fields, typ.Field{Name: name, Type: t, Optional: true})
	return b
}

// ReadonlyField adds a readonly field.
func (b *RecordBuilder) ReadonlyField(name string, t typ.Type) *RecordBuilder {
	b.parts.Fields = append(b.parts.Fields, typ.Field{Name: name, Type: t, Readonly: true})
	return b
}

// OptReadonlyField adds an optional readonly field.
func (b *RecordBuilder) OptReadonlyField(name string, t typ.Type) *RecordBuilder {
	b.parts.Fields = append(b.parts.Fields, typ.Field{Name: name, Type: t, Optional: true, Readonly: true})
	return b
}

// AnnotatedField adds a field with validation annotations.
func (b *RecordBuilder) AnnotatedField(name string, t typ.Type, optional bool, annotations []annotation.Annotation) *RecordBuilder {
	if len(annotations) > 0 {
		t = typ.NewAnnotated(t, annotations)
	}
	if optional {
		return b.OptField(name, t)
	}
	return b.Field(name, t)
}

// StaticStringIndex adds a required bracket-string member.
func (b *RecordBuilder) StaticStringIndex(name string, t typ.Type) *RecordBuilder {
	b.parts.StaticMembers = append(b.parts.StaticMembers, typ.StaticMember{Kind: typ.StaticMemberStringIndex, Name: name, Type: t})
	return b
}

// StaticIntIndex adds a required bracket-integer member.
func (b *RecordBuilder) StaticIntIndex(index int64, t typ.Type) *RecordBuilder {
	b.parts.StaticMembers = append(b.parts.StaticMembers, typ.StaticMember{Kind: typ.StaticMemberIntIndex, Index: index, Type: t})
	return b
}

// AddStaticMember adds a pre-built exact bracket member.
func (b *RecordBuilder) AddStaticMember(member typ.StaticMember) *RecordBuilder {
	b.parts.StaticMembers = append(b.parts.StaticMembers, member)
	return b
}

// Build creates the record type.
func (b *RecordBuilder) Build() *typ.Record {
	return RebuildRecord(b.parts)
}
