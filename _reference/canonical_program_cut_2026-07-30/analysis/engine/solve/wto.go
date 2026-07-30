package solve

import (
	"context"
	"errors"
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/cancellation"
)

// ErrWTOPlanUncovered reports that a transfer observed a backward influence
// edge absent from its immutable weak-topological plan. The solve fails closed
// and publishes no result.
var ErrWTOPlanUncovered = errors.New("solve: WTO plan does not cover dynamic dependency")

// ErrWTOInvalidFrozenPlan reports that caller-supplied WTO elements and
// influences do not form an exact, closed schedule.
var ErrWTOInvalidFrozenPlan = errors.New("solve: invalid frozen WTO plan")

// ErrWTOPlanRestrictionUncovered reports a demand containing a cell which is
// absent from the frozen plan. Restriction is not permitted to manufacture a
// schedule for it.
var ErrWTOPlanRestrictionUncovered = errors.New("solve: WTO restriction contains an uncovered cell")

// WTOElement is one element of a deterministic Bourdoncle weak topological
// ordering. A component has Vertex as its head and a non-nil Body; an ordinary
// vertex has a nil Body.
type WTOElement[Cell comparable] struct {
	Vertex Cell
	Body   []WTOElement[Cell]
}

// WTOInfluence is one immutable dependency-to-consumer scheduling edge.
type WTOInfluence[Cell comparable] struct {
	From Cell
	To   Cell
}

func (e WTOElement[Cell]) IsComponent() bool { return e.Body != nil }

// Elements returns a deep copy of the plan's structured schedule. It is
// intended for immutable, shape-specific executors which compile the generic
// WTO into a denser instruction representation. Mutating the returned slices
// cannot alter the plan.
func (p *WTOPlan[Cell]) Elements() []WTOElement[Cell] {
	if p == nil {
		return nil
	}
	schedule := make([]wtoBracket[Cell], 0)
	for _, root := range p.roots {
		schedule = append(schedule, p.core.schedule[root.start:root.end]...)
	}
	return unbracketWTOElements(schedule)
}

// AppendCells appends the plan's exact execution order to dst.
func (p *WTOPlan[Cell]) AppendCells(dst []Cell) []Cell {
	if p == nil {
		return dst
	}
	for _, root := range p.roots {
		dst = append(dst, p.core.order[root.rank:root.rankEnd]...)
	}
	return dst
}

// Influences returns the complete frozen scheduling-edge inventory in
// canonical plan order.  This is an immutable certificate surface for codecs;
// callers must not derive a replacement WTO from it.
func (p *WTOPlan[Cell]) Influences() []WTOInfluence[Cell] {
	if p == nil {
		return nil
	}
	out := make([]WTOInfluence[Cell], 0, len(p.core.edges))
	for item := range p.core.edges {
		if !p.contains(item.from) || !p.contains(item.to) {
			continue
		}
		out = append(out, WTOInfluence[Cell]{From: item.from, To: item.to})
	}
	sort.Slice(out, func(i, j int) bool {
		fromI, fromJ := p.core.rank[out[i].From], p.core.rank[out[j].From]
		if fromI != fromJ {
			return fromI < fromJ
		}
		return p.core.rank[out[i].To] < p.core.rank[out[j].To]
	})
	return out
}

// CoversInfluence reports whether a dependency read is scheduled by the
// immutable WTO. Dense executors use this as a fail-closed dynamic-read gate.
func (p *WTOPlan[Cell]) CoversInfluence(from, to Cell) bool {
	return p != nil && p.coversInfluence(from, to)
}

// CoversEmission reports whether a contribution is scheduled by the immutable
// WTO. Dense executors use this as a fail-closed dynamic-emission gate.
func (p *WTOPlan[Cell]) CoversEmission(from, to Cell) bool {
	return p != nil && p.coversEmission(from, to)
}

// Matches reports whether cells are exactly the canonical cell set of this
// immutable plan.
func (p *WTOPlan[Cell]) Matches(cells []Cell) bool { return p != nil && p.matches(cells) }

// CanonicalIndex returns a cell's stable index in the frozen plan. Dense
// executors use it to build slice-backed metadata without allocating a second
// cell-to-index map.
func (p *WTOPlan[Cell]) CanonicalIndex(cell Cell) (int, bool) {
	if p == nil {
		return 0, false
	}
	index, ok := p.memberIndex[cell]
	return index, ok
}

// ComponentCount returns the number of structured stabilization components
// without cloning or walking the plan at execution time.
func (p *WTOPlan[Cell]) ComponentCount() int {
	if p == nil {
		return 0
	}
	return p.componentCount
}

// WTOPlan is an immutable structured schedule for one equation-system shape.
// It is generic over Cell and has no dependency on State or any State lane.
// The maps are construction-time indexes; execution order itself is held in
// canonical arrays/slices.
type WTOPlan[Cell comparable] struct {
	core           *wtoPlanCore[Cell]
	cells          []Cell
	memberIndex    map[Cell]int
	componentCount int
	roots          []rootScheduleSpan
}

// wtoPlanCore is the one immutable semantic certificate shared by every
// demand-restricted view. Views never copy or reinterpret its schedule, graph,
// ranks, component heads, or component membership.
type wtoPlanCore[Cell comparable] struct {
	order    []Cell
	schedule []wtoBracket[Cell]
	index    map[Cell]int
	rank     map[Cell]int
	edges    map[edge[Cell]]struct{}
	// componentHeads is the exact, immutable widening cut set of this plan.
	// Each structured WTO component contributes its vertex, including nested
	// components.  Looking a cell up here allocates nothing on the solve path.
	componentHeads map[Cell]struct{}
	// component is the outermost stabilization component each cell belongs to,
	// as the half-open rank span of that component. Cells outside every
	// component are absent. Two cells that share a span are re-evaluated by the
	// same stabilization loop, which is what makes a read between them a
	// chaotic-iteration read rather than an unplanned recurrence.
	component map[Cell]componentSpan
}

type rootScheduleSpan struct{ rank, rankEnd, start, end int }

// IsComponentHead reports whether cell is the head of a structured WTO
// component. These are precisely the feedback vertices selected by this
// immutable plan; cyclic artifact evaluators use them as their widening sites.
func (p *WTOPlan[Cell]) IsComponentHead(cell Cell) bool {
	if p == nil || p.core == nil || p.core.componentHeads == nil || !p.contains(cell) {
		return false
	}
	_, ok := p.core.componentHeads[cell]
	return ok
}

func (p *WTOPlan[Cell]) contains(cell Cell) bool {
	if p == nil {
		return false
	}
	_, ok := p.memberIndex[cell]
	return ok
}

// componentSpan is the half-open rank interval a stabilization component
// occupies in the structured order.
type componentSpan struct{ start, end int }

type wtoBracketKind uint8

const (
	wtoOrdinary wtoBracketKind = iota
	wtoComponentOpen
	wtoComponentClose
)

type wtoBracket[Cell comparable] struct {
	vertex Cell
	kind   wtoBracketKind
}

// bracketWTOElements turns the nested constructor spelling into a flat,
// balanced certificate. It is deliberately iterative: an admitted program may
// contain arbitrarily deep loop nesting, and construction must not consume the
// Go call stack.
func bracketWTOElements[Cell comparable](elements []WTOElement[Cell]) []wtoBracket[Cell] {
	type frame struct {
		items     []WTOElement[Cell]
		next      int
		close     bool
		closeHead Cell
	}
	out := make([]wtoBracket[Cell], 0, len(elements))
	stack := []frame{{items: elements}}
	for len(stack) != 0 {
		top := &stack[len(stack)-1]
		if top.next == len(top.items) {
			if top.close {
				out = append(out, wtoBracket[Cell]{vertex: top.closeHead, kind: wtoComponentClose})
			}
			stack = stack[:len(stack)-1]
			continue
		}
		element := top.items[top.next]
		top.next++
		if !element.IsComponent() {
			out = append(out, wtoBracket[Cell]{vertex: element.Vertex, kind: wtoOrdinary})
			continue
		}
		out = append(out, wtoBracket[Cell]{vertex: element.Vertex, kind: wtoComponentOpen})
		stack = append(stack, frame{items: element.Body, close: true, closeHead: element.Vertex})
	}
	return out
}

func unbracketWTOElements[Cell comparable](schedule []wtoBracket[Cell]) []WTOElement[Cell] {
	type frame struct {
		head Cell
		body []WTOElement[Cell]
	}
	stack := []frame{{}}
	for _, bracket := range schedule {
		switch bracket.kind {
		case wtoOrdinary:
			top := &stack[len(stack)-1]
			top.body = append(top.body, WTOElement[Cell]{Vertex: bracket.vertex})
		case wtoComponentOpen:
			stack = append(stack, frame{head: bracket.vertex, body: make([]WTOElement[Cell], 0)})
		case wtoComponentClose:
			complete := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			top := &stack[len(stack)-1]
			top.body = append(top.body, WTOElement[Cell]{Vertex: complete.head, Body: complete.body})
		}
	}
	return stack[0].body
}

// componentSpans records, for every cell, the outermost component containing
// it. Nesting is proper, so equality of the outermost span is exactly the
// "same stabilization loop" relation.
func componentSpans[Cell comparable](brackets []wtoBracket[Cell]) map[Cell]componentSpan {
	type member struct {
		cell  Cell
		start int
	}
	members := make([]member, 0)
	ends := make(map[int]int)
	stack := make([]int, 0)
	rank := 0
	for _, bracket := range brackets {
		switch bracket.kind {
		case wtoComponentOpen:
			stack = append(stack, rank)
			members = append(members, member{cell: bracket.vertex, start: stack[0]})
			rank++
		case wtoOrdinary:
			if len(stack) != 0 {
				members = append(members, member{cell: bracket.vertex, start: stack[0]})
			}
			rank++
		case wtoComponentClose:
			if len(stack) == 1 {
				ends[stack[0]] = rank
			}
			stack = stack[:len(stack)-1]
		}
	}
	var out map[Cell]componentSpan
	if len(members) != 0 {
		out = make(map[Cell]componentSpan, len(members))
		for _, item := range members {
			out[item.cell] = componentSpan{start: item.start, end: ends[item.start]}
		}
	}
	return out
}

// componentHeadSet derives every component head without recursive traversal.
// Plans can contain nested components, so all component vertices, not only
// top-level ones, are feedback cut points.
func componentHeadSet[Cell comparable](schedule []wtoBracket[Cell]) map[Cell]struct{} {
	var heads map[Cell]struct{}
	for _, bracket := range schedule {
		if bracket.kind == wtoComponentOpen {
			if heads == nil {
				heads = make(map[Cell]struct{})
			}
			heads[bracket.vertex] = struct{}{}
		}
	}
	return heads
}

// coIterated reports whether two cells belong to the same outermost
// stabilization component. Such cells are revisited together until that
// component's head stops changing, so one may read the other's current value
// at any point of the ascent without altering the frozen schedule.
func (p *WTOPlan[Cell]) coIterated(a, b Cell) bool {
	if p == nil || p.core == nil || p.core.component == nil || !p.contains(a) || !p.contains(b) {
		return false
	}
	left, leftOK := p.core.component[a]
	right, rightOK := p.core.component[b]
	return leftOK && rightOK && left == right
}

// InComponent reports whether a cell is revisited by a stabilization loop.
func (p *WTOPlan[Cell]) InComponent(cell Cell) bool {
	if p == nil || p.core == nil || p.core.component == nil || !p.contains(cell) {
		return false
	}
	_, ok := p.core.component[cell]
	return ok
}

// RestrictWTOPlan selects a demanded subset from an existing frozen plan
// without computing another WTO.  Ordinary top-level vertices are retained
// only when demanded.  A demand that touches a component retains the complete
// enclosing component (including nested components), preserving its original
// head, nesting, and visit order.  Thus the result is SCC-closed by
// construction and cannot drift from the production widening schedule.
func RestrictWTOPlan[Cell comparable](plan *WTOPlan[Cell], demanded []Cell) (*WTOPlan[Cell], error) {
	if plan == nil || len(demanded) == 0 {
		return nil, ErrWTOPlanRestrictionUncovered
	}
	wanted := make(map[int]struct{}, len(demanded))
	for _, cell := range demanded {
		if !plan.contains(cell) {
			return nil, ErrWTOPlanRestrictionUncovered
		}
		rank := plan.core.rank[cell]
		if span, inside := plan.core.component[cell]; inside {
			rank = span.start
		}
		wanted[rank] = struct{}{}
	}
	roots := make([]rootScheduleSpan, 0, len(wanted))
	componentCount := 0
	for _, span := range plan.roots {
		if _, retain := wanted[span.rank]; !retain {
			continue
		}
		roots = append(roots, span)
		for _, bracket := range plan.core.schedule[span.start:span.end] {
			if bracket.kind == wtoComponentOpen {
				componentCount++
			}
		}
	}
	if len(roots) != len(wanted) {
		return nil, ErrWTOPlanRestrictionUncovered
	}
	cellCount := 0
	for _, root := range roots {
		cellCount += root.rankEnd - root.rank
	}
	cells := make([]Cell, 0, cellCount)
	for _, root := range roots {
		cells = append(cells, plan.core.order[root.rank:root.rankEnd]...)
	}
	memberIndex := make(map[Cell]int, len(cells))
	for index, cell := range cells {
		memberIndex[cell] = index
	}
	return &WTOPlan[Cell]{
		core: plan.core, cells: cells, memberIndex: memberIndex,
		componentCount: componentCount, roots: roots,
	}, nil
}

func flattenWTOSchedule[Cell comparable](schedule []wtoBracket[Cell]) []Cell {
	cells := make([]Cell, 0, len(schedule))
	for _, bracket := range schedule {
		if bracket.kind != wtoComponentClose {
			cells = append(cells, bracket.vertex)
		}
	}
	return cells
}

// FreezeWTOPlan validates and freezes a caller-computed WTO. elements must
// cover cells exactly once.
// influences is the complete declared graph, not a subset of interesting
// edges. The supplied certificate is admitted only when the canonical
// Bourdoncle decomposition of that graph has exactly the same heads, nesting,
// and visit order. Inputs are copied before publication.
func FreezeWTOPlan[Cell comparable](cells []Cell, elements []WTOElement[Cell], influences []WTOInfluence[Cell]) (*WTOPlan[Cell], error) {
	canonical := append([]Cell(nil), cells...)
	index := make(map[Cell]int, len(canonical))
	for position, cell := range canonical {
		if _, duplicate := index[cell]; duplicate {
			return nil, ErrWTOInvalidFrozenPlan
		}
		index[cell] = position
	}
	adjacency := make([][]int, len(canonical))
	edges := make(map[edge[Cell]]struct{}, len(influences))
	for _, influence := range influences {
		from, fromOK := index[influence.From]
		to, toOK := index[influence.To]
		if !fromOK || !toOK {
			return nil, ErrWTOInvalidFrozenPlan
		}
		adjacency[from] = append(adjacency[from], to)
		edges[edge[Cell]{from: influence.From, to: influence.To}] = struct{}{}
	}
	canonicalizeWTOAdjacency(adjacency)
	supplied := bracketWTOElements(elements)
	visited := 0
	seenVertices := make(map[Cell]struct{}, len(canonical))
	for _, bracket := range supplied {
		if bracket.kind == wtoComponentClose {
			continue
		}
		if _, declared := index[bracket.vertex]; !declared {
			return nil, ErrWTOInvalidFrozenPlan
		}
		if _, duplicate := seenVertices[bracket.vertex]; duplicate {
			return nil, ErrWTOInvalidFrozenPlan
		}
		seenVertices[bracket.vertex] = struct{}{}
		visited++
	}
	if visited != len(canonical) {
		return nil, ErrWTOInvalidFrozenPlan
	}
	derived := weakTopologicalOrder(canonical, adjacency)
	if !equalWTOBrackets(supplied, bracketWTOElements(derived)) {
		return nil, ErrWTOInvalidFrozenPlan
	}
	frozen := append([]wtoBracket[Cell](nil), supplied...)
	return finalizeWTOPlan(canonical, frozen, edges), nil
}

// finalizeWTOPlan publishes metadata derived from an already-admitted exact
// schedule. Public freeze reaches it only after canonical decomposition
// equality. Restriction reaches it only by selecting whole top-level
// components from such a plan, a transformation which cannot change any
// retained component's head, nesting, or edge closure.
func finalizeWTOPlan[Cell comparable](cells []Cell, schedule []wtoBracket[Cell], edges map[edge[Cell]]struct{}) *WTOPlan[Cell] {
	index := make(map[Cell]int, len(cells))
	for position, cell := range cells {
		index[cell] = position
	}
	rank := make(map[Cell]int, len(cells))
	componentCount := 0
	roots := make([]rootScheduleSpan, 0)
	depth := 0
	var rootStart, rootRank int
	for tokenIndex, bracket := range schedule {
		if bracket.kind == wtoComponentClose {
			depth--
			if depth == 0 {
				roots = append(roots, rootScheduleSpan{rank: rootRank, rankEnd: len(rank), start: rootStart, end: tokenIndex + 1})
			}
			continue
		}
		currentRank := len(rank)
		rank[bracket.vertex] = currentRank
		if depth == 0 {
			rootStart, rootRank = tokenIndex, currentRank
			if bracket.kind == wtoOrdinary {
				roots = append(roots, rootScheduleSpan{rank: rootRank, rankEnd: rootRank + 1, start: tokenIndex, end: tokenIndex + 1})
			}
		}
		if bracket.kind == wtoComponentOpen {
			componentCount++
			depth++
		}
	}
	core := &wtoPlanCore[Cell]{
		order: flattenWTOSchedule(schedule), schedule: schedule, index: index, rank: rank, edges: edges,
		componentHeads: componentHeadSet(schedule), component: componentSpans(schedule),
	}
	return &WTOPlan[Cell]{
		core: core, cells: append([]Cell(nil), cells...), memberIndex: index,
		componentCount: componentCount, roots: roots,
	}
}

func equalWTOBrackets[Cell comparable](left, right []wtoBracket[Cell]) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func canonicalizeWTOAdjacency(adjacency [][]int) {
	// Sorting makes cell order the sole graph tie-breaker and lets duplicate
	// edges compact in place. The total cost is O(E log E) in the worst case
	// (and O(E) for already ordered bounded-degree rows); planning remains a
	// construction-time operation and execution never touches adjacency.
	for from := range adjacency {
		row := adjacency[from]
		sort.Ints(row)
		write := 0
		for _, to := range row {
			if write != 0 && row[write-1] == to {
				continue
			}
			row[write] = to
			write++
		}
		adjacency[from] = row[:write]
	}
}

// NewWTOPlan computes a deterministic nested-SCC schedule. Cells establish the
// canonical vertex order. Successors is read exactly once per declared cell;
// undeclared and duplicate successors are ignored.
func NewWTOPlan[Cell comparable](cells []Cell, successors func(Cell) []Cell) *WTOPlan[Cell] {
	canonical := make([]Cell, 0, len(cells))
	index := make(map[Cell]int, len(cells))
	for _, cell := range cells {
		if _, seen := index[cell]; seen {
			continue
		}
		index[cell] = len(canonical)
		canonical = append(canonical, cell)
	}
	if successors == nil {
		successors = func(Cell) []Cell { return nil }
	}
	adjacency := make([][]int, len(canonical))
	for from, cell := range canonical {
		for _, successor := range successors(cell) {
			if to, declared := index[successor]; declared {
				adjacency[from] = append(adjacency[from], to)
			}
		}
	}
	canonicalizeWTOAdjacency(adjacency)
	edges := make(map[edge[Cell]]struct{})
	for from, row := range adjacency {
		for _, to := range row {
			edges[edge[Cell]{from: canonical[from], to: canonical[to]}] = struct{}{}
		}
	}
	elements := weakTopologicalOrder(canonical, adjacency)
	return finalizeWTOPlan(canonical, bracketWTOElements(elements), edges)
}

func weakTopologicalOrder[Cell comparable](cells []Cell, adjacency [][]int) []WTOElement[Cell] {
	dfn := make([]int, len(cells))
	vertexStack := make([]int, 0, len(cells))
	next := 0
	const done = int(^uint(0) >> 1)

	partitions := make([][]WTOElement[Cell], 1)
	type visitFrame struct {
		vertex        int
		partition     int
		bodyPartition int
		nextSuccessor int
		head          int
		loop          bool
		bodyPhase     bool
		awaitMinimum  bool
	}
	frames := make([]visitFrame, 0, len(cells))
	push := func(vertex, partition int) {
		next++
		dfn[vertex] = next
		vertexStack = append(vertexStack, vertex)
		frames = append(frames, visitFrame{vertex: vertex, partition: partition, bodyPartition: -1, head: next})
	}
	absorbMinimum := func(frame *visitFrame, minimum int) {
		if minimum <= frame.head {
			frame.head = minimum
			frame.loop = true
		}
	}
	finish := func(minimum int) {
		frames = frames[:len(frames)-1]
		if len(frames) != 0 {
			parent := &frames[len(frames)-1]
			if parent.awaitMinimum {
				parent.awaitMinimum = false
				absorbMinimum(parent, minimum)
			}
		}
	}

	for rootIndex := range cells {
		if dfn[rootIndex] != 0 {
			continue
		}
		push(rootIndex, 0)
		for len(frames) != 0 {
			frame := &frames[len(frames)-1]
			if !frame.bodyPhase && frame.nextSuccessor < len(adjacency[frame.vertex]) {
				successor := adjacency[frame.vertex][frame.nextSuccessor]
				frame.nextSuccessor++
				minimum := dfn[successor]
				if minimum == 0 {
					frame.awaitMinimum = true
					push(successor, frame.partition)
					continue
				}
				absorbMinimum(frame, minimum)
				continue
			}

			if !frame.bodyPhase {
				if frame.head != dfn[frame.vertex] {
					finish(frame.head)
					continue
				}
				dfn[frame.vertex] = done
				if !frame.loop {
					vertexStack = vertexStack[:len(vertexStack)-1]
					partitions[frame.partition] = append(partitions[frame.partition], WTOElement[Cell]{Vertex: cells[frame.vertex]})
					finish(frame.head)
					continue
				}
				for {
					last := len(vertexStack) - 1
					vertex := vertexStack[last]
					vertexStack = vertexStack[:last]
					dfn[vertex] = 0
					if vertex == frame.vertex {
						break
					}
				}
				dfn[frame.vertex] = done
				frame.bodyPartition = len(partitions)
				partitions = append(partitions, make([]WTOElement[Cell], 0))
				frame.bodyPhase = true
				frame.nextSuccessor = 0
				continue
			}

			if frame.nextSuccessor < len(adjacency[frame.vertex]) {
				successor := adjacency[frame.vertex][frame.nextSuccessor]
				frame.nextSuccessor++
				if dfn[successor] == 0 {
					push(successor, frame.bodyPartition)
				}
				continue
			}
			body := partitions[frame.bodyPartition]
			reverseWTOElements(body)
			partitions[frame.partition] = append(partitions[frame.partition], WTOElement[Cell]{Vertex: cells[frame.vertex], Body: body})
			finish(frame.head)
		}
	}
	reverseWTOElements(partitions[0])
	return canonicalizeWTORoots(partitions[0], cells, adjacency)
}

// canonicalizeWTORoots keeps Bourdoncle's exact decomposition inside each
// strongly connected region, then gives unrelated top-level regions the
// canonical Cells tie-breaker. This makes producer-computed and caller-frozen
// certificates agree without changing any component head or nesting.
func canonicalizeWTORoots[Cell comparable](roots []WTOElement[Cell], cells []Cell, adjacency [][]int) []WTOElement[Cell] {
	if len(roots) < 2 {
		return roots
	}
	cellIndex := make(map[Cell]int, len(cells))
	for index, cell := range cells {
		cellIndex[cell] = index
	}
	rootOf := make([]int, len(adjacency))
	for index := range rootOf {
		rootOf[index] = -1
	}
	priority := make([]int, len(roots))
	for index := range priority {
		priority[index] = len(adjacency)
	}
	rootIndex, depth := -1, 0
	for _, bracket := range bracketWTOElements(roots) {
		if bracket.kind == wtoComponentClose {
			depth--
			continue
		}
		if depth == 0 {
			rootIndex++
		}
		index := cellIndex[bracket.vertex]
		rootOf[index] = rootIndex
		if index < priority[rootIndex] {
			priority[rootIndex] = index
		}
		if bracket.kind == wtoComponentOpen {
			depth++
		}
	}
	successors := make([][]int, len(roots))
	indegree := make([]int, len(roots))
	for from, row := range adjacency {
		fromRoot := rootOf[from]
		for _, to := range row {
			toRoot := rootOf[to]
			if fromRoot == toRoot {
				continue
			}
			successors[fromRoot] = append(successors[fromRoot], toRoot)
		}
	}
	for root, row := range successors {
		sort.Ints(row)
		write := 0
		for _, successor := range row {
			if write != 0 && row[write-1] == successor {
				continue
			}
			row[write] = successor
			write++
			indegree[successor]++
		}
		successors[root] = row[:write]
	}
	ready := make([]int, 0, len(roots))
	push := func(root int) {
		ready = append(ready, root)
		for child := len(ready) - 1; child > 0; {
			parent := (child - 1) / 2
			if priority[ready[parent]] <= priority[ready[child]] {
				break
			}
			ready[parent], ready[child] = ready[child], ready[parent]
			child = parent
		}
	}
	pop := func() int {
		root := ready[0]
		last := ready[len(ready)-1]
		ready = ready[:len(ready)-1]
		if len(ready) == 0 {
			return root
		}
		ready[0] = last
		for parent := 0; ; {
			left := parent*2 + 1
			if left >= len(ready) {
				break
			}
			child := left
			right := left + 1
			if right < len(ready) && priority[ready[right]] < priority[ready[left]] {
				child = right
			}
			if priority[ready[parent]] <= priority[ready[child]] {
				break
			}
			ready[parent], ready[child] = ready[child], ready[parent]
			parent = child
		}
		return root
	}
	for root := range roots {
		if indegree[root] == 0 {
			push(root)
		}
	}
	ordered := make([]WTOElement[Cell], 0, len(roots))
	for len(ready) != 0 {
		root := pop()
		ordered = append(ordered, roots[root])
		for _, successor := range successors[root] {
			indegree[successor]--
			if indegree[successor] == 0 {
				push(successor)
			}
		}
	}
	return ordered
}

func reverseWTOElements[Cell comparable](items []WTOElement[Cell]) {
	for left, right := 0, len(items)-1; left < right; left, right = left+1, right-1 {
		items[left], items[right] = items[right], items[left]
	}
}

func (p *WTOPlan[Cell]) matches(cells []Cell) bool {
	if p == nil {
		return false
	}
	if len(cells) != len(p.cells) {
		return false
	}
	for i, cell := range cells {
		if p.cells[i] != cell {
			return false
		}
	}
	return true
}

// coversInfluence accepts declared graph edges, dynamic forward reads, and
// reads between two cells of one stabilization component. A forward edge
// preserves a WTO: its source is stabilized before its reader in every
// containing-component iteration. A read inside a component is the ordinary
// chaotic-iteration read the component's own revisit loop already accounts
// for, and it adds no vertex the frozen decomposition did not schedule. A
// backward edge that leaves every enclosing component can create an unplanned
// component and therefore fails the frozen solve.
func (p *WTOPlan[Cell]) coversInfluence(from, to Cell) bool {
	if !p.contains(from) || !p.contains(to) {
		return false
	}
	// A transfer reading its own current input is ubiquitous and does not by
	// itself create a recurrence; emissions are validated separately below.
	if from == to {
		return true
	}
	if _, ok := p.core.edges[edge[Cell]{from: from, to: to}]; ok {
		return true
	}
	fromRank, fromOK := p.core.rank[from]
	toRank, toOK := p.core.rank[to]
	if fromOK && toOK && fromRank < toRank {
		return true
	}
	return p.coIterated(from, to)
}

func (p *WTOPlan[Cell]) allowsRead(from, to Cell, evaluate bool) bool {
	if !p.coversInfluence(from, to) {
		return false
	}
	// Evaluate publishes only to itself, so an undeclared self read is a
	// recurrence which needs either an explicit self edge or an enclosing
	// component. Transfer self reads merely inspect the contribution being
	// propagated; any recursive self emission is audited separately.
	return !evaluate || from != to || p.stabilizes(to) || p.InComponent(to)
}

// coversEmission reports whether the existing WTO can schedule an observed
// transfer output. Emitted-only destinations have no equation to revisit. For
// declared cells, a planned edge or a new strict-forward edge is safe; a new
// self/backward edge can alter component structure and requires a rebuilt plan.
func (p *WTOPlan[Cell]) coversEmission(from, to Cell) bool {
	if !p.contains(from) {
		return false
	}
	if _, declared := p.core.index[to]; !declared {
		return true
	}
	if !p.contains(to) {
		return false
	}
	if _, ok := p.core.edges[edge[Cell]{from: from, to: to}]; ok {
		return true
	}
	if from == to {
		return false
	}
	fromRank, fromOK := p.core.rank[from]
	toRank, toOK := p.core.rank[to]
	return fromOK && toOK && fromRank < toRank
}

func (p *WTOPlan[Cell]) stabilizes(cell Cell) bool {
	if p == nil || !p.contains(cell) {
		return false
	}
	_, ok := p.core.edges[edge[Cell]{from: cell, to: cell}]
	return ok
}

// SolveWTOContext is the cancelable structured solver for callers which do not
// publish revision metadata. The solver still owns its internal revisions for
// stabilization, but does not allocate a second result-sized versions map.
func SolveWTOContext[Cell comparable, State any](ctx context.Context, sys EquationSystem[Cell, State], plan *WTOPlan[Cell]) (map[Cell]State, error) {
	cancel := newCancellationGuard(cancellation.FromContext(ctx))
	result, err := solveWTOSystem(sys, plan, cancel)
	return result, err
}

func solveWTOSystem[Cell comparable, State any](sys EquationSystem[Cell, State], plan *WTOPlan[Cell], cancel *cancellationGuard) (map[Cell]State, error) {
	validateEquationSystem(sys)
	if plan == nil || !plan.matches(sys.Cells) {
		return nil, ErrWTOPlanUncovered
	}
	if err := cancel.err(0); err != nil {
		return nil, err
	}
	s := newStructuredState(sys, false)
	// Structured ascent owns scheduling. Avoid FIFO queue/dependency storage on
	// its hot path; versions and widening history remain the ordinary solver's.

	uncovered := false
	evaluate := sys.Evaluate != nil
	read := func(d Cell) State {
		if !plan.allowsRead(d, s.active, evaluate) {
			uncovered = true
		}
		return s.curOf(d)
	}
	emit := func(d Cell, value State) {
		if !plan.coversEmission(s.active, d) {
			uncovered = true
		}
		s.emitStructured(d, value)
	}
	readVersioned := func(d Cell) (State, uint64) { return read(d), s.versionOf(d) }
	var iteration uint64
	runCell := func(cell Cell) error {
		if err := cancel.err(iteration); err != nil {
			return err
		}
		iteration++
		s.active = cell
		s.activeReadSelf = false
		s.activeSelfChanged = false
		if s.stats != nil {
			s.stats.TransferCalls++
		}
		if s.transfer != nil {
			if s.transferVersioned != nil {
				s.transferVersioned(cell, readVersioned, emit)
			} else {
				s.transfer(cell, read, emit)
			}
		} else if s.evaluateVersioned != nil {
			s.emitStructured(cell, s.evaluateVersioned(cell, readVersioned))
		} else {
			s.emitStructured(cell, s.evaluate(cell, read))
		}
		if err := cancel.err(0); err != nil {
			return err
		}
		if uncovered {
			return ErrWTOPlanUncovered
		}
		s.recordVisit(cell)
		return nil
	}
	type componentFrame struct {
		open   int
		head   Cell
		before uint64
	}
	frames := make([]componentFrame, 0, plan.componentCount)
	for _, root := range plan.roots {
		for pc := root.start; pc < root.end; {
			step := plan.core.schedule[pc]
			switch step.kind {
			case wtoOrdinary:
				if err := runCell(step.vertex); err != nil {
					return nil, err
				}
				pc++
			case wtoComponentOpen:
				if len(frames) == 0 || frames[len(frames)-1].open != pc {
					frames = append(frames, componentFrame{open: pc, head: step.vertex})
				}
				frames[len(frames)-1].before = s.versionOf(step.vertex)
				if err := runCell(step.vertex); err != nil {
					return nil, err
				}
				pc++
			case wtoComponentClose:
				frame := frames[len(frames)-1]
				if s.versionOf(frame.head) != frame.before {
					pc = frame.open
					continue
				}
				frames = frames[:len(frames)-1]
				pc++
			}
		}
	}
	if err := s.runNarrowing(cancel, plan, evaluate); err != nil {
		return nil, err
	}
	if err := cancel.err(0); err != nil {
		return nil, err
	}
	return s.materialize(), nil
}
