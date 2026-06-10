package typ

import "github.com/wippyai/go-lua/analysis/type/annotation"

// Field represents a record field with name, type, optionality, and mutability.
type Field struct {
	Name     string
	Type     Type
	Optional bool // True if field may be absent (nil access returns nil)
	Readonly bool // True if field cannot be reassigned
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
