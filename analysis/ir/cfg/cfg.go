// Package cfg provides control flow graph representation for dataflow analysis.
//
// A control flow graph (CFG) models the flow of control through a function body.
// Each node represents a program point (assignment, branch, call, etc.) and edges
// represent possible control flow between points.
//
// # Core Types
//
// Point: An index identifying a location in the CFG. Points are local to each
// function's CFG and serve as keys for type state maps.
//
// Node: Contains only the program point and node kind. Language-specific facts
// live in sidecars outside this package.
//
// Edge: A directed edge from one point to another. For branch edges, Cond is the
// taken-branch flag.
//
// Graph: Interface for CFG access, implemented by CFG. Provides node lookup,
// edge traversal, predecessor/successor queries, and reverse post-order iteration.
//
// # Symbol Identity
//
// symbol.ID identifies a lexical binding. It distinguishes shadowed declarations
// but does not identify assignments. SSA definition versions are computed by a
// separate IR layer over the CFG.
//
// # Analysis Support
//
// The CFG supports forward dataflow analysis via RPO (reverse post-order) traversal.
// Join and branch topology is exposed through predecessor/successor shape.
package cfg

import (
	"fmt"
	"sync/atomic"
)

// Point represents a location in a control flow graph.
// Each Point is an index into the CFG's node array, local to that CFG.
type Point uint32

// NodeKind identifies the type of CFG node.
type NodeKind uint8

// Node kind constants identify the type of each CFG node.
const (
	NodeEntry  NodeKind = iota // Function entry point, always first node
	NodeExit                   // Function exit point, always last node
	NodeAssign                 // Assignment statement (local x = expr)
	NodeCall                   // Function call expression
	NodeBranch                 // Conditional branch (if, while, for condition)
	NodeJoin                   // Join point where multiple paths merge
	NodeReturn                 // Return statement
	NodeNoop                   // Structural no-op used where a source statement needs a CFG point
)

// Node represents a CFG topology node.
type Node struct {
	Point Point
	Kind  NodeKind
}

// Edge represents a directed control flow edge between two points.
//
// For branch nodes (NodeBranch), the Cond field distinguishes branches:
//   - Cond=true: the "then" branch, taken when condition is truthy
//   - Cond=false: the "else" branch, taken when condition is falsy
//
// For non-branch edges, Cond has no semantic meaning.
type Edge struct {
	From Point
	To   Point
	Cond bool // true = then branch, false = else branch
}

// Graph is the interface for control flow graphs.
//
// This interface abstracts over different CFG implementations for dataflow
// consumers.
//
// Key methods for dataflow analysis:
//   - RPO(): Returns nodes in reverse post-order for forward analysis
//   - Predecessors/Successors: Enable backward/forward traversal
//   - EdgeCond: Retrieves branch edge direction
//   - IsJoin/IsBranch: Identifies merge points and decision points
type Graph interface {
	ID() uint64                           // Process-local identifier for this CFG instance
	Entry() Point                         // Function entry point (always node 0)
	Exit() Point                          // Function exit point
	Node(p Point) *Node                   // Node metadata at point p
	RPO() []Point                         // Reverse post-order for forward analysis
	Predecessors(p Point) []Point         // Incoming edges
	Successors(p Point) []Point           // All successors (branch nodes have 2)
	Edges() []Edge                        // All edges in the graph
	Size() int                            // Number of nodes
	EdgeCond(from, to Point) (bool, bool) // Branch taken flag for an edge
	IsJoin(p Point) bool                  // True if p has multiple predecessors
	IsBranch(p Point) bool                // True if p has multiple successors
}

type readOnlySuccessorGraph interface {
	SuccessorsReadOnly(Point) []Point
}

type readOnlyPredecessorGraph interface {
	PredecessorsReadOnly(Point) []Point
}

type readOnlyRPOGraph interface {
	RPOReadOnly() []Point
}

// CFG represents the control flow graph for a function.
//
// A CFG is built during AST analysis and consumed by flow-sensitive analysis.
// The graph is immutable after construction.
//
// Structure:
//   - nodes: flat array indexed by Point
//   - Edges: directed edges with predecessor/successor maps for efficient lookup
//   - Entry/Exit: special nodes for function boundaries
//
// The CFG is built incrementally via AddNode/AddEdge, then traversed via
// RPO for forward dataflow analysis.
type CFG struct {
	id        uint64
	entry     Point
	exit      Point
	nodes     []Node
	edges     []Edge
	preds     [][]Point
	succs     [][]Point
	succConds [][]bool
	rpo       []Point
}

var cfgCounter uint64

func nextCFGID() uint64 {
	return atomic.AddUint64(&cfgCounter, 1)
}

// New creates an empty CFG.
func New() *CFG {
	return NewWithCapacity(0, 0)
}

// NewWithCapacity creates an empty CFG with initial node/edge capacity hints.
func NewWithCapacity(nodeCap, edgeCap int) *CFG {
	if nodeCap < 2 {
		nodeCap = 2
	}
	if edgeCap < 0 {
		edgeCap = 0
	}

	c := &CFG{
		id:        nextCFGID(),
		nodes:     make([]Node, 0, nodeCap),
		edges:     make([]Edge, 0, edgeCap),
		preds:     make([][]Point, 0, nodeCap),
		succs:     make([][]Point, 0, nodeCap),
		succConds: make([][]bool, 0, nodeCap),
	}
	c.entry = c.AddNode(NodeEntry)
	c.exit = c.AddNode(NodeExit)
	return c
}

// Entry returns the entry point.
func (c *CFG) Entry() Point {
	if c == nil {
		return 0
	}
	return c.entry
}

// Exit returns the exit point.
func (c *CFG) Exit() Point {
	if c == nil {
		return 0
	}
	return c.exit
}

// ID returns the process-local identifier for this CFG instance.
func (c *CFG) ID() uint64 {
	if c == nil {
		return 0
	}
	return c.id
}

// AddNode adds a node and returns its point.
func (c *CFG) AddNode(kind NodeKind) Point {
	c.invalidateRPO()
	p := Point(len(c.nodes))
	c.nodes = append(c.nodes, Node{Point: p, Kind: kind})
	c.ensureAdjacencyLen(len(c.nodes))
	return p
}

// AddEdge adds an edge.
func (c *CFG) AddEdge(from, to Point, cond bool) {
	if c == nil {
		panic("cfg: AddEdge on nil CFG")
	}
	fromIdx := int(from)
	toIdx := int(to)
	if fromIdx < 0 || fromIdx >= len(c.nodes) {
		panic(fmt.Sprintf("cfg: edge from nonexistent point %d", from))
	}
	if toIdx < 0 || toIdx >= len(c.nodes) {
		panic(fmt.Sprintf("cfg: edge to nonexistent point %d", to))
	}

	c.invalidateRPO()
	c.edges = append(c.edges, Edge{From: from, To: to, Cond: cond})
	succs := c.succs[fromIdx]
	if succs == nil {
		succs = make([]Point, 0, 2)
	}
	succs = append(succs, to)
	c.succs[fromIdx] = succs

	conds := c.succConds[fromIdx]
	if conds == nil {
		conds = make([]bool, 0, 2)
	}
	conds = append(conds, cond)
	c.succConds[fromIdx] = conds

	preds := c.preds[toIdx]
	if preds == nil {
		preds = make([]Point, 0, 2)
	}
	preds = append(preds, from)
	c.preds[toIdx] = preds
}

// Node returns the node at point p.
func (c *CFG) Node(p Point) *Node {
	if int(p) < len(c.nodes) {
		return &c.nodes[p]
	}
	return nil
}

// NodeSnapshot returns a copy of all nodes in point order.
func (c *CFG) NodeSnapshot() []Node {
	if c == nil || len(c.nodes) == 0 {
		return nil
	}
	out := make([]Node, len(c.nodes))
	copy(out, c.nodes)
	return out
}

// Predecessors returns all predecessors of p.
func (c *CFG) Predecessors(p Point) []Point {
	return copyEdges(c.preds, p)
}

// PredecessorsReadOnly returns the internal predecessor slice for p.
//
// The returned slice must be treated as read-only by callers.
func (c *CFG) PredecessorsReadOnly(p Point) []Point {
	return edgesReadOnly(c.preds, p)
}

// Successors returns all successors of p.
func (c *CFG) Successors(p Point) []Point {
	return copyEdges(c.succs, p)
}

// copyEdges returns a fresh copy of table[p], or nil when p is out of range or
// has no edges.
func copyEdges(table [][]Point, p Point) []Point {
	edges := edgesReadOnly(table, p)
	if len(edges) == 0 {
		return nil
	}
	result := make([]Point, len(edges))
	copy(result, edges)
	return result
}

// edgesReadOnly returns the internal slice table[p] without copying, or nil when
// p is out of range. Callers must treat the result as read-only.
func edgesReadOnly(table [][]Point, p Point) []Point {
	idx := int(p)
	if idx < 0 || idx >= len(table) {
		return nil
	}
	return table[idx]
}

// SuccessorsReadOnly returns the internal successor slice for p.
//
// The returned slice must be treated as read-only by callers.
func (c *CFG) SuccessorsReadOnly(p Point) []Point {
	return edgesReadOnly(c.succs, p)
}

// SuccessorsReadOnly returns graph's successor slice without copying when the
// implementation exposes an immutable adjacency view. Generic Graph
// implementations fall back to the copy-preserving Successors contract.
func SuccessorsReadOnly(graph Graph, p Point) []Point {
	if graph == nil {
		return nil
	}
	if ro, ok := graph.(readOnlySuccessorGraph); ok {
		return ro.SuccessorsReadOnly(p)
	}
	return graph.Successors(p)
}

// PredecessorsReadOnly returns graph's predecessor slice without copying when
// available, preserving Graph's copy semantics as the fallback.
func PredecessorsReadOnly(graph Graph, p Point) []Point {
	if graph == nil {
		return nil
	}
	if ro, ok := graph.(readOnlyPredecessorGraph); ok {
		return ro.PredecessorsReadOnly(p)
	}
	return graph.Predecessors(p)
}

// RPOReadOnly returns graph's reverse post-order without copying when
// available, preserving Graph's copy semantics as the fallback.
func RPOReadOnly(graph Graph) []Point {
	if graph == nil {
		return nil
	}
	if ro, ok := graph.(readOnlyRPOGraph); ok {
		return ro.RPOReadOnly()
	}
	return graph.RPO()
}

// PointCanReach reports whether control can flow from point from to point to by
// following successor edges of graph. A point reaches itself. The entry point is
// a valid CFG point even though it is normally numbered 0.
func PointCanReach(graph Graph, from, to Point) bool {
	if graph == nil {
		return false
	}
	if from == to {
		return true
	}
	seen := map[Point]struct{}{from: {}}
	stack := append([]Point(nil), SuccessorsReadOnly(graph, from)...)
	for len(stack) != 0 {
		point := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if point == to {
			return true
		}
		if _, ok := seen[point]; ok {
			continue
		}
		seen[point] = struct{}{}
		stack = append(stack, SuccessorsReadOnly(graph, point)...)
	}
	return false
}

// IsJoin returns true if p has multiple predecessors.
func (c *CFG) IsJoin(p Point) bool {
	idx := int(p)
	return idx >= 0 && idx < len(c.preds) && len(c.preds[idx]) > 1
}

// IsBranch returns true if p has multiple successors.
func (c *CFG) IsBranch(p Point) bool {
	idx := int(p)
	return idx >= 0 && idx < len(c.succs) && len(c.succs[idx]) > 1
}

// Edges returns all edges.
func (c *CFG) Edges() []Edge {
	if len(c.edges) == 0 {
		return nil
	}
	result := make([]Edge, len(c.edges))
	copy(result, c.edges)
	return result
}

// Size returns node count.
func (c *CFG) Size() int {
	return len(c.nodes)
}

// EdgeCond returns the branch taken flag for edge from->to.
// Returns (true, ok) for then-branch, (false, ok) for else-branch.
func (c *CFG) EdgeCond(from, to Point) (bool, bool) {
	idx := int(from)
	if idx < 0 || idx >= len(c.succs) {
		return false, false
	}
	succs := c.succs[idx]
	conds := c.succConds[idx]
	for i, succ := range succs {
		if succ != to {
			continue
		}
		if i >= len(conds) {
			return false, false
		}
		return conds[i], true
	}
	return false, false
}

// AddBranch adds a branch node.
func (c *CFG) AddBranch() Point {
	return c.AddNode(NodeBranch)
}

// RPO returns nodes in reverse post-order for forward dataflow analysis.
//
// Reverse post-order (RPO) is the optimal traversal order for forward dataflow
// analysis: predecessors are visited before successors (except for back-edges
// in loops). This ensures that type state propagates correctly through the
// control flow graph.
//
// Only nodes reachable from the entry point are included. Unreachable code
// (after unconditional return, for example) is excluded.
func (c *CFG) RPO() []Point {
	ro := c.RPOReadOnly()
	if len(ro) == 0 {
		return nil
	}
	out := make([]Point, len(ro))
	copy(out, ro)
	return out
}

// RPOReadOnly returns cached reverse post-order without making a copy.
//
// The returned slice must be treated as read-only by callers.
func (c *CFG) RPOReadOnly() []Point {
	if c == nil {
		return nil
	}
	if len(c.rpo) > 0 {
		return c.rpo
	}

	n := len(c.nodes)
	visited := make([]bool, n)
	order := make([]Point, 0, n)

	var visit func(p Point)
	visit = func(p Point) {
		if int(p) >= n || visited[p] {
			return
		}
		visited[p] = true
		for _, succ := range c.succs[p] {
			visit(succ)
		}
		order = append(order, p)
	}

	visit(c.entry)

	// Reverse to get RPO
	for i, j := 0, len(order)-1; i < j; i, j = i+1, j-1 {
		order[i], order[j] = order[j], order[i]
	}
	c.rpo = make([]Point, len(order))
	copy(c.rpo, order)
	return c.rpo
}

func (c *CFG) invalidateRPO() {
	if c == nil {
		return
	}
	c.rpo = nil
}

func (c *CFG) ensureAdjacencyLen(n int) {
	if c == nil || n <= len(c.preds) {
		return
	}
	grow := n - len(c.preds)
	c.preds = append(c.preds, make([][]Point, grow)...)
	c.succs = append(c.succs, make([][]Point, grow)...)
	c.succConds = append(c.succConds, make([][]bool, grow)...)
}
