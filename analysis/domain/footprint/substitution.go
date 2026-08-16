package footprint

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/heap"
)

// Substitution is exact Heap-scoped structural-root transport. It may be
// non-injective: collisions join their symbolic bounds.
type Substitution struct {
	source heap.Schema
	roots  map[heap.Key]heap.Key
}

func NewSubstitution(source heap.Schema, pairs [][2]heap.Key) (Substitution, bool) {
	if !source.Valid() {
		return Substitution{}, false
	}
	out := Substitution{source: source, roots: make(map[heap.Key]heap.Key, len(pairs))}
	for _, pair := range pairs {
		if !source.OwnsKey(pair[0]) || pair[0].Kind() != heap.RootAllocation {
			return Substitution{}, false
		}
		if !source.OwnsKey(pair[1]) || pair[1].Kind() != heap.RootAllocation {
			return Substitution{}, false
		}
		if _, duplicate := out.roots[pair[0]]; duplicate {
			return Substitution{}, false
		}
		out.roots[pair[0]] = pair[1]
	}
	return out, true
}

func (s Substitution) Root(root heap.Key) heap.Key {
	if replacement, ok := s.roots[root]; ok {
		return replacement
	}
	return root
}

func (s Substitution) Key(key Key) (Key, bool) {
	if key.Kind() != KeyAllocation {
		return key, s.source.Valid() && key.universe != nil && key.universe.heap == s.source && key.valid()
	}
	if !s.source.Valid() || key.universe == nil || key.universe.heap != s.source {
		return Key{}, false
	}
	root, ok := key.HeapKey()
	if !ok {
		return Key{}, false
	}
	slot, ok := key.universe.rootIndex[s.Root(root)]
	if !ok {
		return Key{}, false
	}
	return Key{universe: key.universe, slot: slot}, true
}

type Fact struct {
	Key   Key
	Value Value
}

// Substitute transports one key-local fact inside the same family algebra.
// There is no second owner or adapter path.
func (a Algebra) Substitute(fact Fact, substitution Substitution) (Fact, bool) {
	if !a.valid() || substitution.source != a.schema.universe.heap || !a.schema.universe.ownsKey(fact.Key) || !a.accepts(fact.Value) {
		return Fact{}, false
	}
	destination, ok := substitution.Key(fact.Key)
	if !ok {
		return Fact{}, false
	}
	if !a.schema.universe.ownsKey(destination) {
		return Fact{}, false
	}
	if fact.Value.IsTop() {
		return Fact{Key: destination, Value: a.Top()}, true
	}
	if fact.Value.IsBottom() {
		return Fact{Key: destination, Value: a.Default()}, true
	}

	nodes := make([]node, len(fact.Value.nodes))
	for index, stored := range fact.Value.nodes {
		declared, ok := a.schema.universe.rootAt(stored.root)
		if !ok {
			return Fact{}, false
		}
		root, ok := a.schema.universe.rootIndex[substitution.Root(declared)]
		if !ok {
			return Fact{}, false
		}
		nodes[index] = node{root: root, Objects: stored.Objects, Elements: stored.Elements, Capacity: stored.Capacity}
	}
	sort.Slice(nodes, func(left, right int) bool { return nodes[left].root < nodes[right].root })
	mergedNodes := nodes[:0]
	for _, candidate := range nodes {
		if len(mergedNodes) == 0 || mergedNodes[len(mergedNodes)-1].root != candidate.root {
			mergedNodes = append(mergedNodes, candidate)
			continue
		}
		previous := &mergedNodes[len(mergedNodes)-1]
		previous.Objects = joinBound(previous.Objects, candidate.Objects)
		previous.Elements = joinBound(previous.Elements, candidate.Elements)
		previous.Capacity = joinBound(previous.Capacity, candidate.Capacity)
	}
	storedEdges := make([]edge, len(fact.Value.edges))
	for index, stored := range fact.Value.edges {
		from, fromOK := a.schema.universe.rootAt(stored.from)
		to, toOK := a.schema.universe.rootAt(stored.to)
		if !fromOK || !toOK {
			return Fact{}, false
		}
		mappedFrom, fromOK := a.schema.universe.rootIndex[substitution.Root(from)]
		mappedTo, toOK := a.schema.universe.rootIndex[substitution.Root(to)]
		if !fromOK || !toOK {
			return Fact{}, false
		}
		storedEdges[index] = edge{from: mappedFrom, to: mappedTo}
	}
	sort.Slice(storedEdges, func(left, right int) bool { return lessEdge(storedEdges[left], storedEdges[right]) })
	mergedEdges := storedEdges[:0]
	for _, candidate := range storedEdges {
		if len(mergedEdges) == 0 || mergedEdges[len(mergedEdges)-1] != candidate {
			mergedEdges = append(mergedEdges, candidate)
		}
	}
	value := Value{universe: a.schema.universe, nodes: mergedNodes, edges: mergedEdges}
	return Fact{Key: destination, Value: value}, true
}
