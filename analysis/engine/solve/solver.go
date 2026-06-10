// Package solve implements a deterministic fixed-point engine over the
// lattice.Lattice contract.
//
// A flow analysis is, abstractly, a system of monotone equations over an
// abstract domain: a finite set of cells (program points, path keys, summary
// slots) each carrying an element of a lattice, related by transfer functions
// that read some cells and contribute to others. The least solution of that
// system — the least fixed point of the combined transfer function — is the
// analysis result. This package computes it generically, so callers supply only
// their Lattice value and transfer functions rather than re-deriving a worklist
// loop each time.
//
// The algorithm is Kildall's worklist iteration (Gary A. Kildall, "A unified
// approach to global program optimization", POPL 1973) with join-accumulation
// at every cell and Cousot/Cousot widening (Patrick Cousot & Radhia Cousot,
// "Abstract interpretation: a unified lattice model …", POPL 1977) applied at
// the cells the caller marks as widening points (loop heads / feedback-vertex
// cells). The caller may delay widening for the first few strict updates after a
// widening cell's first transfer visit, the standard precision-preserving
// strategy used by practical chaotic solvers: initial predecessor fan-in and
// early feedback iterations use exact Join, then Widen guarantees convergence if
// the chain keeps growing. The accumulate-and-widen update is the iterate the
// lattice laws (monotonicity of Join, ACC of Widen at WidenAt cells) are
// designed to make terminate with a sound over-approximation of the collecting
// semantics.
//
// Determinism: the worklist is a FIFO seeded in Cells order; re-queued
// dependents are sorted by their Cells index before being appended, and the
// queue never holds a cell twice. Two Solve runs over the same system therefore
// visit cells in the same order and return identical maps. There is no use of
// maps for ordering, no randomness, and no wall-clock dependence.
//
// Termination: the solver imposes no numeric iteration cap. It relies entirely
// on the lattice contract — Join is monotone and a cell's value only ever moves
// up the order, and Widen at WidenAt cells guarantees every ascending chain is
// eventually stationary. A domain whose Widen is not a true widening (does not
// satisfy ACC) can make Solve run forever; that is a domain-law bug, not a
// concern the solver papers over with a cap.
package solve

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/lattice"
)

// EquationSystem describes a monotone equation system to be solved.
//
// Cell is the index of an equation (program point, path key, summary slot);
// it must be comparable so it can key the solver's internal maps. State is the
// abstract-domain element each cell carries.
//
// The system is solved by repeatedly applying Transfer at cells until no cell's
// value changes. Transfer reads the current value of any cells it depends on
// (via read) and contributes lattice elements to any cells it constrains (via
// emit). Dependencies are discovered dynamically through read, so the caller
// does not pre-declare the dependency graph. With no widening, the result is the
// least fixed point on finite-height systems; with widening, it is the standard
// sound post-fixpoint over-approximation of that least fixed point.
type EquationSystem[Cell comparable, State any] struct {
	// Lattice is the abstract domain. Join accumulates contributions, Equal
	// detects convergence, and Widen (at WidenAt cells) enforces termination.
	Lattice lattice.Lattice[State]

	// Cells lists every cell in a deterministic order. The order seeds the
	// worklist and tie-breaks re-queueing, so it fixes the result's
	// reproducibility. A cell not in Cells is never visited; emit to such a
	// cell still accumulates into its value and re-queues its readers, but the
	// cell's own Transfer never runs.
	Cells []Cell

	// Initial returns the starting value for a cell, conventionally
	// Lattice.Bottom(). Any value is admissible; the solver only ever moves a
	// cell up the order from its initial value via Join.
	Initial func(Cell) State

	// Transfer is the equation for one cell. It observes other cells with read
	// — which records a dependency from the read cell to this cell — and
	// contributes to cells with emit. A cell may emit to itself. Transfer must
	// be a monotone function of the values it reads; the lattice laws then make
	// the iterate sound and terminating.
	Transfer func(cell Cell, read func(Cell) State, emit func(Cell, State))

	// WidenAt reports whether emit into a cell may apply Widen rather than plain
	// Join. It is true at feedback-vertex / loop-head cells — enough cells to
	// cut every cycle in the dependency graph — so that ascending chains there
	// stabilize. If nil, no cell is widened (sound only for finite-height
	// domains, where Join itself is a widening).
	WidenAt func(Cell) bool

	// WidenDelay returns how many post-visit strict value changes a WidenAt cell
	// gets with exact Join before Widen is used. A nil function means no delay.
	// Pre-visit contributions are always joined exactly; widening starts only
	// after the destination cell has run its own Transfer once. Delaying is a
	// precision policy, not a termination cap: after the delay is exhausted the
	// same lattice Widen operator enforces convergence.
	WidenDelay func(Cell) int

	// Abstract, when non-nil, is a cell-local upper-closure applied after
	// Join/Widen and before convergence comparison. It is for sound abstractions
	// such as relevance or shape projection: Abstract(c, x) must over-approximate
	// x in the lattice order, be monotone, and be deterministic. Nil means the raw
	// Join/Widen result is stored.
	Abstract func(Cell, State) State
}

// Solve computes the converged solution of sys by Kildall worklist iteration and
// returns the value of every cell in sys.Cells.
//
// If WidenAt is nil, or every domain Widen equals Join, this is the least fixed
// point for finite-height systems. If WidenAt marks infinite-height cycles, the
// result is a sound widened post-fixpoint.
//
// The returned map has one entry per cell in sys.Cells (the value it held when
// the worklist drained). Cells that were only ever emitted into but are absent
// from sys.Cells do not appear in the result.
func Solve[Cell comparable, State any](sys EquationSystem[Cell, State]) map[Cell]State {
	s := newState(sys)
	s.run()
	return s.materialize()
}

// solveState is the mutable scratch of one Solve run.
type solveState[Cell comparable, State any] struct {
	domain lattice.Lattice[State]

	// transfer/widenAt mirror the system's functions so run does not re-read
	// the struct each visit.
	transfer   func(cell Cell, read func(Cell) State, emit func(Cell, State))
	widenAt    func(Cell) bool
	widenDelay func(Cell) int
	abstract   func(Cell, State) State

	// order is the canonical index of each cell, fixing deterministic
	// re-queue ordering. Only cells in sys.Cells receive an order; emitted-only
	// cells are not enqueued, so they need none.
	order map[Cell]int

	// cur is the working value of every cell touched so far.
	cur map[Cell]State

	// visits counts how many times a cell's own Transfer has run. widenChanges
	// counts strict post-visit changes that consumed WidenDelay.
	visits       map[Cell]int
	widenChanges map[Cell]int

	// dependents[d] is the set of cells whose Transfer has read d. When d's
	// value changes, every dependent is re-queued. Stored as an
	// insertion-ordered slice plus a membership set so the edge set dedups and
	// the iteration order is stable.
	dependents map[Cell][]Cell
	dependEdge map[edge[Cell]]struct{}

	// queue is the FIFO worklist; inQueue dedups membership.
	queue   []Cell
	inQueue map[Cell]struct{}

	// active is the cell currently in Transfer; read attributes dependency
	// edges to it.
	active Cell
}

type edge[Cell comparable] struct {
	from Cell // the read cell
	to   Cell // the reader
}

func newState[Cell comparable, State any](sys EquationSystem[Cell, State]) *solveState[Cell, State] {
	widenAt := sys.WidenAt
	if widenAt == nil {
		widenAt = func(Cell) bool { return false }
	}
	widenDelay := sys.WidenDelay
	if widenDelay == nil {
		widenDelay = func(Cell) int { return 0 }
	}
	abstract := sys.Abstract
	if abstract == nil {
		abstract = func(_ Cell, v State) State { return v }
	}
	initial := sys.Initial
	if initial == nil {
		initial = func(Cell) State { return sys.Lattice.Bottom() }
	}

	n := len(sys.Cells)
	s := &solveState[Cell, State]{
		domain:       sys.Lattice,
		transfer:     sys.Transfer,
		widenAt:      widenAt,
		widenDelay:   widenDelay,
		abstract:     abstract,
		order:        make(map[Cell]int, n),
		cur:          make(map[Cell]State, n),
		visits:       make(map[Cell]int, n),
		widenChanges: make(map[Cell]int, n),
		dependents:   make(map[Cell][]Cell),
		dependEdge:   make(map[edge[Cell]]struct{}),
		queue:        make([]Cell, 0, n),
		inQueue:      make(map[Cell]struct{}, n),
	}

	// Seed initial values and the worklist in Cells order. Deduplicate so a
	// repeated cell does not appear twice in the queue or shadow its own
	// initial value.
	for _, c := range sys.Cells {
		if _, seen := s.order[c]; seen {
			continue
		}
		s.order[c] = len(s.order)
		s.cur[c] = initial(c)
		s.enqueue(c)
	}
	return s
}

// curOf returns the working value of a cell, materializing its initial value
// (Bottom) on first touch. This handles emit into a cell absent from Cells.
func (s *solveState[Cell, State]) curOf(c Cell) State {
	if v, ok := s.cur[c]; ok {
		return v
	}
	v := s.domain.Bottom()
	s.cur[c] = v
	return v
}

// enqueue appends a cell to the FIFO unless it is already pending.
func (s *solveState[Cell, State]) enqueue(c Cell) {
	if _, pending := s.inQueue[c]; pending {
		return
	}
	s.inQueue[c] = struct{}{}
	s.queue = append(s.queue, c)
}

// recordDependency notes that the active cell read d, so a later change to d
// re-queues the active cell. The edge set dedups; dependents preserves
// insertion order for deterministic re-queueing.
func (s *solveState[Cell, State]) recordDependency(d Cell) {
	e := edge[Cell]{from: d, to: s.active}
	if e.from == e.to {
		// A cell reading itself is already re-queued by its own emit; no edge
		// needed and it keeps the dependents lists minimal.
		return
	}
	if _, ok := s.dependEdge[e]; ok {
		return
	}
	s.dependEdge[e] = struct{}{}
	s.dependents[d] = append(s.dependents[d], s.active)
}

// emit accumulates v into cell d via Join. Contributions that arrive before d's
// own transfer has run are always exact joins; this preserves high-fan-in facts
// from being widened during the initial wave. At WidenAt cells, the first
// WidenDelay(d) strict post-visit changes are kept exact; subsequent changes
// apply Widen to the previous iterate and the joined candidate. If d changed, d
// and its readers are re-queued.
func (s *solveState[Cell, State]) emit(d Cell, v State) {
	prev := s.curOf(d)
	next := s.domain.Join(prev, v)
	delayConsumed := false
	if s.widenAt(d) && s.visits[d] > 0 {
		if s.widenChanges[d] >= max(0, s.widenDelay(d)) {
			next = s.domain.Widen(prev, next)
		} else {
			delayConsumed = true
		}
	}
	next = s.abstract(d, next)
	if s.domain.Equal(next, prev) {
		return
	}
	s.cur[d] = next
	if delayConsumed {
		s.widenChanges[d]++
	}
	s.requeueChanged(d)
}

// requeueChanged re-queues a cell whose value moved and every cell that read
// it, in deterministic order. The changed cell is itself re-queued only when it
// is a system cell (present in Cells); a cell that is merely emitted into and
// has no equation of its own is never visited, per the EquationSystem contract.
func (s *solveState[Cell, State]) requeueChanged(d Cell) {
	if _, isCell := s.order[d]; isCell {
		s.enqueue(d)
	}
	deps := s.dependents[d]
	if len(deps) == 0 {
		return
	}
	// Sort dependents by their canonical Cells index so enqueue order is
	// reproducible. Cells absent from order (emitted-only) sort after ordered
	// cells, then by no further key — but such cells are never the active
	// reader, so they cannot be dependents; the missing-index branch is
	// defensive only.
	sorted := append([]Cell(nil), deps...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return s.indexOf(sorted[i]) < s.indexOf(sorted[j])
	})
	for _, dep := range sorted {
		s.enqueue(dep)
	}
}

// indexOf returns a cell's canonical order, or a sentinel past all real
// indices for cells absent from Cells.
func (s *solveState[Cell, State]) indexOf(c Cell) int {
	if i, ok := s.order[c]; ok {
		return i
	}
	return len(s.order)
}

func (s *solveState[Cell, State]) run() {
	read := func(d Cell) State {
		s.recordDependency(d)
		return s.curOf(d)
	}
	emit := func(d Cell, v State) {
		s.emit(d, v)
	}

	for len(s.queue) > 0 {
		c := s.queue[0]
		s.queue = s.queue[1:]
		delete(s.inQueue, c)

		s.active = c
		s.transfer(c, read, emit)
		s.visits[c]++
	}
}

func (s *solveState[Cell, State]) materialize() map[Cell]State {
	out := make(map[Cell]State, len(s.order))
	for c := range s.order {
		out[c] = s.cur[c]
	}
	return out
}
