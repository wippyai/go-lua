package solve

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/wippyai/go-lua/analysis/engine/cancellation"
)

// ErrWTOPlanUncovered reports that a transfer observed a backward influence
// edge absent from its immutable weak-topological plan. Callers must discard
// the scratch result and use the ordinary FIFO solver.
var ErrWTOPlanUncovered = errors.New("solve: WTO plan does not cover dynamic dependency")

// ErrWTOInvalidFrozenPlan reports that caller-supplied WTO elements and
// influences do not form an exact, closed schedule.
var ErrWTOInvalidFrozenPlan = errors.New("solve: invalid frozen WTO plan")

// ErrRetainedEvaluateUnsupported reports the intentionally narrow boundary of
// direct equations: retained replay currently owns contribution provenance for
// Transfer equations only. Rejecting the unsupported form is fail-closed and
// avoids installing incomplete bindings in a later retained update.
var ErrRetainedEvaluateUnsupported = errors.New("solve: retained direct equations are unsupported")

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
		componentCount: componentCount,
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
	return &WTOPlan[Cell]{cells: canonical, elements: elements, index: index, rank: rank, edges: edges, componentCount: componentCount}
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

// coversInfluence accepts declared graph edges and dynamic forward reads. A
// forward edge preserves a WTO: its source is stabilized before its reader in
// every containing-component iteration. A new backward edge can create an
// unplanned component and therefore requires FIFO fallback.
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
	return fromOK && toOK && fromRank < toRank
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

// SolveWTO computes the canonical structured solution for plan.
func SolveWTO[Cell comparable, State any](sys EquationSystem[Cell, State], plan *WTOPlan[Cell]) (map[Cell]State, error) {
	result, _, _, err := solveWTOSystem(sys, plan, nil, false, nil)
	return result, err
}

// SolveWTOWithVersions is the uncancelable version-reporting structured
// solver. It avoids cancellation polling on the default batch hot path.
func SolveWTOWithVersions[Cell comparable, State any](sys EquationSystem[Cell, State], plan *WTOPlan[Cell]) (map[Cell]State, map[Cell]uint64, error) {
	result, versions, _, err := solveWTOSystem(sys, plan, nil, true, nil)
	return result, versions, err
}

// SolveWTOContextWithVersions is the cancelable, version-reporting structured
// solver. All mutation is scratch-owned; error returns publish no maps.
func SolveWTOContextWithVersions[Cell comparable, State any](ctx context.Context, sys EquationSystem[Cell, State], plan *WTOPlan[Cell]) (map[Cell]State, map[Cell]uint64, error) {
	cancel := newCancellationGuard(cancellation.FromContext(ctx))
	result, versions, _, err := solveWTOSystem(sys, plan, cancel, true, nil)
	return result, versions, err
}

// RetainedSystem is an opt-in retained pre-narrowing generation. Its contents
// are intentionally opaque outside solve: regional replay must preserve the
// solver's equation ownership, revisions, and WTO history as one unit.
type RetainedSystem[Cell comparable, State any] struct {
	values              map[Cell]State
	versions            map[Cell]uint64
	valueBase           map[Cell]State
	versionBase         map[Cell]uint64
	initial             map[Cell]State
	visits              map[Cell]int
	widenChanges        map[Cell]int
	cells               []Cell
	emittedOnly         []Cell
	nextVersion         uint64
	owners              []retainedOwner[Cell, State]
	ownerDelta          map[int]retainedOwner[Cell, State]
	readers             []retainedReverse[Cell]
	outputOwners        []retainedReverse[Cell]
	outputOwnerDelta    map[Cell][]Cell
	contributions       map[Cell][]retainedContribution[Cell, State]
	contributionBase    map[Cell][]retainedContribution[Cell, State]
	contributionRemoved map[Cell]struct{}
	influences          map[Cell][]Cell
	influenceBase       map[Cell][]Cell
	cellsShared         bool
	usage               RetainedUsage
	system              EquationSystem[Cell, State]
	plan                *WTOPlan[Cell]
	budget              RetainedBudget
	generation          uint64
	updateGate          *atomic.Bool
	mu                  *sync.RWMutex
	released            bool
}

func (r *RetainedSystem[Cell, State]) Value(cell Cell) (State, bool) {
	if r == nil {
		var zero State
		return zero, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	value, ok := r.retainedValue(cell)
	return value, ok
}
func (r *RetainedSystem[Cell, State]) Version(cell Cell) uint64 {
	if r == nil {
		return 0
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.retainedVersion(cell)
}
func (r *RetainedSystem[Cell, State]) VisitCount(cell Cell) int {
	if r == nil {
		return 0
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.visits[cell]
}
func (r *RetainedSystem[Cell, State]) WidenChangeCount(cell Cell) int {
	if r == nil {
		return 0
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.widenChanges[cell]
}
func (r *RetainedSystem[Cell, State]) NextRevision() uint64 {
	if r == nil {
		return 0
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.nextVersion
}
func (r *RetainedSystem[Cell, State]) CellCount() int {
	if r == nil {
		return 0
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.cells)
}
func (r *RetainedSystem[Cell, State]) Usage() RetainedUsage {
	if r == nil {
		return RetainedUsage{}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.usage
}

func (r *RetainedSystem[Cell, State]) Release() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.values = nil
	r.versions = nil
	r.valueBase = nil
	r.versionBase = nil
	r.initial = nil
	r.visits = nil
	r.widenChanges = nil
	r.cells = nil
	r.emittedOnly = nil
	r.owners = nil
	r.ownerDelta = nil
	r.readers = nil
	r.outputOwners = nil
	r.outputOwnerDelta = nil
	r.contributions = nil
	r.contributionBase = nil
	r.contributionRemoved = nil
	r.influences = nil
	r.influenceBase = nil
	r.usage = RetainedUsage{}
	r.nextVersion = 0
	r.system = EquationSystem[Cell, State]{}
	r.plan = nil
	r.released = true
	if r.updateGate != nil {
		r.updateGate.Store(false)
	}
}

// BuildRetainedWTO performs the same canonical clean solve while retaining the
// complete ascending generation. It is deliberately separate from the default
// SolveWTO APIs, so ordinary lint pays no provenance branches or allocations.
// Cancellation, uncovered plans, and budget failures publish nothing.
func BuildRetainedWTO[Cell comparable, State any](ctx context.Context, sys EquationSystem[Cell, State], plan *WTOPlan[Cell], budget RetainedBudget) (map[Cell]State, map[Cell]uint64, *RetainedSystem[Cell, State], error) {
	if sys.Evaluate != nil {
		return nil, nil, nil, ErrRetainedEvaluateUnsupported
	}
	cancel := newCancellationGuard(cancellation.FromContext(ctx))
	recorder := newRetainedRecorder(sys.Cells, sys.Lattice, budget)
	return solveWTOSystem(sys, plan, cancel, true, recorder)
}

func solveWTOSystem[Cell comparable, State any](sys EquationSystem[Cell, State], plan *WTOPlan[Cell], cancel *cancellationGuard, includeVersions bool, recorder *retainedRecorder[Cell, State]) (map[Cell]State, map[Cell]uint64, *RetainedSystem[Cell, State], error) {
	validateEquationSystem(sys)
	if plan == nil || !plan.matches(sys.Cells) {
		return nil, nil, nil, ErrWTOPlanUncovered
	}
	if err := cancel.err(0); err != nil {
		return nil, nil, nil, err
	}
	s := newStructuredState(sys, recorder != nil)
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
	if recorder != nil {
		read = func(d Cell) State {
			if d == s.active {
				s.activeReadSelf = true
			}
			if !plan.coversInfluence(d, s.active) {
				uncovered = true
			}
			value := s.curOf(d)
			recorder.read(d, value, s.versionOf(d))
			return value
		}
		emit = func(d Cell, value State) {
			if !plan.coversEmission(s.active, d) {
				uncovered = true
			}
			recorder.emit(d, value)
			s.emitStructured(d, value)
		}
		readVersioned = func(d Cell) (State, uint64) { return read(d), s.versionOf(d) }
	}
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
		if (s.evaluate != nil && s.activeReadSelf && !plan.stabilizes(cell)) || uncovered {
			return ErrWTOPlanUncovered
		}
		s.recordVisit(cell)
		return nil
	}
	if recorder != nil {
		runCell = func(cell Cell) error {
			if err := cancel.err(iteration); err != nil {
				return err
			}
			iteration++
			s.active = cell
			recorder.begin(cell)
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
				candidate := s.evaluateVersioned(cell, readVersioned)
				recorder.emit(cell, candidate)
				s.emitStructured(cell, candidate)
			} else {
				candidate := s.evaluate(cell, read)
				recorder.emit(cell, candidate)
				s.emitStructured(cell, candidate)
			}
			if err := cancel.err(0); err != nil {
				recorder.discard()
				return err
			}
			if (s.evaluate != nil && s.activeReadSelf && !plan.stabilizes(cell)) || uncovered {
				recorder.discard()
				return ErrWTOPlanUncovered
			}
			if err := recorder.commit(); err != nil {
				recorder.discard()
				return err
			}
			s.recordVisit(cell)
			return nil
		}
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
		return nil, nil, nil, err
	}
	var retained *RetainedSystem[Cell, State]
	if recorder != nil {
		cells := append([]Cell(nil), s.cells...)
		cells = append(cells, s.emittedOrder...)
		values := make(map[Cell]State, len(cells))
		versions := make(map[Cell]uint64, len(cells))
		for _, cell := range cells {
			value, ok := s.cur[cell]
			if !ok {
				value = s.domain.Bottom()
			}
			values[cell] = value
			versions[cell] = s.versions[cell]
		}
		usage := recorder.usage
		usage.StateRefs += len(values) + len(s.initial)
		if err := retainedBudgetError(recorder.budget, usage); err != nil {
			return nil, nil, nil, err
		}
		owners, readers, outputOwners := recorder.compact(cells)
		retained = &RetainedSystem[Cell, State]{
			values: values, versions: versions, initial: cloneMap(s.initial),
			visits: cloneMap(s.visits), widenChanges: cloneMap(s.widenChanges),
			cells: cells, emittedOnly: append([]Cell(nil), s.emittedOrder...), nextVersion: s.nextVersion,
			owners: owners, readers: readers, outputOwners: outputOwners, usage: usage,
			contributions: contributionIndex(owners),
			system:        sys, plan: plan, budget: recorder.budget, generation: 1, updateGate: &atomic.Bool{}, mu: &sync.RWMutex{},
		}
		retained.rebuildInfluences()
	}
	if err := s.runNarrowing(cancel); err != nil {
		if retained != nil {
			retained.Release()
		}
		return nil, nil, nil, err
	}
	if err := cancel.err(0); err != nil {
		if retained != nil {
			retained.Release()
		}
		return nil, nil, nil, err
	}
	result := s.materialize()
	if !includeVersions {
		return result, nil, retained, nil
	}
	return result, s.materializeVersions(), retained, nil
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
