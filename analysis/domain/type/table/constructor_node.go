package table

import "github.com/wippyai/go-lua/analysis/domain/type/typ"

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
