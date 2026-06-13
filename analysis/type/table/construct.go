package table

import (
	"github.com/wippyai/go-lua/analysis/type/annotation"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// NewMap creates a map type with table-key normalization.
func NewMap(key, value typ.Type) *typ.Map {
	return typ.RebuildMap(NormalizeKey(key), value)
}

// NewReadonlyMap creates a read-only map type with table-key normalization.
func NewReadonlyMap(key, value typ.Type) *typ.ReadonlyMap {
	return typ.RebuildReadonlyMap(NormalizeKey(key), value)
}

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

// RebuildRecord rebuilds a record with table-key normalization for its map
// component and nil/absence normalization for optional field/static-member
// payloads.
func RebuildRecord(parts typ.RecordParts) *typ.Record {
	return typ.RebuildRecord(recordPartsWithTableNormalization(parts))
}

// recordPartsWithTableNormalization returns a copy of parts with Lua table
// construction policy applied.
func recordPartsWithTableNormalization(parts typ.RecordParts) typ.RecordParts {
	parts = recordPartsWithMapKeyNormalization(parts)
	parts.Fields = fieldsWithOptionalPayloadsSplit(parts.Fields)
	parts.StaticMembers = staticMembersWithOptionalPayloadsSplit(parts.StaticMembers)
	return parts
}

// recordPartsWithMapKeyNormalization returns a copy of parts with the map key
// normalized when a map component key is present.
func recordPartsWithMapKeyNormalization(parts typ.RecordParts) typ.RecordParts {
	if parts.MapKey != nil {
		parts.MapKey = NormalizeKey(parts.MapKey)
	}
	return parts
}

// fieldsWithOptionalPayloadsSplit returns fields with nilable optional payloads
// split into absent-vs-present table shape.
func fieldsWithOptionalPayloadsSplit(fields []typ.Field) []typ.Field {
	var out []typ.Field
	for i, field := range fields {
		normalized := fieldWithOptionalPayloadSplit(field)
		if out == nil {
			if sameFieldShape(normalized, field) {
				continue
			}
			out = make([]typ.Field, 0, len(fields))
			out = append(out, fields[:i]...)
		}
		out = append(out, normalized)
	}
	if out == nil {
		return fields
	}
	return out
}

// fieldWithOptionalPayloadSplit returns field with nilable optional payloads
// split into absent-vs-present table shape.
func fieldWithOptionalPayloadSplit(field typ.Field) typ.Field {
	if !field.Optional {
		return field
	}
	if inner, optional := splitNilableFieldType(field.Type); optional {
		field.Type = inner
		field.Optional = true
	}
	return field
}

// staticMembersWithOptionalPayloadsSplit returns static members with nilable
// optional payloads split into absent-vs-present table shape.
func staticMembersWithOptionalPayloadsSplit(members []typ.StaticMember) []typ.StaticMember {
	var out []typ.StaticMember
	for i, member := range members {
		normalized := staticMemberWithOptionalPayloadSplit(member)
		if out == nil {
			if sameStaticMemberShape(normalized, member) {
				continue
			}
			out = make([]typ.StaticMember, 0, len(members))
			out = append(out, members[:i]...)
		}
		out = append(out, normalized)
	}
	if out == nil {
		return members
	}
	return out
}

// staticMemberWithOptionalPayloadSplit returns member with nilable optional
// payloads split into absent-vs-present table shape.
func staticMemberWithOptionalPayloadSplit(member typ.StaticMember) typ.StaticMember {
	if !member.Optional {
		return member
	}
	if inner, optional := splitNilableFieldType(member.Type); optional {
		member.Type = inner
		member.Optional = true
	}
	return member
}

func sameFieldShape(a, b typ.Field) bool {
	return a.Name == b.Name &&
		a.Optional == b.Optional &&
		a.Readonly == b.Readonly &&
		typ.SameNode(a.Type, b.Type)
}

func sameStaticMemberShape(a, b typ.StaticMember) bool {
	return a.Kind == b.Kind &&
		a.Name == b.Name &&
		a.Index == b.Index &&
		a.Optional == b.Optional &&
		a.Readonly == b.Readonly &&
		typ.SameNode(a.Type, b.Type)
}
