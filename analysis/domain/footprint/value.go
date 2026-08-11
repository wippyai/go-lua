package footprint

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/heap"
)

// Node is one abstract object rooted at an opaque structural allocation root.
type Node struct {
	Root     heap.Key
	Objects  Bound
	Elements Bound
	Capacity Bound
}

// Edge is one sparse may-containment edge between canonical Link roots.
type Edge struct {
	From heap.Key
	To   heap.Key
}

type node struct {
	root     uint32
	Objects  Bound
	Elements Bound
	Capacity Bound
}

type edge struct{ from, to uint32 }

// Value is Bottom, a finite normalized graph, or Top. Hot values contain only
// dense schema-local coordinates and never duplicate structural Link handles.
type Value struct {
	universe *universe
	unknown  bool
	nodes    []node
	edges    []edge
}

func bottom(universe *universe) Value { return Value{universe: universe} }
func top(universe *universe) Value    { return Value{universe: universe, unknown: true} }

func (v Value) IsBottom() bool { return !v.unknown && len(v.nodes) == 0 && len(v.edges) == 0 }
func (v Value) IsTop() bool    { return v.unknown }

func (v Value) Nodes() []Node {
	if v.universe == nil {
		return nil
	}
	result := make([]Node, len(v.nodes))
	for i, stored := range v.nodes {
		root, ok := v.universe.rootAt(stored.root)
		if !ok {
			return nil
		}
		result[i] = Node{Root: root, Objects: stored.Objects, Elements: stored.Elements, Capacity: stored.Capacity}
	}
	return result
}

func (v Value) Edges() []Edge {
	if v.universe == nil {
		return nil
	}
	result := make([]Edge, len(v.edges))
	for i, stored := range v.edges {
		from, fromOK := v.universe.rootAt(stored.from)
		to, toOK := v.universe.rootAt(stored.to)
		if !fromOK || !toOK {
			return nil
		}
		result[i] = Edge{From: from, To: to}
	}
	return result
}

func equalValue(left, right Value) bool {
	if left.universe != right.universe || left.unknown != right.unknown || len(left.nodes) != len(right.nodes) || len(left.edges) != len(right.edges) {
		return false
	}
	for i := range left.nodes {
		if left.nodes[i] != right.nodes[i] {
			return false
		}
	}
	for i := range left.edges {
		if left.edges[i] != right.edges[i] {
			return false
		}
	}
	return true
}

func normalize(universe *universe, nodes []Node, edges []Edge) (Value, bool) {
	if len(nodes) == 0 && len(edges) == 0 {
		return bottom(universe), true
	}
	storedNodes := make([]node, len(nodes))
	for i, value := range nodes {
		coordinate, ok := universe.rootIndex[value.Root]
		if !ok || !validBound(value.Objects) || !validBound(value.Elements) || !validBound(value.Capacity) {
			return Value{}, false
		}
		storedNodes[i] = node{root: coordinate, Objects: value.Objects, Elements: value.Elements, Capacity: value.Capacity}
	}
	sort.Slice(storedNodes, func(i, j int) bool { return storedNodes[i].root < storedNodes[j].root })
	for i := 1; i < len(storedNodes); i++ {
		if storedNodes[i-1].root == storedNodes[i].root {
			return Value{}, false
		}
	}
	storedEdges := make([]edge, len(edges))
	for i, value := range edges {
		from, fromOK := universe.rootIndex[value.From]
		to, toOK := universe.rootIndex[value.To]
		candidate := edge{from: from, to: to}
		if !fromOK || !toOK || !containsNode(storedNodes, from) || !containsNode(storedNodes, to) {
			return Value{}, false
		}
		storedEdges[i] = candidate
	}
	sort.Slice(storedEdges, func(i, j int) bool { return lessEdge(storedEdges[i], storedEdges[j]) })
	unique := storedEdges[:0]
	for _, candidate := range storedEdges {
		if len(unique) == 0 || unique[len(unique)-1] != candidate {
			unique = append(unique, candidate)
		}
	}
	return Value{universe: universe, nodes: storedNodes, edges: unique}, true
}

func containsNode(nodes []node, root uint32) bool {
	i := sort.Search(len(nodes), func(i int) bool { return nodes[i].root >= root })
	return i < len(nodes) && nodes[i].root == root
}

func nodeAt(nodes []node, root uint32) (node, bool) {
	i := sort.Search(len(nodes), func(i int) bool { return nodes[i].root >= root })
	if i >= len(nodes) || nodes[i].root != root {
		return node{}, false
	}
	return nodes[i], true
}

func lessEdge(left, right edge) bool {
	return left.from < right.from || left.from == right.from && left.to < right.to
}

func containsEdge(edges []edge, candidate edge) bool {
	i := sort.Search(len(edges), func(i int) bool { return !lessEdge(edges[i], candidate) })
	return i < len(edges) && edges[i] == candidate
}

func lessValue(left, right Value) bool {
	if left.universe == nil || left.universe != right.universe {
		return false
	}
	if left.unknown || right.unknown {
		return right.unknown
	}
	for _, value := range left.nodes {
		other, ok := nodeAt(right.nodes, value.root)
		if !ok || !lessBound(value.Objects, other.Objects) || !lessBound(value.Elements, other.Elements) || !lessBound(value.Capacity, other.Capacity) {
			return false
		}
	}
	for _, value := range left.edges {
		if !containsEdge(right.edges, value) {
			return false
		}
	}
	return true
}

func joinValue(left, right Value) Value {
	if left.unknown || right.unknown {
		return top(left.universe)
	}
	if left.IsBottom() {
		return right
	}
	if right.IsBottom() {
		return left
	}
	nodes := make([]node, 0, len(left.nodes)+len(right.nodes))
	li, ri := 0, 0
	for li < len(left.nodes) && ri < len(right.nodes) {
		l, r := left.nodes[li], right.nodes[ri]
		switch {
		case l.root < r.root:
			nodes = append(nodes, l)
			li++
		case r.root < l.root:
			nodes = append(nodes, r)
			ri++
		default:
			nodes = append(nodes, node{root: l.root, Objects: joinBound(l.Objects, r.Objects), Elements: joinBound(l.Elements, r.Elements), Capacity: joinBound(l.Capacity, r.Capacity)})
			li++
			ri++
		}
	}
	nodes = append(nodes, left.nodes[li:]...)
	nodes = append(nodes, right.nodes[ri:]...)
	return Value{universe: left.universe, nodes: nodes, edges: joinEdges(left.edges, right.edges)}
}

func joinEdges(left, right []edge) []edge {
	result := make([]edge, 0, len(left)+len(right))
	li, ri := 0, 0
	for li < len(left) && ri < len(right) {
		switch {
		case left[li] == right[ri]:
			result = append(result, left[li])
			li++
			ri++
		case lessEdge(left[li], right[ri]):
			result = append(result, left[li])
			li++
		default:
			result = append(result, right[ri])
			ri++
		}
	}
	result = append(result, left[li:]...)
	return append(result, right[ri:]...)
}

func meetValue(left, right Value) Value {
	if left.unknown {
		return right
	}
	if right.unknown {
		return left
	}
	nodes := make([]node, 0, min(len(left.nodes), len(right.nodes)))
	li, ri := 0, 0
	for li < len(left.nodes) && ri < len(right.nodes) {
		l, r := left.nodes[li], right.nodes[ri]
		switch {
		case l.root < r.root:
			li++
		case r.root < l.root:
			ri++
		default:
			objects, ook := meetBound(l.Objects, r.Objects)
			elements, eok := meetBound(l.Elements, r.Elements)
			capacity, cok := meetBound(l.Capacity, r.Capacity)
			if ook && eok && cok {
				nodes = append(nodes, node{root: l.root, Objects: objects, Elements: elements, Capacity: capacity})
			}
			li++
			ri++
		}
	}
	edges := make([]edge, 0, min(len(left.edges), len(right.edges)))
	li, ri = 0, 0
	for li < len(left.edges) && ri < len(right.edges) {
		switch {
		case left.edges[li] == right.edges[ri]:
			edges = append(edges, left.edges[li])
			li++
			ri++
		case lessEdge(left.edges[li], right.edges[ri]):
			li++
		default:
			ri++
		}
	}
	filtered := edges[:0]
	for _, candidate := range edges {
		if containsNode(nodes, candidate.from) && containsNode(nodes, candidate.to) {
			filtered = append(filtered, candidate)
		}
	}
	return Value{universe: left.universe, nodes: nodes, edges: filtered}
}

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}
