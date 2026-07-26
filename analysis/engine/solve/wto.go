package solve

import (
	"context"
	"errors"
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/cancellation"
)

// ErrWTOPlanUncovered reports that a transfer observed a backward influence
// edge absent from its immutable weak-topological plan. Callers must discard
// the scratch result and use the ordinary FIFO solver.
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
	var clone func([]WTOElement[Cell]) []WTOElement[Cell]
	clone = func(in []WTOElement[Cell]) []WTOElement[Cell] {
		out := make([]WTOElement[Cell], len(in))
		for i, element := range in {
			out[i].Vertex = element.Vertex
			if element.Body != nil {
				out[i].Body = clone(element.Body)
			}
		}
		return out
	}
	return clone(p.elements)
}

// Influences returns the complete frozen scheduling-edge inventory in
// canonical plan order.  This is an immutable certificate surface for codecs;
// callers must not derive a replacement WTO from it.
func (p *WTOPlan[Cell]) Influences() []WTOInfluence[Cell] {
	if p == nil {
		return nil
	}
	out := make([]WTOInfluence[Cell], 0, len(p.edges))
	for item := range p.edges {
		out = append(out, WTOInfluence[Cell]{From: item.from, To: item.to})
	}
	sort.Slice(out, func(i, j int) bool {
		fromI, fromJ := p.rank[out[i].From], p.rank[out[j].From]
		if fromI != fromJ {
			return fromI < fromJ
		}
		return p.rank[out[i].To] < p.rank[out[j].To]
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
	index, ok := p.index[cell]
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
	cells          []Cell
	elements       []WTOElement[Cell]
	index          map[Cell]int
	rank           map[Cell]int
	edges          map[edge[Cell]]struct{}
	componentCount int
	// component is the outermost stabilization component each cell belongs to,
	// as the half-open rank span of that component. Cells outside every
	// component are absent. Two cells that share a span are re-evaluated by the
	// same stabilization loop, which is what makes a read between them a
	// chaotic-iteration read rather than an unplanned recurrence.
	component map[Cell]componentSpan
}

// componentSpan is the half-open rank interval a stabilization component
// occupies in the structured order.
type componentSpan struct{ start, end int }

// componentSpans records, for every cell, the outermost component containing
// it. Nesting is proper, so equality of the outermost span is exactly the
// "same stabilization loop" relation.
func componentSpans[Cell comparable](elements []WTOElement[Cell]) map[Cell]componentSpan {
	var out map[Cell]componentSpan
	visited := 0
	var walk func([]WTOElement[Cell], componentSpan, bool)
	walk = func(items []WTOElement[Cell], enclosing componentSpan, inside bool) {
		for _, item := range items {
			start := visited
			visited++
			span, within := enclosing, inside
			if item.IsComponent() && !within {
				span, within = componentSpan{start: start, end: start + 1 + countWTOCells(item.Body)}, true
			}
			if within {
				if out == nil {
					out = make(map[Cell]componentSpan)
				}
				out[item.Vertex] = span
			}
			walk(item.Body, span, within)
		}
	}
	walk(elements, componentSpan{}, false)
	return out
}

func countWTOCells[Cell comparable](items []WTOElement[Cell]) int {
	total := 0
	for _, item := range items {
		total += 1 + countWTOCells(item.Body)
	}
	return total
}

// coIterated reports whether two cells belong to the same outermost
// stabilization component. Such cells are revisited together until that
// component's head stops changing, so one may read the other's current value
// at any point of the ascent without altering the frozen schedule.
func (p *WTOPlan[Cell]) coIterated(a, b Cell) bool {
	if p == nil || p.component == nil {
		return false
	}
	left, leftOK := p.component[a]
	right, rightOK := p.component[b]
	return leftOK && rightOK && left == right
}

// InComponent reports whether a cell is revisited by a stabilization loop.
func (p *WTOPlan[Cell]) InComponent(cell Cell) bool {
	if p == nil || p.component == nil {
		return false
	}
	_, ok := p.component[cell]
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
	wanted := make(map[Cell]struct{}, len(demanded))
	for _, cell := range demanded {
		if _, present := plan.index[cell]; !present {
			return nil, ErrWTOPlanRestrictionUncovered
		}
		wanted[cell] = struct{}{}
	}
	var intersects func(WTOElement[Cell]) bool
	intersects = func(element WTOElement[Cell]) bool {
		if _, wanted := wanted[element.Vertex]; wanted {
			return true
		}
		for _, child := range element.Body {
			if intersects(child) {
				return true
			}
		}
		return false
	}
	elements := make([]WTOElement[Cell], 0, len(plan.elements))
	for _, element := range plan.elements {
		if !intersects(element) {
			continue
		}
		// Components are indivisible for restriction. cloneWTOElements is
		// intentionally used instead of NewWTOPlan: this preserves the exact
		// schedule selected at production freeze time.
		if element.IsComponent() {
			elements = append(elements, cloneWTOElements([]WTOElement[Cell]{element})...)
			continue
		}
		elements = append(elements, WTOElement[Cell]{Vertex: element.Vertex})
	}
	cells := flattenWTOElements(elements)
	edges := make([]WTOInfluence[Cell], 0, len(plan.edges))
	retained := make(map[Cell]struct{}, len(cells))
	for _, cell := range cells {
		retained[cell] = struct{}{}
	}
	for edge := range plan.edges {
		if _, from := retained[edge.from]; !from {
			continue
		}
		if _, to := retained[edge.to]; !to {
			continue
		}
		edges = append(edges, WTOInfluence[Cell]{From: edge.from, To: edge.to})
	}
	return FreezeWTOPlan(cells, elements, edges)
}

func flattenWTOElements[Cell comparable](elements []WTOElement[Cell]) []Cell {
	var cells []Cell
	var visit func([]WTOElement[Cell])
	visit = func(items []WTOElement[Cell]) {
		for _, item := range items {
			cells = append(cells, item.Vertex)
			visit(item.Body)
		}
	}
	visit(elements)
	return cells
}

// FreezeWTOPlan validates and freezes a caller-computed WTO without running a
// second graph decomposition. elements must cover cells exactly once.
// Influences must be forward in the structured order or backedges to an
// enclosing component head. Inputs are copied before publication.
func FreezeWTOPlan[Cell comparable](cells []Cell, elements []WTOElement[Cell], influences []WTOInfluence[Cell]) (*WTOPlan[Cell], error) {
	canonical := append([]Cell(nil), cells...)
	index := make(map[Cell]int, len(canonical))
	for position, cell := range canonical {
		if _, duplicate := index[cell]; duplicate {
			return nil, ErrWTOInvalidFrozenPlan
		}
		index[cell] = position
	}
	frozen := cloneWTOElements(elements)
	var componentEnd map[Cell]int
	componentCount := 0
	valid := true
	visited := 0
	var walk func([]WTOElement[Cell])
	walk = func(items []WTOElement[Cell]) {
		for _, item := range items {
			if visited >= len(canonical) || canonical[visited] != item.Vertex {
				valid = false
			}
			visited++
			if item.IsComponent() {
				componentCount++
				if componentEnd == nil {
					componentEnd = make(map[Cell]int)
				}
			}
			walk(item.Body)
			if item.IsComponent() {
				componentEnd[item.Vertex] = visited
			}
		}
	}
	walk(frozen)
	if !valid || visited != len(canonical) {
		return nil, ErrWTOInvalidFrozenPlan
	}
	edges := make(map[edge[Cell]]struct{}, len(influences))
	for _, influence := range influences {
		fromRank, fromOK := index[influence.From]
		toRank, toOK := index[influence.To]
		if !fromOK || !toOK {
			return nil, ErrWTOInvalidFrozenPlan
		}
		if fromRank >= toRank {
			end, head := componentEnd[influence.To]
			if !head || fromRank >= end {
				return nil, ErrWTOInvalidFrozenPlan
			}
		}
		edges[edge[Cell]{from: influence.From, to: influence.To}] = struct{}{}
	}
	return &WTOPlan[Cell]{
		cells: canonical, elements: frozen, index: index, rank: index, edges: edges,
		componentCount: componentCount, component: componentSpans(frozen),
	}, nil
}

func cloneWTOElements[Cell comparable](in []WTOElement[Cell]) []WTOElement[Cell] {
	out := make([]WTOElement[Cell], len(in))
	for index, element := range in {
		out[index].Vertex = element.Vertex
		if element.Body != nil {
			out[index].Body = cloneWTOElements(element.Body)
		}
	}
	return out
}

// NewWTOPlan computes a deterministic nested-SCC schedule. Cells and each
// Successors result must be in canonical order. Edges to undeclared cells are
// ignored.
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
	edges := make(map[edge[Cell]]struct{})
	for _, from := range canonical {
		for _, to := range successors(from) {
			if _, declared := index[to]; declared {
				edges[edge[Cell]{from: from, to: to}] = struct{}{}
			}
		}
	}
	elements := weakTopologicalOrder(canonical, index, successors)
	rank := make(map[Cell]int, len(canonical))
	componentCount := 0
	var assign func([]WTOElement[Cell])
	assign = func(items []WTOElement[Cell]) {
		for _, item := range items {
			rank[item.Vertex] = len(rank)
			if item.IsComponent() {
				componentCount++
			}
			assign(item.Body)
		}
	}
	assign(elements)
	return &WTOPlan[Cell]{cells: canonical, elements: elements, index: index, rank: rank, edges: edges, componentCount: componentCount, component: componentSpans(elements)}
}

func weakTopologicalOrder[Cell comparable](cells []Cell, declared map[Cell]int, successors func(Cell) []Cell) []WTOElement[Cell] {
	dfn := make([]int, len(cells))
	stack := make([]Cell, 0, len(cells))
	next := 0
	const done = int(^uint(0) >> 1)
	var visit func(Cell, *[]WTOElement[Cell]) int
	visit = func(v Cell, partition *[]WTOElement[Cell]) int {
		vIndex := declared[v]
		next++
		dfn[vIndex] = next
		stack = append(stack, v)
		head := next
		loop := false
		for _, w := range successors(v) {
			wIndex, ok := declared[w]
			if !ok {
				continue
			}
			minimum := dfn[wIndex]
			if minimum == 0 {
				minimum = visit(w, partition)
			}
			if minimum <= head {
				head = minimum
				loop = true
			}
		}
		if head == dfn[vIndex] {
			dfn[vIndex] = done
			element := WTOElement[Cell]{Vertex: v}
			if loop {
				for {
					last := len(stack) - 1
					w := stack[last]
					stack = stack[:last]
					dfn[declared[w]] = 0
					if w == v {
						break
					}
				}
				dfn[vIndex] = done
				body := make([]WTOElement[Cell], 0)
				for _, w := range successors(v) {
					if wIndex, ok := declared[w]; ok && dfn[wIndex] == 0 {
						visit(w, &body)
					}
				}
				reverseWTOElements(body)
				element.Body = body
			} else {
				stack = stack[:len(stack)-1]
			}
			*partition = append(*partition, element)
		}
		return head
	}
	partition := make([]WTOElement[Cell], 0)
	for _, cell := range cells {
		if dfn[declared[cell]] == 0 {
			visit(cell, &partition)
		}
	}
	reverseWTOElements(partition)
	return partition
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
// component and therefore still requires FIFO fallback.
func (p *WTOPlan[Cell]) coversInfluence(from, to Cell) bool {
	// A transfer reading its own current input is ubiquitous and does not by
	// itself create a recurrence; emissions are validated separately below.
	if from == to {
		return true
	}
	if _, ok := p.edges[edge[Cell]{from: from, to: to}]; ok {
		return true
	}
	fromRank, fromOK := p.rank[from]
	toRank, toOK := p.rank[to]
	if fromOK && toOK && fromRank < toRank {
		return true
	}
	return p.coIterated(from, to)
}

// coversEmission reports whether the existing WTO can schedule an observed
// transfer output. Emitted-only destinations have no equation to revisit. For
// declared cells, a planned edge or a new strict-forward edge is safe; a new
// self/backward edge can alter component structure and requires a rebuilt plan.
func (p *WTOPlan[Cell]) coversEmission(from, to Cell) bool {
	if _, declared := p.index[to]; !declared {
		return true
	}
	if _, ok := p.edges[edge[Cell]{from: from, to: to}]; ok {
		return true
	}
	if from == to {
		return false
	}
	fromRank, fromOK := p.rank[from]
	toRank, toOK := p.rank[to]
	return fromOK && toOK && fromRank < toRank
}

func (p *WTOPlan[Cell]) stabilizes(cell Cell) bool {
	if p == nil {
		return false
	}
	_, ok := p.edges[edge[Cell]{from: cell, to: cell}]
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
	read := func(d Cell) State {
		if d == s.active {
			s.activeReadSelf = true
		}
		if !plan.coversInfluence(d, s.active) {
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
		// Reading one's own current value is a recurrence. It is planned when
		// the frozen graph carries the self edge, and equally when the cell sits
		// inside a stabilization component: the component's revisit loop is what
		// re-evaluates the cell until that value stops changing.
		if (s.evaluate != nil && s.activeReadSelf && !plan.stabilizes(cell) && !plan.InComponent(cell)) || uncovered {
			return ErrWTOPlanUncovered
		}
		s.recordVisit(cell)
		return nil
	}
	var runPartition func([]WTOElement[Cell]) error
	runPartition = func(items []WTOElement[Cell]) error {
		for _, item := range items {
			if !item.IsComponent() {
				if err := runCell(item.Vertex); err != nil {
					return err
				}
				continue
			}
			for {
				before := s.versionOf(item.Vertex)
				if err := runCell(item.Vertex); err != nil {
					return err
				}
				if err := runPartition(item.Body); err != nil {
					return err
				}
				if s.versionOf(item.Vertex) == before {
					break
				}
			}
		}
		return nil
	}
	if err := runPartition(plan.elements); err != nil {
		return nil, err
	}
	if err := s.runNarrowing(cancel); err != nil {
		return nil, err
	}
	if err := cancel.err(0); err != nil {
		return nil, err
	}
	return s.materialize(), nil
}
