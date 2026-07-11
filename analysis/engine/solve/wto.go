package solve

import (
	"context"
	"errors"

	"github.com/wippyai/go-lua/analysis/engine/cancellation"
)

// ErrWTOPlanUncovered reports that a transfer observed a backward influence
// edge absent from its immutable weak-topological plan. Callers must discard
// the scratch result and use the ordinary FIFO solver.
var ErrWTOPlanUncovered = errors.New("solve: WTO plan does not cover dynamic dependency")

// WTOElement is one element of a deterministic Bourdoncle weak topological
// ordering. A component has Vertex as its head and a non-nil Body; an ordinary
// vertex has a nil Body.
type WTOElement[Cell comparable] struct {
	Vertex Cell
	Body   []WTOElement[Cell]
}

func (e WTOElement[Cell]) IsComponent() bool { return e.Body != nil }

// WTOPlan is an immutable structured schedule for one equation-system shape.
// It is generic over Cell and has no dependency on State or any State lane.
// The maps are construction-time indexes; execution order itself is held in
// canonical arrays/slices.
type WTOPlan[Cell comparable] struct {
	cells    []Cell
	elements []WTOElement[Cell]
	index    map[Cell]int
	rank     map[Cell]int
	edges    map[edge[Cell]]struct{}
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
	var assign func([]WTOElement[Cell])
	assign = func(items []WTOElement[Cell]) {
		for _, item := range items {
			rank[item.Vertex] = len(rank)
			assign(item.Body)
		}
	}
	assign(elements)
	return &WTOPlan[Cell]{cells: canonical, elements: elements, index: index, rank: rank, edges: edges}
}

func weakTopologicalOrder[Cell comparable](cells []Cell, declared map[Cell]int, successors func(Cell) []Cell) []WTOElement[Cell] {
	dfn := make(map[Cell]int, len(cells))
	stack := make([]Cell, 0, len(cells))
	next := 0
	const done = int(^uint(0) >> 1)
	var visit func(Cell, *[]WTOElement[Cell]) int
	visit = func(v Cell, partition *[]WTOElement[Cell]) int {
		next++
		dfn[v] = next
		stack = append(stack, v)
		head := next
		loop := false
		for _, w := range successors(v) {
			if _, ok := declared[w]; !ok {
				continue
			}
			minimum := dfn[w]
			if minimum == 0 {
				minimum = visit(w, partition)
			}
			if minimum <= head {
				head = minimum
				loop = true
			}
		}
		if head == dfn[v] {
			dfn[v] = done
			element := WTOElement[Cell]{Vertex: v}
			if loop {
				for {
					last := len(stack) - 1
					w := stack[last]
					stack = stack[:last]
					dfn[w] = 0
					if w == v {
						break
					}
				}
				dfn[v] = done
				body := make([]WTOElement[Cell], 0)
				for _, w := range successors(v) {
					if _, ok := declared[w]; ok && dfn[w] == 0 {
						visit(w, &body)
					}
				}
				element.Body = body
			} else {
				stack = stack[:len(stack)-1]
			}
			*partition = append([]WTOElement[Cell]{element}, (*partition)...)
		}
		return head
	}
	partition := make([]WTOElement[Cell], 0)
	for _, cell := range cells {
		if dfn[cell] == 0 {
			visit(cell, &partition)
		}
	}
	return partition
}

func (p *WTOPlan[Cell]) matches(cells []Cell) bool {
	if p == nil {
		return false
	}
	if len(cells) != len(p.cells) {
		cells = uniqueCells(cells)
		if len(cells) != len(p.cells) {
			return false
		}
	}
	for i, cell := range cells {
		if p.cells[i] != cell {
			return false
		}
	}
	return true
}

// coversInfluence accepts declared graph edges and dynamic forward reads. A
// forward edge preserves a WTO: its source is stabilized before its reader in
// every containing-component iteration. A new backward edge can create an
// unplanned component and therefore requires FIFO fallback.
func (p *WTOPlan[Cell]) coversInfluence(from, to Cell) bool {
	if from == to {
		return true
	}
	if _, ok := p.edges[edge[Cell]{from: from, to: to}]; ok {
		return true
	}
	fromRank, fromOK := p.rank[from]
	toRank, toOK := p.rank[to]
	return fromOK && toOK && fromRank < toRank
}

// SolveWTO computes the canonical structured solution for plan.
func SolveWTO[Cell comparable, State any](sys EquationSystem[Cell, State], plan *WTOPlan[Cell]) (map[Cell]State, error) {
	result, _, err := solveWTOSystem(sys, plan, nil, false)
	return result, err
}

// SolveWTOWithVersions is the uncancelable version-reporting structured
// solver. It avoids cancellation polling on the default batch hot path.
func SolveWTOWithVersions[Cell comparable, State any](sys EquationSystem[Cell, State], plan *WTOPlan[Cell]) (map[Cell]State, map[Cell]uint64, error) {
	return solveWTOSystem(sys, plan, nil, true)
}

// SolveWTOContextWithVersions is the cancelable, version-reporting structured
// solver. All mutation is scratch-owned; error returns publish no maps.
func SolveWTOContextWithVersions[Cell comparable, State any](ctx context.Context, sys EquationSystem[Cell, State], plan *WTOPlan[Cell]) (map[Cell]State, map[Cell]uint64, error) {
	cancel := newCancellationGuard(cancellation.FromContext(ctx))
	return solveWTOSystem(sys, plan, cancel, true)
}

func solveWTOSystem[Cell comparable, State any](sys EquationSystem[Cell, State], plan *WTOPlan[Cell], cancel *cancellationGuard, includeVersions bool) (map[Cell]State, map[Cell]uint64, error) {
	validateEquationSystem(sys)
	if plan == nil || !plan.matches(sys.Cells) {
		return nil, nil, ErrWTOPlanUncovered
	}
	if err := cancel.err(0); err != nil {
		return nil, nil, err
	}
	s := newStructuredState(sys)
	// Structured ascent owns scheduling. Avoid FIFO queue/dependency storage on
	// its hot path; versions and widening history remain the ordinary solver's.

	uncovered := false
	read := func(d Cell) State {
		if !plan.coversInfluence(d, s.active) {
			uncovered = true
		}
		return s.curOf(d)
	}
	emit := func(d Cell, value State) { s.emitStructured(d, value) }
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
		if s.transferVersioned != nil {
			s.transferVersioned(cell, readVersioned, emit)
		} else {
			s.transfer(cell, read, emit)
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
		return nil, nil, err
	}
	if err := s.runNarrowing(cancel); err != nil {
		return nil, nil, err
	}
	if err := cancel.err(0); err != nil {
		return nil, nil, err
	}
	result := s.materialize()
	if !includeVersions {
		return result, nil, nil
	}
	return result, s.materializeVersions(), nil
}

func uniqueCells[Cell comparable](cells []Cell) []Cell {
	out := make([]Cell, 0, len(cells))
	seen := make(map[Cell]struct{}, len(cells))
	for _, cell := range cells {
		if _, ok := seen[cell]; ok {
			continue
		}
		seen[cell] = struct{}{}
		out = append(out, cell)
	}
	return out
}
