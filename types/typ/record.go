package typ

import (
	"sort"
	"strings"

	"github.com/wippyai/go-lua/types/kind"
)

// Field represents a record field with name, type, optionality, and mutability.
type Field struct {
	Name     string
	Type     Type
	Optional bool // True if field may be absent (nil access returns nil)
	// InferredPresence marks absence inferred from a possible write to a
	// locally built table. Gradual reads use Type; declared optionality and
	// explicit nil values still contribute nil to the read type.
	InferredPresence bool
	Readonly         bool // True if field cannot be reassigned
}

// Record represents a Lua table with named fields: {field1: T1, field2: T2, ...}.
//
// Records support both structural typing (field presence/type matching) and
// optional map components for tables with dynamic indexing.
//
// Features:
//   - Open: When true, unknown field access returns Unknown instead of error
//   - Complete: When true, the record lists every field its table holds; a
//     field it lacks reads as nil
//   - Declared: When true, the shape came from a type annotation, type alias,
//     or a module manifest rather than inference. An absent field read on a
//     declared record is an error; on an inferred record it reads gradually.
//   - MapKey/MapValue: Optional map component for {foo: T, [K]: V} patterns
//   - Metatable: Optional metatable type for metamethod resolution
//
// Fields are sorted by name for deterministic hashing and comparison.
type Record struct {
	Fields              []Field
	Metatable           Type // Metatable type for metamethod lookup
	MapKey              Type // Map component key type (nil if no map component)
	MapValue            Type // Map component value type (nil if no map component)
	MapInferredPresence bool
	MapExplicitNilWrite bool
	Open                bool // Allow access to undefined fields
	// Complete marks a record that lists every field its table holds, such as
	// the type of a table literal built in the checked module: a field it
	// lacks reads as nil. A record without it describes its value partially,
	// and a field it lacks reads as unknown.
	Complete     bool
	Declared     bool // Shape came from a declaration, not inference
	sorted       bool
	hash         uint64
	softPrunable bool
	strCache     stringCache
}

// RecordBuilder provides a fluent API for constructing record types.
//
// Example:
//
//	rec := typ.NewRecord().
//	    Field("name", typ.String).
//	    OptField("age", typ.Integer).
//	    Build()
type RecordBuilder struct {
	fields              []Field
	metatable           Type
	mapKey              Type
	mapValue            Type
	mapInferredPresence bool
	mapExplicitNilWrite bool
	open                bool
	complete            bool
	declared            bool
}

// NewRecord starts building a record type.
func NewRecord() *RecordBuilder {
	return &RecordBuilder{}
}

// Builder copies every semantic property of r for a metadata-preserving rebuild.
func (r *Record) Builder() *RecordBuilder {
	return &RecordBuilder{
		fields: append([]Field(nil), r.Fields...), metatable: r.Metatable,
		mapKey: r.MapKey, mapValue: r.MapValue,
		mapInferredPresence: r.MapInferredPresence,
		mapExplicitNilWrite: r.MapExplicitNilWrite,
		open:                r.Open, complete: r.Complete, declared: r.Declared,
	}
}

// BuilderEmptyFields keeps record metadata while allowing callers to rebuild fields.
func (r *Record) BuilderEmptyFields() *RecordBuilder {
	b := r.Builder()
	b.fields = nil
	return b
}

// WithChildren replaces child types while retaining all metadata of r and its fields.
func (r *Record) WithChildren(fields []Field, metatable, mapKey, mapValue Type) *Record {
	b := r.Builder()
	b.fields = append([]Field(nil), fields...)
	b.metatable, b.mapKey, b.mapValue = metatable, mapKey, mapValue
	return b.Build()
}

func (r *Record) WithComplete(complete bool) *Record {
	b := r.Builder()
	b.complete = complete
	return b.Build()
}

// Field adds a required field.
func (b *RecordBuilder) Field(name string, t Type) *RecordBuilder {
	b.fields = append(b.fields, Field{Name: name, Type: t})
	return b
}

// AddField preserves all field metadata when rebuilding a record.
func (b *RecordBuilder) AddField(f Field) *RecordBuilder {
	b.fields = append(b.fields, f)
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
func (b *RecordBuilder) AnnotatedField(name string, t Type, optional bool, annotations []Annotation) *RecordBuilder {
	if len(annotations) > 0 {
		t = NewAnnotated(t, annotations)
	}
	if optional {
		return b.OptField(name, t)
	}
	return b.Field(name, t)
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

// SetComplete marks the record as listing every field its table holds.
func (b *RecordBuilder) SetComplete(complete bool) *RecordBuilder {
	b.complete = complete
	return b
}

// joinedComplete reports whether a record joining a and b lists every field
// of its table: both inputs must.
func joinedComplete(a, b *Record) bool {
	return a.Complete && b.Complete
}

// JoinedComplete reports whether a record joining a and b lists every field
// of its table: both inputs must.
func JoinedComplete(a, b *Record) bool {
	return joinedComplete(a, b)
}

// SetDeclared marks the record shape as coming from a declaration (a type
// annotation, type alias, or module manifest) rather than inference.
func (b *RecordBuilder) SetDeclared(declared bool) *RecordBuilder {
	b.declared = declared
	return b
}

// MapComponent sets the map component key and value types.
func (b *RecordBuilder) MapComponent(key, value Type) *RecordBuilder {
	b.mapKey = key
	b.mapValue = value
	b.mapInferredPresence = false
	b.mapExplicitNilWrite = false
	return b
}

func (b *RecordBuilder) MapComponentWithFlags(key, value Type, inferred, explicitNil bool) *RecordBuilder {
	b.mapKey, b.mapValue = key, value
	b.mapInferredPresence, b.mapExplicitNilWrite = inferred, explicitNil
	return b
}

// Build creates the record type.
func (b *RecordBuilder) Build() *Record {
	return buildRecordTypeWithFlags(b.fields, b.metatable, b.mapKey, b.mapValue, b.open, b.declared, false, b.mapInferredPresence, b.mapExplicitNilWrite, b.complete)
}

// WithMetatable returns r with meta as its metatable and everything else kept.
func (r *Record) WithMetatable(meta Type) *Record {
	return r.WithChildren(r.Fields, meta, r.MapKey, r.MapValue)
}

// WithDeclared returns r with its declaration provenance set to declared.
// Rebuilding through the builder keeps the flag in the hash and equality.
func (r *Record) WithDeclared(declared bool) *Record {
	if r.Declared == declared {
		return r
	}
	b := r.Builder()
	b.declared = declared
	return b.Build()
}

// WithField returns r with f replacing the field of the same name, or added
// when r has no such field; everything else is kept.
func (r *Record) WithField(f Field) *Record {
	fields := make([]Field, 0, len(r.Fields)+1)
	replaced := false
	for _, existing := range r.Fields {
		if existing.Name == f.Name {
			fields = append(fields, f)
			replaced = true
			continue
		}
		fields = append(fields, existing)
	}
	if !replaced {
		fields = append(fields, f)
	}
	return r.WithChildren(fields, r.Metatable, r.MapKey, r.MapValue)
}

func (r *Record) Kind() kind.Kind { return kind.Record }

func (r *Record) String() string {
	return r.strCache.get(func() string {
		var sb strings.Builder

		sb.WriteString("{")

		for i, f := range r.Fields {
			if i > 0 {
				sb.WriteString(", ")
			}

			if f.Readonly {
				sb.WriteString("readonly ")
			}

			sb.WriteString(f.Name)

			if f.Optional {
				sb.WriteString("?")
			}

			sb.WriteString(": ")
			if f.Type != nil {
				sb.WriteString(f.Type.String())
			} else {
				sb.WriteString("unknown")
			}
		}

		if r.HasMapComponent() {
			if len(r.Fields) > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString("[")
			if r.MapKey != nil {
				sb.WriteString(r.MapKey.String())
			} else {
				sb.WriteString("unknown")
			}
			sb.WriteString("]: ")
			if r.MapValue != nil {
				sb.WriteString(r.MapValue.String())
			} else {
				sb.WriteString("unknown")
			}
		}

		if r.Open {
			if len(r.Fields) > 0 || r.HasMapComponent() {
				sb.WriteString(", ")
			}
			sb.WriteString("...")
		}

		sb.WriteString("}")

		return sb.String()
	})
}

func (r *Record) Hash() uint64 { return r.hash }

func (r *Record) Equals(other Type) bool {
	return TypeEquals(r, other)
}

// HasMapComponent returns true if the record has a map component (MapKey and MapValue set).
func (r *Record) HasMapComponent() bool {
	return r.MapKey != nil && r.MapValue != nil
}

// HasSameFieldNames reports whether r and other declare the same field names
// and agree on having a map component.
func (r *Record) HasSameFieldNames(other *Record) bool {
	if len(r.Fields) != len(other.Fields) || r.HasMapComponent() != other.HasMapComponent() {
		return false
	}
	for _, f := range r.Fields {
		if other.GetField(f.Name) == nil {
			return false
		}
	}
	return true
}

// GetField returns the field with the given name, or nil.
func (r *Record) GetField(name string) *Field {
	if r.sorted {
		i := sort.Search(len(r.Fields), func(i int) bool {
			return r.Fields[i].Name >= name
		})
		if i < len(r.Fields) && r.Fields[i].Name == name {
			return &r.Fields[i]
		}
		return nil
	}

	for i := range r.Fields {
		if r.Fields[i].Name == name {
			return &r.Fields[i]
		}
	}

	return nil
}

// PartialView returns t with its top-level complete records marked partial:
// the value t describes may hold fields the record misses, as a method
// receiver is any table that uses the method table.
func PartialView(t Type) Type {
	switch v := t.(type) {
	case *Record:
		if !v.Complete {
			return t
		}
		return v.WithComplete(false)
	case *Alias:
		target := PartialView(v.Target)
		if target == v.Target {
			return t
		}
		return NewAlias(v.Name, target)
	case *Optional:
		inner := PartialView(v.Inner)
		if inner == v.Inner {
			return t
		}
		return NewOptional(inner)
	case *Union:
		members := make([]Type, len(v.Members))
		changed := false
		for i, m := range v.Members {
			members[i] = PartialView(m)
			changed = changed || members[i] != m
		}
		if !changed {
			return t
		}
		return NewUnion(members...)
	}
	return t
}

// PartialViewDeep marks every complete record reachable in t partial: the
// tables t describes are written by code outside the view, as a module export
// is written by its importers.
func PartialViewDeep(t Type) Type {
	if t == nil {
		return nil
	}
	return Rewrite(t, func(node Type) (Type, bool) {
		if r, ok := node.(*Record); ok && r.Complete {
			return PartialViewDeep(r.WithComplete(false)), true
		}
		return nil, false
	})
}
