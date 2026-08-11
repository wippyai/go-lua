// Package schedule builds the canonical nested weak-topological ordering for
// one finite dense solver graph. It is deliberately independent of Program
// and domains: the equation compiler supplies dense action identities, their
// canonical semantic order, and directed influences.
package schedule

import (
	"errors"
	"fmt"
	"sort"
)

// Node is a dense action identity in [0, nodeCount). Prepare uses the identity
// order for compatibility; PrepareOrdered lets the caller supply the
// canonical semantic order independently of this dense representation.
type Node int

// Edge is one directed influence from From to To.
type Edge struct {
	From Node
	To   Node
}

// ErrInvalidEdge marks an edge whose endpoint is outside the supplied dense
// graph. Invalid inputs are rejected before any schedule is published.
var ErrInvalidEdge = errors.New("schedule: invalid edge")

// ErrInvalidOrder marks semantic node ranks that do not form one complete
// permutation of [0, nodeCount).
var ErrInvalidOrder = errors.New("schedule: invalid semantic node order")

// errWTOConstruction is deliberately private: a cyclic projected frontier is
// an implementation-invariant failure, never a supplied graph error.
// Prepare fails closed rather than publishing a partial or reordered schedule.
var errWTOConstruction = errors.New("schedule: invalid constructed WTO")

// EventKind distinguishes structural recurrence brackets from ordinary node
// work. Enter and Exit name the same cyclic Region. An EventEnter is not a
// widening or restart instruction: only the compiled graph and evaluator,
// after binding that exact Region to its recurrence interface, may derive an
// iteration action from it.
type EventKind uint8

const (
	EventEnter EventKind = iota + 1
	EventNode
	EventExit
)

// NoRegion marks an acyclic root-level Node event.
const NoRegion = -1

// Event is one canonical Solver scheduling action. Node is the feedback head
// for Enter/Exit and the transferred node for EventNode.
type Event struct {
	Kind   EventKind
	Node   Node
	Region int
}

// Region is one nested recurrence cut in the immutable event stream. Enter
// and Exit index its exact brackets. Heads and nesting derive only from the
// canonical semantic node order and the influence graph.
type Region struct {
	Head   Node
	Parent int
	Enter  int
	Exit   int
}

// Schedule is an immutable nested weak-topological ordering.
type Schedule struct {
	nodes   int
	events  []Event
	regions []Region
}

// Prepare constructs a schedule using the dense Node identity as its semantic
// order. It is retained as the identity-order compatibility constructor.
func Prepare(nodeCount int, edges []Edge) (*Schedule, error) {
	return PrepareOrdered(nodeCount, edges, identityRanks(nodeCount))
}

// PrepareOrdered constructs a schedule using semanticRanks[node] as the
// canonical rank of that dense Node. Ranks must be a permutation of
// [0, nodeCount), where lower ranks win every otherwise-free structural tie
// break. The rank slice is copied before construction so subsequent caller
// mutation cannot change the published schedule.
func PrepareOrdered(nodeCount int, edges []Edge, semanticRanks []int) (*Schedule, error) {
	if nodeCount < 0 {
		return nil, fmt.Errorf("%w: negative node count %d", ErrInvalidEdge, nodeCount)
	}
	order, err := newNodeOrder(nodeCount, semanticRanks)
	if err != nil {
		return nil, err
	}
	graph, err := normalizedGraph(nodeCount, edges, order)
	if err != nil {
		return nil, err
	}
	schedule := &Schedule{nodes: nodeCount}
	if err := schedule.buildWTO(graph, order); err != nil {
		return nil, err
	}
	return schedule, nil
}

type nodeOrder struct {
	ranks []int
	nodes []Node
}

func identityRanks(nodeCount int) []int {
	if nodeCount < 0 {
		return nil
	}
	ranks := make([]int, nodeCount)
	for node := range ranks {
		ranks[node] = node
	}
	return ranks
}

func newNodeOrder(nodeCount int, semanticRanks []int) (nodeOrder, error) {
	if len(semanticRanks) != nodeCount {
		return nodeOrder{}, fmt.Errorf("%w: want %d ranks, got %d", ErrInvalidOrder, nodeCount, len(semanticRanks))
	}
	ranks := append([]int(nil), semanticRanks...)
	nodes := make([]Node, nodeCount)
	seen := make([]bool, nodeCount)
	for node, rank := range ranks {
		if rank < 0 || rank >= nodeCount {
			return nodeOrder{}, fmt.Errorf("%w at node %d: rank %d outside [0, %d)", ErrInvalidOrder, node, rank, nodeCount)
		}
		if seen[rank] {
			return nodeOrder{}, fmt.Errorf("%w at node %d: duplicate rank %d", ErrInvalidOrder, node, rank)
		}
		seen[rank] = true
		nodes[rank] = Node(node)
	}
	return nodeOrder{ranks: ranks, nodes: nodes}, nil
}

func (order nodeOrder) less(left, right Node) bool {
	return order.ranks[int(left)] < order.ranks[int(right)]
}

func normalizedGraph(nodeCount int, edges []Edge, order nodeOrder) ([][]Node, error) {
	graph := make([][]Node, nodeCount)
	for index, edge := range edges {
		if edge.From < 0 || edge.To < 0 || int(edge.From) >= nodeCount || int(edge.To) >= nodeCount {
			return nil, fmt.Errorf("%w at %d: %d -> %d for %d nodes", ErrInvalidEdge, index, edge.From, edge.To, nodeCount)
		}
		graph[edge.From] = append(graph[edge.From], edge.To)
	}
	for _, node := range order.nodes {
		index := int(node)
		graph[index] = sortedUniqueDense(graph[index], order)
	}
	return graph, nil
}

// NodeCount reports the dense graph cardinality used to build this schedule.
func (s *Schedule) NodeCount() int {
	if s == nil {
		return 0
	}
	return s.nodes
}

// EventCount reports the number of enter/node/exit actions.
func (s *Schedule) EventCount() int {
	if s == nil {
		return 0
	}
	return len(s.events)
}

// EventAt returns one action in canonical scheduling order.
func (s *Schedule) EventAt(index int) (Event, bool) {
	if s == nil || index < 0 || index >= len(s.events) {
		return Event{}, false
	}
	return s.events[index], true
}

// RegionCount reports the number of published feedback regions.
func (s *Schedule) RegionCount() int {
	if s == nil {
		return 0
	}
	return len(s.regions)
}

// RegionAt returns one published feedback region.
func (s *Schedule) RegionAt(index int) (Region, bool) {
	if s == nil || index < 0 || index >= len(s.regions) {
		return Region{}, false
	}
	return s.regions[index], true
}

// buildWTO constructs a Bourdoncle-equivalent hierarchy by the bottom-up
// loop-nesting-forest method, then emits its sole event representation.  It
// uses the bottom-up loop-nesting construction behind ConstructWTOBU from Kim,
// Venet and Thakur (POPL 2020): a canonical iterative DFS establishes the
// forest, union-find discovers every nested recurrence once, and one
// canonical-min Kahn extension orders each containment frontier. There is no
// residual-graph SCC loop and no second schedule representation.
//
// The loop construction is O((V+E) α(V)) after canonical adjacency is
// available. Canonical-min frontier selection adds O(V log V) in the worst
// case. Offline Tarjan LCA and loop representatives are union-find based, each
// edge is classified/restored/projected once, and no semantic traversal
// recurses on the Go stack. PrepareOrdered sorts arbitrary edge spellings per
// source to establish canonical adjacency.
func (s *Schedule) buildWTO(graph [][]Node, order nodeOrder) error {
	forest := buildForest(graph, order)
	parents, components := loopNestingForest(graph, forest)
	children, ok := orderFrontiers(graph, parents, components, order)
	if !ok {
		return errWTOConstruction
	}
	s.emitFrontiers(children, components)
	return nil
}

type dfsForest struct {
	// parent includes one private virtual root at index len(graph).  It gives
	// cross-tree edges a deterministic LCA without inventing a semantic node.
	parent   []int
	depth    []int
	enter    []int
	leave    []int
	children [][]int
	order    []int // increasing canonical DFS number
	back     [][]Node
	restore  [][]arc
	workPred [][]Node
}

type arc struct{ from, to Node }

// buildForest visits nodes and already-canonical successor rows in semantic
// order. Its offline Tarjan LCA pass assigns every cross/forward edge to the
// one point where ConstructWPOBU restores it; a virtual-root result instead
// remains a root-frontier influence.
func buildForest(graph [][]Node, order nodeOrder) *dfsForest {
	nodes := len(graph)
	virtual := nodes
	forest := &dfsForest{
		parent:   make([]int, nodes+1),
		depth:    make([]int, nodes+1),
		enter:    make([]int, nodes+1),
		leave:    make([]int, nodes+1),
		children: make([][]int, nodes+1),
		back:     make([][]Node, nodes),
		restore:  make([][]arc, nodes+1),
		workPred: make([][]Node, nodes),
	}
	for node := range forest.parent {
		forest.parent[node] = -1
		forest.enter[node] = -1
		forest.leave[node] = -1
	}
	forest.parent[virtual] = -1
	forest.enter[virtual] = 0

	type frame struct{ node, next int }
	clock := 0
	for _, rootNode := range order.nodes {
		root := int(rootNode)
		if forest.enter[root] != -1 {
			continue
		}
		forest.parent[root] = virtual
		forest.depth[root] = 1
		forest.children[virtual] = append(forest.children[virtual], root)
		forest.enter[root] = clock
		forest.order = append(forest.order, root)
		clock++
		stack := []frame{{node: root}}
		for len(stack) != 0 {
			current := &stack[len(stack)-1]
			if current.next != len(graph[current.node]) {
				child := int(graph[current.node][current.next])
				current.next++
				if forest.enter[child] != -1 {
					continue
				}
				forest.parent[child] = current.node
				forest.depth[child] = forest.depth[current.node] + 1
				forest.children[current.node] = append(forest.children[current.node], child)
				forest.enter[child] = clock
				forest.order = append(forest.order, child)
				clock++
				stack = append(stack, frame{node: child})
				continue
			}
			forest.leave[current.node] = clock
			stack = stack[:len(stack)-1]
		}
	}
	forest.leave[virtual] = clock

	queries := make([][]lcaQuery, nodes+1)
	var crossForward []arc
	for _, fromNode := range order.nodes {
		from := int(fromNode)
		row := graph[from]
		for _, to := range row {
			item := arc{from: Node(from), to: to}
			if ancestor(forest, int(to), from) {
				forest.back[to] = append(forest.back[to], Node(from))
				continue
			}
			if forest.parent[int(to)] == from {
				forest.workPred[to] = append(forest.workPred[to], Node(from))
				continue
			}
			index := len(crossForward)
			crossForward = append(crossForward, item)
			queries[from] = append(queries[from], lcaQuery{other: int(to), index: index})
			queries[to] = append(queries[to], lcaQuery{other: from, index: index})
		}
	}
	for index, at := range offlineLCA(forest, queries) {
		forest.restore[at] = append(forest.restore[at], crossForward[index])
	}
	return forest
}

type lcaQuery struct{ other, index int }

// offlineLCA is Tarjan's union-find LCA algorithm in explicit-stack form.
// The virtual root identifies a cross edge between DFS trees without making it
// a semantic header; it is never emitted as a Node or Region.
func offlineLCA(forest *dfsForest, queries [][]lcaQuery) []int {
	nodes := len(forest.parent)
	virtual := nodes - 1
	answerCount := 0
	for _, row := range queries {
		for _, query := range row {
			if query.index >= answerCount {
				answerCount = query.index + 1
			}
		}
	}
	answers := make([]int, answerCount)
	set := make([]int, nodes)
	ancestorOfSet := make([]int, nodes)
	black := make([]bool, nodes)
	for node := range set {
		set[node] = node
	}
	find := func(node int) int {
		root := node
		for set[root] != root {
			root = set[root]
		}
		for node != root {
			parent := set[node]
			set[node] = root
			node = parent
		}
		return root
	}
	type frame struct{ node, next int }
	stack := []frame{{node: virtual}}
	ancestorOfSet[virtual] = virtual
	for len(stack) != 0 {
		current := &stack[len(stack)-1]
		if current.next < len(forest.children[current.node]) {
			child := forest.children[current.node][current.next]
			current.next++
			ancestorOfSet[find(child)] = child
			stack = append(stack, frame{node: child})
			continue
		}
		node := current.node
		black[node] = true
		for _, query := range queries[node] {
			if black[query.other] {
				answers[query.index] = ancestorOfSet[find(query.other)]
			}
		}
		stack = stack[:len(stack)-1]
		if len(stack) == 0 {
			continue
		}
		parent := stack[len(stack)-1].node
		set[find(node)] = find(parent)
		ancestorOfSet[find(parent)] = parent
	}
	return answers
}

func ancestor(forest *dfsForest, possibleAncestor, node int) bool {
	return forest.enter[possibleAncestor] <= forest.enter[node] && forest.leave[node] <= forest.leave[possibleAncestor]
}

// loopNestingForest is the bottom-up Havlak/Tarjan-Ramaligam component pass
// used by ConstructWPOBU.  Each representative becomes a child exactly once;
// `parents` is therefore the complete nested WTO component relation rather
// than an independently reconstructed SCC hierarchy.
func loopNestingForest(graph [][]Node, forest *dfsForest) ([]int, []bool) {
	nodes := len(graph)
	parents := make([]int, nodes)
	for node := range parents {
		parents[node] = NoRegion
	}
	components := make([]bool, nodes)
	representative := make([]int, nodes)
	for node := range representative {
		representative[node] = node
	}
	find := func(node int) int {
		root := node
		for representative[root] != root {
			root = representative[root]
		}
		for node != root {
			parent := representative[node]
			representative[node] = root
			node = parent
		}
		return root
	}

	// A virtual-root cross edge connects two maximal DFS trees.  It contributes
	// to the final root frontier, but cannot participate in a semantic SCC, so
	// it is deliberately not restored into the header sweep.
	restore := func(at int) {
		for _, item := range forest.restore[at] {
			target := find(int(item.to))
			forest.workPred[target] = append(forest.workPred[target], item.from)
		}
	}
	queued := make([]uint32, nodes)
	seen := make([]uint32, nodes)
	var stamp uint32
	for order := len(forest.order) - 1; order >= 0; order-- {
		header := forest.order[order]
		restore(header)
		stamp++
		if stamp == 0 {
			clear(queued)
			clear(seen)
			stamp = 1
		}
		predecessors := make([]int, 0, len(forest.back[header]))
		work := make([]int, 0, len(forest.back[header]))
		for _, predecessor := range forest.back[header] {
			item := find(int(predecessor))
			if queued[item] == stamp {
				continue
			}
			queued[item] = stamp
			predecessors = append(predecessors, item)
			if item != header {
				work = append(work, item)
			}
		}
		if len(predecessors) == 0 {
			continue
		}

		nested := make([]int, 0, len(work))
		for len(work) != 0 {
			last := len(work) - 1
			item := work[last]
			work = work[:last]
			if seen[item] == stamp {
				continue
			}
			seen[item] = stamp
			nested = append(nested, item)
			for _, predecessor := range forest.workPred[item] {
				parent := find(int(predecessor))
				if parent == header || seen[parent] == stamp || queued[parent] == stamp {
					continue
				}
				queued[parent] = stamp
				work = append(work, parent)
			}
		}

		components[header] = true
		for _, child := range nested {
			parents[child] = header
			representative[find(child)] = header
		}
	}
	return parents, components
}

// orderFrontiers projects every non-feedback influence onto the immediate
// children of the smallest shared component.  The resulting per-component
// graphs are acyclic by the WTO feedback law, so a deterministic Kahn pass
// gives one compact HTO without creating WPO exits or a second scheduler.
type frontierProjection struct {
	from, to Node
	at       int
}

type frontierChildQuery struct {
	projection int
	from       bool
}

func orderFrontiers(graph [][]Node, parents []int, components []bool, order nodeOrder) ([][]Node, bool) {
	nodes := len(graph)
	hierarchy := containmentForest(parents, order)
	children := make([][]Node, nodes+1) // final slot is the private root
	for _, nodeValue := range order.nodes {
		node := int(nodeValue)
		parent := parents[node]
		if parent == NoRegion {
			children[nodes] = append(children[nodes], Node(node))
		} else {
			children[parent] = append(children[parent], Node(node))
		}
	}

	// First obtain the shared containment component for every non-feedback
	// edge in one offline LCA pass.  The second pass below answers the matching
	// immediate-child queries from the DFS path in O(1) each; no edge walks a
	// nesting chain.
	projections := make([]frontierProjection, 0)
	queries := make([][]lcaQuery, nodes+1)
	for _, fromNode := range order.nodes {
		from := int(fromNode)
		row := graph[from]
		for _, toNode := range row {
			to := int(toNode)
			if componentAncestor(hierarchy, components, to, from) || (components[from] && componentAncestor(hierarchy, components, from, to)) {
				continue
			}
			index := len(projections)
			projections = append(projections, frontierProjection{from: Node(from), to: toNode})
			queries[from] = append(queries[from], lcaQuery{other: to, index: index})
			queries[to] = append(queries[to], lcaQuery{other: from, index: index})
		}
	}
	for index, at := range offlineLCA(hierarchy, queries) {
		projections[index].at = at
	}

	childQueries := make([][]frontierChildQuery, nodes)
	for index, item := range projections {
		childQueries[item.from] = append(childQueries[item.from], frontierChildQuery{projection: index, from: true})
		childQueries[item.to] = append(childQueries[item.to], frontierChildQuery{projection: index})
	}
	left := make([]Node, len(projections))
	right := make([]Node, len(projections))
	if !resolveImmediateChildren(hierarchy, childQueries, projections, left, right) {
		return nil, false
	}

	type projected struct{ from, to Node }
	edges := make([][]projected, nodes+1)
	for index, item := range projections {
		at := item.at
		if left[index] != right[index] {
			edges[at] = append(edges[at], projected{from: left[index], to: right[index]})
		}
	}

	ordered := make([][]Node, nodes+1)
	position := make([]int, nodes)
	for node := range position {
		position[node] = -1
	}
	containers := make([]int, 0, len(order.nodes)+1)
	for _, node := range order.nodes {
		containers = append(containers, int(node))
	}
	containers = append(containers, nodes)
	for _, container := range containers {
		items := children[container]
		for index, item := range items {
			position[item] = index
		}
		out := make([][]int, len(items))
		indegree := make([]int, len(items))
		for _, edge := range edges[container] {
			from, to := position[edge.from], position[edge.to]
			if from < 0 || to < 0 || from == to {
				return nil, false
			}
			out[from] = append(out[from], to)
			indegree[to]++
		}
		ready := frontierReady{nodes: items, order: order}
		for index, degree := range indegree {
			if degree == 0 {
				ready.push(index)
			}
		}
		for len(ready.items) != 0 {
			index := ready.pop()
			ordered[container] = append(ordered[container], items[index])
			for _, successor := range out[index] {
				indegree[successor]--
				if indegree[successor] == 0 {
					ready.push(successor)
				}
			}
		}
		if len(ordered[container]) != len(items) {
			return nil, false
		}
		for _, item := range items {
			position[item] = -1
		}
	}
	return ordered, true
}

// frontierReady is the exact canonical ready set for one acyclic projected
// frontier. Its ordering is by semantic rank, not insertion time: a newly
// unlocked lower-rank Node must precede every already-ready higher-rank Node.
type frontierReady struct {
	items []int
	nodes []Node
	order nodeOrder
}

func (h *frontierReady) push(item int) {
	h.items = append(h.items, item)
	index := len(h.items) - 1
	for index != 0 {
		parent := (index - 1) / 2
		if !h.order.less(h.nodes[item], h.nodes[h.items[parent]]) {
			break
		}
		h.items[index] = h.items[parent]
		index = parent
	}
	h.items[index] = item
}

func (h *frontierReady) pop() int {
	root := h.items[0]
	last := h.items[len(h.items)-1]
	h.items = h.items[:len(h.items)-1]
	if len(h.items) == 0 {
		return root
	}
	index := 0
	for {
		left := index*2 + 1
		if left >= len(h.items) {
			break
		}
		right := left + 1
		child := left
		if right < len(h.items) && h.order.less(h.nodes[h.items[right]], h.nodes[h.items[left]]) {
			child = right
		}
		if !h.order.less(h.nodes[h.items[child]], h.nodes[last]) {
			break
		}
		h.items[index] = h.items[child]
		index = child
	}
	h.items[index] = last
	return root
}

// containmentForest turns the union-find result into an ordinary rooted tree
// for a second offline-LCA pass.  It is private construction data; its final
// slot is a virtual root and never reaches Schedule.
func containmentForest(parents []int, order nodeOrder) *dfsForest {
	nodes := len(parents)
	virtual := nodes
	forest := &dfsForest{
		parent:   make([]int, nodes+1),
		depth:    make([]int, nodes+1),
		enter:    make([]int, nodes+1),
		leave:    make([]int, nodes+1),
		children: make([][]int, nodes+1),
	}
	for node := range forest.parent {
		forest.parent[node] = -1
		forest.enter[node] = -1
		forest.leave[node] = -1
	}
	for _, nodeValue := range order.nodes {
		node := int(nodeValue)
		parent := parents[node]
		if parent == NoRegion {
			parent = virtual
		}
		forest.parent[node] = parent
		forest.children[parent] = append(forest.children[parent], node)
	}
	forest.enter[virtual] = 0
	clock := 1
	type frame struct{ node, next int }
	stack := []frame{{node: virtual}}
	for len(stack) != 0 {
		current := &stack[len(stack)-1]
		if current.next < len(forest.children[current.node]) {
			child := forest.children[current.node][current.next]
			current.next++
			forest.depth[child] = forest.depth[current.node] + 1
			forest.enter[child] = clock
			clock++
			stack = append(stack, frame{node: child})
			continue
		}
		forest.leave[current.node] = clock
		stack = stack[:len(stack)-1]
	}
	return forest
}

func componentAncestor(forest *dfsForest, components []bool, possibleAncestor, node int) bool {
	if possibleAncestor == node {
		return components[possibleAncestor]
	}
	return forest.enter[possibleAncestor] <= forest.enter[node] && forest.leave[node] <= forest.leave[possibleAncestor]
}

func resolveImmediateChildren(forest *dfsForest, queries [][]frontierChildQuery, projections []frontierProjection, left, right []Node) bool {
	virtual := len(forest.parent) - 1
	type frame struct {
		node    int
		next    int
		entered bool
	}
	path := make([]int, 0, len(forest.parent))
	frames := []frame{{node: virtual}}
	for len(frames) != 0 {
		current := &frames[len(frames)-1]
		if !current.entered {
			current.entered = true
			path = append(path, current.node)
			if current.node != virtual {
				for _, query := range queries[current.node] {
					at := projections[query.projection].at
					childDepth := forest.depth[at] + 1
					if childDepth >= len(path) {
						return false
					}
					child := Node(path[childDepth])
					if query.from {
						left[query.projection] = child
					} else {
						right[query.projection] = child
					}
				}
			}
		}
		if current.next < len(forest.children[current.node]) {
			child := forest.children[current.node][current.next]
			current.next++
			frames = append(frames, frame{node: child})
			continue
		}
		frames = frames[:len(frames)-1]
		path = path[:len(path)-1]
	}
	return true
}

// emitFrontiers publishes only the immutable event stream and its exact
// Region brackets.  The private root is never observable.
func (s *Schedule) emitFrontiers(children [][]Node, components []bool) {
	root := len(components)
	type frame struct {
		container int
		next      int
		parent    int
		close     int
	}
	frames := []frame{{container: root, parent: NoRegion, close: NoRegion}}
	for len(frames) != 0 {
		current := &frames[len(frames)-1]
		items := children[current.container]
		if current.next == len(items) {
			if current.close != NoRegion {
				region := &s.regions[current.close]
				region.Exit = len(s.events)
				s.events = append(s.events, Event{Kind: EventExit, Node: region.Head, Region: current.close})
			}
			frames = frames[:len(frames)-1]
			continue
		}
		node := items[current.next]
		current.next++
		if !components[node] {
			s.events = append(s.events, Event{Kind: EventNode, Node: node, Region: current.parent})
			continue
		}
		region := len(s.regions)
		s.regions = append(s.regions, Region{Head: node, Parent: current.parent, Enter: len(s.events), Exit: -1})
		s.events = append(s.events, Event{Kind: EventEnter, Node: node, Region: region})
		s.events = append(s.events, Event{Kind: EventNode, Node: node, Region: region})
		frames = append(frames, frame{container: int(node), parent: region, close: region})
	}
}

func sortedUniqueDense(nodes []Node, order nodeOrder) []Node {
	if len(nodes) < 2 {
		return nodes
	}
	sort.Slice(nodes, func(left, right int) bool { return order.less(nodes[left], nodes[right]) })
	end := 1
	for _, node := range nodes[1:] {
		if node != nodes[end-1] {
			nodes[end] = node
			end++
		}
	}
	return nodes[:end]
}
