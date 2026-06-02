package typ

import (
	"sort"
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/types/kind"
)

// Field represents a record field with name, type, optionality, and mutability.
type Field struct {
	Name     string
	Type     Type
	Optional bool // True if field may be absent (nil access returns nil)
	Readonly bool // True if field cannot be reassigned
}

// StaticMemberKind classifies an exact non-dot table member stored on a record.
type StaticMemberKind uint8

const (
	StaticMemberStringIndex StaticMemberKind = iota + 1
	StaticMemberIntIndex
)

// StaticMember represents a provably-present bracket member such as t["k"] or
// t[1]. It is separate from Field so dot-field shape and bracket-key facts do
// not collapse into one raw string namespace.
type StaticMember struct {
	Kind     StaticMemberKind
	Name     string
	Index    int64
	Type     Type
	Optional bool
	Readonly bool
}

// Record represents a Lua table with named fields: {field1: T1, field2: T2, ...}.
//
// Records support both structural typing (field presence/type matching) and
// optional map components for tables with dynamic indexing.
//
// Features:
//   - Open: When true, unknown field access returns Unknown instead of error
//   - MapKey/MapValue: Optional map component for {foo: T, [K]: V} patterns
//   - Metatable: Optional metatable type for metamethod resolution
//
// Fields are sorted by name for deterministic hashing and comparison.
type Record struct {
	Fields        []Field
	StaticMembers []StaticMember
	Metatable     Type // Metatable type for metamethod lookup
	MapKey        Type // Map component key type (nil if no map component)
	MapValue      Type // Map component value type (nil if no map component)
	Open          bool // Allow access to undefined fields
	// Fresh marks a transient empty-table-literal seed ({}). It is the gradual
	// bottom of the table lattice: invisible to IsSubtype (a Fresh empty record
	// behaves exactly as a closed empty record under <:) but admitted by
	// subtype.Consistent against any table-like target. Fresh is set only via
	// NewFreshEmptyRecord; rebuilt/merged records drop it.
	Fresh                 bool
	sorted                bool
	hash                  uint64
	softPrunable          bool
	containsAny           bool
	containsNever         bool
	containsTypeParam     bool
	containsInstantiated  bool
	containsRecursive     bool
	containsOpenRecursive bool
	containsCallableSurf  bool
	strCache              stringCache
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
func (b *RecordBuilder) AnnotatedField(name string, t Type, optional bool, annotations []Annotation) *RecordBuilder {
	if len(annotations) > 0 {
		t = NewAnnotated(t, annotations)
	}
	if optional {
		return b.OptField(name, t)
	}
	return b.Field(name, t)
}

// StaticStringIndex adds a required bracket-string member.
func (b *RecordBuilder) StaticStringIndex(name string, t Type) *RecordBuilder {
	b.staticMembers = append(b.staticMembers, StaticMember{Kind: StaticMemberStringIndex, Name: name, Type: t})
	return b
}

// StaticIntIndex adds a required bracket-integer member.
func (b *RecordBuilder) StaticIntIndex(index int64, t Type) *RecordBuilder {
	b.staticMembers = append(b.staticMembers, StaticMember{Kind: StaticMemberIntIndex, Index: index, Type: t})
	return b
}

// AddStaticMember adds a pre-built exact bracket member.
func (b *RecordBuilder) AddStaticMember(member StaticMember) *RecordBuilder {
	b.staticMembers = append(b.staticMembers, member)
	return b
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
	return buildRecordType(b.fields, b.staticMembers, b.metatable, b.mapKey, b.mapValue, b.open, false, false)
}

// NewFreshEmptyRecord creates the transient empty-table-literal seed ({}).
//
// It is a zero-field record (so under IsSubtype it is exactly the closed empty
// record), but Fresh is folded into hash/equals so it is distinct from an
// ordinary empty record. As the gradual bottom of the table lattice it is
// admitted by subtype.Consistent against any table-like target an empty table
// can satisfy (see emptyTableSatisfies).
func NewFreshEmptyRecord() *Record {
	return buildRecordType(nil, nil, nil, nil, nil, false, true, true)
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

		for i, member := range r.StaticMembers {
			if len(r.Fields) > 0 || i > 0 {
				sb.WriteString(", ")
			}
			if member.Readonly {
				sb.WriteString("readonly ")
			}
			writeStaticMemberKey(&sb, member)
			if member.Optional {
				sb.WriteString("?")
			}
			sb.WriteString(": ")
			if member.Type != nil {
				sb.WriteString(member.Type.String())
			} else {
				sb.WriteString("unknown")
			}
		}

		if r.HasMapComponent() {
			if len(r.Fields) > 0 || len(r.StaticMembers) > 0 {
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
			if len(r.Fields) > 0 || len(r.StaticMembers) > 0 || r.HasMapComponent() {
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

// GetStaticStringIndex returns the exact bracket-string member with the given
// key, or nil when no such member is carried.
func (r *Record) GetStaticStringIndex(name string) *StaticMember {
	return r.getStaticMember(StaticMemberStringIndex, name, 0)
}

// GetStaticIntIndex returns the exact bracket-integer member with the given key,
// or nil when no such member is carried.
func (r *Record) GetStaticIntIndex(index int64) *StaticMember {
	return r.getStaticMember(StaticMemberIntIndex, "", index)
}

func (r *Record) getStaticMember(kind StaticMemberKind, name string, index int64) *StaticMember {
	if r == nil || len(r.StaticMembers) == 0 {
		return nil
	}
	i := sort.Search(len(r.StaticMembers), func(i int) bool {
		return compareStaticMemberKey(r.StaticMembers[i], kind, name, index) >= 0
	})
	if i < len(r.StaticMembers) {
		member := &r.StaticMembers[i]
		if member.Kind == kind && member.Name == name && member.Index == index {
			return member
		}
	}
	return nil
}

func writeStaticMemberKey(sb *strings.Builder, member StaticMember) {
	switch member.Kind {
	case StaticMemberStringIndex:
		sb.WriteString("[\"")
		sb.WriteString(strings.ReplaceAll(member.Name, `"`, `\"`))
		sb.WriteString("\"]")
	case StaticMemberIntIndex:
		sb.WriteString("[")
		sb.WriteString(strconv.FormatInt(member.Index, 10))
		sb.WriteString("]")
	default:
		sb.WriteString("[unknown]")
	}
}
