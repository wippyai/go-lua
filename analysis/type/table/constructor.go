package table

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/type/typ"
)

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

type constructorNode struct {
	value         typ.Type
	sealed        bool
	fields        map[string]*constructorNode
	stringIndexes map[string]*constructorNode
	intIndexes    map[int64]*constructorNode
}

func newConstructorNode() *constructorNode {
	return &constructorNode{
		fields:        make(map[string]*constructorNode),
		stringIndexes: make(map[string]*constructorNode),
		intIndexes:    make(map[int64]*constructorNode),
	}
}

func (n *constructorNode) insert(path []ConstructorKey, t typ.Type, sealed bool) bool {
	if n == nil || len(path) == 0 || t == nil {
		return false
	}
	key := path[0]
	child, ok := n.childFor(key)
	if !ok {
		return false
	}
	if len(path) == 1 {
		if sealed {
			child.value = t
			child.sealed = true
			child.fields = nil
			child.stringIndexes = nil
			child.intIndexes = nil
		} else if !child.sealed {
			child.value = t
		}
		return true
	}
	if child.sealed {
		return true
	}
	return child.insert(path[1:], t, sealed)
}

func (n *constructorNode) childFor(key ConstructorKey) (*constructorNode, bool) {
	switch key.Kind {
	case ConstructorField:
		if key.Name == "" {
			return nil, false
		}
		child := n.fields[key.Name]
		if child == nil {
			child = newConstructorNode()
			n.fields[key.Name] = child
		}
		return child, true
	case ConstructorStringIndex:
		if key.Name == "" {
			return nil, false
		}
		child := n.stringIndexes[key.Name]
		if child == nil {
			child = newConstructorNode()
			n.stringIndexes[key.Name] = child
		}
		return child, true
	case ConstructorIntIndex:
		if key.Index < 0 {
			return nil, false
		}
		child := n.intIndexes[key.Index]
		if child == nil {
			child = newConstructorNode()
			n.intIndexes[key.Index] = child
		}
		return child, true
	default:
		return nil, false
	}
}

func (n *constructorNode) build() (typ.Type, bool) {
	if n == nil {
		return nil, false
	}
	if len(n.fields) == 0 && len(n.stringIndexes) == 0 && len(n.intIndexes) == 0 {
		return n.value, n.value != nil
	}
	if len(n.fields) == 0 && len(n.stringIndexes) == 0 && len(n.intIndexes) > 0 {
		indexes := sortedConstructorIntKeys(n.intIndexes)
		elems := make([]typ.Type, 0, len(indexes))
		for _, idx := range indexes {
			t, ok := n.intIndexes[idx].build()
			if !ok {
				return nil, false
			}
			elems = append(elems, t)
		}
		if len(elems) == 0 {
			return nil, false
		}
		return typ.NewTuple(elems...), true
	}
	builder := NewRecord()
	for _, name := range sortedConstructorStringKeys(n.fields) {
		t, ok := n.fields[name].build()
		if !ok {
			return nil, false
		}
		builder.Field(name, t)
	}
	for _, name := range sortedConstructorStringKeys(n.stringIndexes) {
		t, ok := n.stringIndexes[name].build()
		if !ok {
			return nil, false
		}
		builder.StaticStringIndex(name, t)
	}
	for _, idx := range sortedConstructorIntKeys(n.intIndexes) {
		t, ok := n.intIndexes[idx].build()
		if !ok {
			return nil, false
		}
		builder.StaticIntIndex(idx, t)
	}
	return builder.Build(), true
}

func sortedConstructorStringKeys(values map[string]*constructorNode) []string {
	if len(values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedConstructorIntKeys(values map[int64]*constructorNode) []int64 {
	if len(values) == 0 {
		return nil
	}
	keys := make([]int64, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}
