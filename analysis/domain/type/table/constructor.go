package table

import "github.com/wippyai/go-lua/analysis/domain/type/typ"

type ConstructorKeyKind uint8

const (
	// ConstructorField represents a dot-style record field.
	ConstructorField ConstructorKeyKind = iota + 1
	// ConstructorStringIndex represents an exact bracket-string member.
	ConstructorStringIndex
	// ConstructorIntIndex represents an exact bracket-integer member.
	ConstructorIntIndex
)

// ConstructorKey identifies one static constructor path segment.
type ConstructorKey struct {
	Kind  ConstructorKeyKind
	Name  string
	Index int64
}

// ConstructorEntry contributes one static path and type to a table constructor.
type ConstructorEntry struct {
	Path   []ConstructorKey
	Type   typ.Type
	Sealed bool
}

// ConstructorBuilder assembles nested table constructor paths into a type.
type ConstructorBuilder struct {
	root *constructorNode
}

// NewConstructorBuilder starts an empty table constructor type builder.
func NewConstructorBuilder() *ConstructorBuilder {
	return &ConstructorBuilder{root: newConstructorNode()}
}

// ConstructorType builds a type from constructor entries.
func ConstructorType(entries []ConstructorEntry) (typ.Type, bool) {
	builder := NewConstructorBuilder()
	seen := false
	for _, entry := range entries {
		ok := false
		if entry.Sealed {
			ok = builder.AddSealed(entry.Path, entry.Type)
		} else {
			ok = builder.Add(entry.Path, entry.Type)
		}
		if !ok {
			return nil, false
		}
		seen = true
	}
	if !seen {
		return nil, false
	}
	return builder.Build()
}

// Add contributes an unsealed static constructor path.
func (b *ConstructorBuilder) Add(path []ConstructorKey, t typ.Type) bool {
	if b == nil {
		return false
	}
	return b.root.insert(path, t, false)
}

// AddSealed contributes a path whose declared type must not be narrowed by
// descendant constructor entries.
func (b *ConstructorBuilder) AddSealed(path []ConstructorKey, t typ.Type) bool {
	if b == nil {
		return false
	}
	return b.root.insert(path, t, true)
}

// Build returns the assembled constructor type.
func (b *ConstructorBuilder) Build() (typ.Type, bool) {
	if b == nil {
		return nil, false
	}
	return b.root.build()
}
