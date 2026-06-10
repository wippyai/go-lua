package typ

import (
	"sort"
	"strings"

	"github.com/wippyai/go-lua/analysis/type/kind"
)

// Field represents a record field with name, type, optionality, and mutability.
type Field struct {
	Name     string
	Type     Type
	Optional bool // True if field may be absent (nil access returns nil)
	Readonly bool // True if field cannot be reassigned
}

// Record represents a product type with named fields: {field1: T1, field2: T2, ...}.
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
	Fields                []Field
	StaticMembers         []StaticMember
	Metatable             Type // Metatable type for metamethod lookup
	MapKey                Type // Map component key type (nil if no map component)
	MapValue              Type // Map component value type (nil if no map component)
	Open                  bool // Allow access to undefined fields
	sorted                bool
	hash                  uint64
	containsAny           bool
	containsNever         bool
	containsTypeParam     bool
	containsInstantiated  bool
	containsRecursive     bool
	containsOpenRecursive bool
	strCache              stringCache
}

func (r *Record) Kind() kind.Kind { return kind.Record }

func (r *Record) Hash() uint64 { return r.hash }

func (r *Record) Equals(other Type) bool {
	return TypeEquals(r, other)
}

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
	return r.GetStaticMember(StaticMemberStringIndex, name, 0)
}

// GetStaticIntIndex returns the exact bracket-integer member with the given key,
// or nil when no such member is carried.
func (r *Record) GetStaticIntIndex(index int64) *StaticMember {
	return r.GetStaticMember(StaticMemberIntIndex, "", index)
}

// GetStaticMember returns the exact bracket member with the given key, or nil
// when no such member is carried.
func (r *Record) GetStaticMember(kind StaticMemberKind, name string, index int64) *StaticMember {
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
