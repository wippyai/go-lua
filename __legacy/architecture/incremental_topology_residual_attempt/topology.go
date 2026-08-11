// Package topology owns the monotone structural graph used by one Solver
// generation. It knows only stable dense nodes and two semantic edge
// provenances. It neither assigns recurrence heads nor interprets a domain.
package topology

import (
	"errors"
	"sort"
)

// Node is one stable append-only graph identity. A Node never changes meaning
// during a Graph generation; it is not a semantic Program identity.
type Node int

// Edge records one directed influence. Ordinary participates in the residual
// acyclicity law. Boundary records an explicit semantic boundary. An edge may
// carry both provenances; such a mixed edge participates in both graphs.
type Edge struct {
	From     Node
	To       Node
	Ordinary bool
	Boundary bool
}

// Change is the exact structural delta from one accepted Apply. Affected and
// Merged contain canonical Node order. Merged is non-empty only when the full
// graph condensation coalesced components. Sweep is the post-Apply residual
// order view.
type Change struct {
	Inserted bool
	Affected []Node
	Merged   []Node
	Sweep    Sweep
}

// Sweep is an expiring read-only view over a Graph generation's residual
// order. It exposes no component identity or mutator.
type Sweep struct {
	graph   *Graph
	version uint64
}

// Len reports the number of residual nodes in this view. It returns zero once
// the Graph advances, so a prior schedule cannot be used after topology grows.
func (s Sweep) Len() int {
	if s.graph == nil || s.graph.version != s.version {
		return 0
	}
	return s.graph.residual.Len()
}

// At returns the node at one deterministic residual sweep position.
func (s Sweep) At(index int) (Node, bool) {
	if s.graph == nil || s.graph.version != s.version {
		return 0, false
	}
	entry := s.graph.residual.At(index)
	if entry == nil {
		return 0, false
	}
	return Node(entry.id), true
}

// ComponentSweep is an expiring view of full-graph condensation order. Each
// item is the least member of one current SCC; it is a presentation handle,
// not a stable component identity.
type ComponentSweep struct {
	graph   *Graph
	version uint64
}

// Len reports the number of current full-graph components.
func (s ComponentSweep) Len() int {
	if s.graph == nil || s.graph.version != s.version {
		return 0
	}
	return s.graph.full.Len()
}

// At reports one component's canonical representative in full-edge order.
func (s ComponentSweep) At(index int) (Node, bool) {
	if s.graph == nil || s.graph.version != s.version {
		return 0, false
	}
	entry := s.graph.full.At(index)
	if entry == nil {
		return 0, false
	}
	root := s.graph.find(entry.id)
	return s.graph.nodes[root].first, true
}

// AppendMembers appends the denoted component's members in current residual
// order. Callers own dst and can reuse its capacity across sweeps; this avoids
// a per-member selection/cursor layer in the evaluator hot path.
func (s ComponentSweep) AppendMembers(dst []Node, head Node) ([]Node, bool) {
	if s.graph == nil || s.graph.version != s.version || !s.graph.valid(head) {
		return dst, false
	}
	root := s.graph.find(int(head))
	if s.graph.nodes[root].first != head {
		return dst, false
	}
	return appendMemberOrder(dst, s.graph.nodes[root].memberRoot), true
}

var (
	// ErrInvalidBatch marks a negative node count or a graph whose mutation
	// serial is exhausted. It is returned before any part of Apply is visible.
	ErrInvalidBatch = errors.New("topology: invalid batch")
	// ErrInvalidEdge marks an unknown endpoint or an edge with no provenance.
	ErrInvalidEdge = errors.New("topology: invalid edge")
	// ErrResidualCycle marks an ordinary or mixed edge that would make the
	// residual graph cyclic. Boundary-only cycles remain valid full-graph SCCs.
	ErrResidualCycle = errors.New("topology: residual cycle")
	// ErrExhausted marks a stable-node or view-serial space that cannot grow
	// without wrapping. The graph fails closed rather than reusing an identity.
	ErrExhausted = errors.New("topology: exhausted")
)

// Graph incrementally maintains a full condensation and an ordinary residual
// DAG. It is one Solver-generation, single-writer structural cache: accepted
// topology is monotone and survives cancellation; support/reachability do not
// live here.
type Graph struct {
	nodes []nodeState
	edges map[edgeKey]edgeFlags

	residual blockOrder
	full     blockOrder

	stamp         uint64
	searchStack   []int
	memberScratch []*memberNode
	version       uint64
}

type nodeState struct {
	parent int
	size   int
	first  Node
	cyclic bool

	// Condensation adjacency is presence-only. The graph only inserts raw
	// edges, so a multiplicity cannot affect reachability or later contraction.
	out map[int]struct{}
	in  map[int]struct{}

	// Residual adjacency remains raw-node owned even when full components
	// contract: boundary edges may form SCCs but ordinary edges never cycle.
	residualOut []Node
	residualIn  []Node

	residualEntry *orderEntry
	fullEntry     *orderEntry
	member        *memberNode
	memberRoot    *memberNode // live only on a full-component root

	residualForward uint64
	residualBackward uint64
	fullForward     uint64
	fullBackward    uint64
}

type edgeKey struct{ from, to Node }

type edgeFlags struct{ ordinary, boundary bool }

// New constructs an empty topology generation.
func New() *Graph { return &Graph{edges: make(map[edgeKey]edgeFlags)} }

// NodeCount reports the currently retained stable node prefix.
func (g *Graph) NodeCount() int {
	if g == nil {
		return 0
	}
	return len(g.nodes)
}

// Edge reports the complete provenance currently attached to from -> to.
func (g *Graph) Edge(from, to Node) (Edge, bool) {
	if !g.valid(from) || !g.valid(to) {
		return Edge{}, false
	}
	flags, ok := g.edges[edgeKey{from: from, to: to}]
	if !ok {
		return Edge{}, false
	}
	return Edge{From: from, To: to, Ordinary: flags.ordinary, Boundary: flags.boundary}, true
}

// Component reports the canonical least Node of node's current full-graph SCC
// and whether that SCC is cyclic. The representative is a query result, never
// a stable semantic identity.
func (g *Graph) Component(node Node) (head Node, cyclic bool, ok bool) {
	if !g.valid(node) {
		return 0, false, false
	}
	root := g.find(int(node))
	part := &g.nodes[root]
	return part.first, part.cyclic, true
}

// Members returns a fresh canonical numeric copy of node's current full SCC.
// Scheduling uses ComponentSweep.AppendMembers instead, which is residual
// ordered and caller-buffered.
func (g *Graph) Members(node Node) []Node {
	if !g.valid(node) {
		return nil
	}
	root := g.find(int(node))
	result := appendMemberOrder(nil, g.nodes[root].memberRoot)
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	return result
}

// ResidualSweep returns the current deterministic residual topological order.
// The returned view expires after a successful Apply.
func (g *Graph) ResidualSweep() Sweep {
	if g == nil {
		return Sweep{}
	}
	return Sweep{graph: g, version: g.version}
}

// FullSweep returns the current deterministic full-condensation order. The
// view expires after a successful Apply.
func (g *Graph) FullSweep() ComponentSweep {
	if g == nil {
		return ComponentSweep{}
	}
	return ComponentSweep{graph: g, version: g.version}
}

func (g *Graph) valid(node Node) bool { return g != nil && node >= 0 && int(node) < len(g.nodes) }

func (g *Graph) find(index int) int {
	for g.nodes[index].parent != index {
		index = g.nodes[index].parent
	}
	return index
}

func (g *Graph) nextStamp() uint64 {
	if g.stamp == ^uint64(0) {
		for index := range g.nodes {
			g.nodes[index].residualForward = 0
			g.nodes[index].residualBackward = 0
			g.nodes[index].fullForward = 0
			g.nodes[index].fullBackward = 0
		}
		g.stamp = 0
	}
	g.stamp++
	return g.stamp
}

func canonicalNodes(nodes ...Node) []Node {
	if len(nodes) == 0 {
		return nil
	}
	result := append([]Node(nil), nodes...)
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	end := 0
	for _, node := range result {
		if end != 0 && result[end-1] == node {
			continue
		}
		result[end] = node
		end++
	}
	return result[:end]
}

func unionNodes(left, right []Node) []Node {
	result := make([]Node, 0, len(left)+len(right))
	result = append(result, left...)
	result = append(result, right...)
	return canonicalNodes(result...)
}
