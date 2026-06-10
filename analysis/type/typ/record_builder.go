package typ

import "github.com/wippyai/go-lua/analysis/type/annotation"

// RecordBuilder provides a fluent API for constructing record types.
//
// Example:
//
//	rec := typ.NewRecord().
//	    Field("name", typ.String).
//	    OptField("age", typ.Integer).
//	    Build()
type RecordBuilder struct {
	fields        []Field
	staticMembers []StaticMember
	metatable     Type
	mapKey        Type
	mapValue      Type
	open          bool
}

// NewRecord starts building a record type.
func NewRecord() *RecordBuilder {
	return &RecordBuilder{}
}

// Metatable sets the metatable type.
func (b *RecordBuilder) Metatable(t Type) *RecordBuilder {
	b.metatable = t
	return b
}

// SetOpen marks the record as open (unknown field access returns unknown).
func (b *RecordBuilder) SetOpen(open bool) *RecordBuilder {
	b.open = open
	return b
}

// MapComponent sets the map component key and value types.
func (b *RecordBuilder) MapComponent(key, value Type) *RecordBuilder {
	b.mapKey = key
	b.mapValue = value
	return b
}

// Build creates the record type.
func (b *RecordBuilder) Build() *Record {
	return buildRecordType(b.fields, b.staticMembers, b.metatable, b.mapKey, b.mapValue, b.open, false)
}

// Field adds a required field.
func (b *RecordBuilder) Field(name string, t Type) *RecordBuilder {
	b.fields = append(b.fields, Field{Name: name, Type: t})
	return b
}

// OptField adds an optional field.
func (b *RecordBuilder) OptField(name string, t Type) *RecordBuilder {
	b.fields = append(b.fields, Field{Name: name, Type: t, Optional: true})
	return b
}

// ReadonlyField adds a readonly field.
func (b *RecordBuilder) ReadonlyField(name string, t Type) *RecordBuilder {
	b.fields = append(b.fields, Field{Name: name, Type: t, Readonly: true})
	return b
}

// OptReadonlyField adds an optional readonly field.
func (b *RecordBuilder) OptReadonlyField(name string, t Type) *RecordBuilder {
	b.fields = append(b.fields, Field{Name: name, Type: t, Optional: true, Readonly: true})
	return b
}

// AnnotatedField adds a field with validation annotations.
func (b *RecordBuilder) AnnotatedField(name string, t Type, optional bool, annotations []annotation.Annotation) *RecordBuilder {
	if len(annotations) > 0 {
		t = NewAnnotated(t, annotations)
	}
	if optional {
		return b.OptField(name, t)
	}
	return b.Field(name, t)
}
