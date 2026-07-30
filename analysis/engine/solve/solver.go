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
// The algorithm is the deterministic Bourdoncle weak-topological iteration in
// wto.go with join accumulation and Cousot/Cousot widening at caller-declared
// feedback cells. The caller may delay widening for the first few strict
// updates; after that, the domain widening guarantees convergence.
//
// Determinism comes from the immutable WTO plan and canonical Cells order.
//
// Termination: the widening phase imposes no numeric iteration cap. It relies
// entirely on the lattice contract — Join is monotone and a cell's value only
// ever moves up the order, and Widen at WidenAt cells guarantees every ascending
// chain is eventually stationary. A domain whose Widen is not a true widening
// (does not satisfy ACC) can make the solver run forever; that is a domain-law bug,
// not a concern the solver papers over with a cap. The optional narrowing phase
// also has no artificial iteration cap. Each accepted replacement must be a
// strict decrease in the lattice order, and the domain's Narrow operator must
// make every such decreasing sequence eventually stationary. Narrowing stops
// only at equality; failure to stabilize is a domain-law bug rather than a
// condition the solver hides with a cap.
package solve

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
)

// ErrCanceled reports that a context stopped a fixed-point run before it
// converged. It wraps the context cause, when one is available, so callers can
// also use errors.Is(err, context.Canceled) or errors.Is(err,
// context.DeadlineExceeded).
var ErrCanceled = errors.New("solve: canceled")

const cancellationCheckInterval = cancellation.EveryCheap

// Stats holds caller-owned observational counters for one or more solver runs.
// The solver never retains ownership beyond the call or synchronizes access.
type Stats struct {
	TransferCalls int
}

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
	// Lattice is the abstract domain. Bottom, Equal, and Join are required; Widen
	// is also required when WidenAt is non-nil. Join accumulates contributions,
	// Equal detects convergence, and Widen (at WidenAt cells) enforces termination.
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

	// InitialSparse returns the starting value only for cells whose initial
	// value is known to differ from the lattice bottom, or whose exact starting
	// spelling matters to the caller. Cells for which it returns false start at
	// Bottom and are materialized lazily on first read/emit. When set,
	// InitialSparse takes precedence over Initial.
	InitialSparse func(Cell) (State, bool)

	// Transfer is the equation for one cell. It observes other cells with read
	// — which records a dependency from the read cell to this cell — and
	// contributes to cells with emit. A cell may emit to itself. Transfer must
	// be a monotone function of the values it reads; the lattice laws then make
	// the iterate sound and terminating.
	Transfer func(cell Cell, read func(Cell) State, emit func(Cell, State))

	// TransferVersioned is an optional observation-only spelling of Transfer.
	// When present, the main worklist uses it and supplies the exact current
	// revision of every value read. It must compute the same equation as
	// Transfer. Narrowing deliberately continues to use Transfer: a narrowing
	// candidate is not a final observation, and a final narrowing update can
	// change cur without another transfer evaluation.
	//
	// Revisions are solve-local and have no semantic meaning outside this solver
	// call. They exist so a consumer can validate a captured transfer artifact
	// against the converged working solution without re-running that transfer.
	TransferVersioned func(cell Cell, read func(Cell) (State, uint64), emit func(Cell, State))

	// Evaluate is the direct equation form for a cell whose only destination is
	// itself. The returned candidate is accumulated into cell with the same
	// Join/Widen/Abstract and revision rules as emit(cell, candidate), and every
	// declared cell owns exactly one equation. Evaluate and Transfer
	// are mutually exclusive.
	Evaluate func(cell Cell, read func(Cell) State) State

	// EvaluateVersioned is the revision-observing spelling of Evaluate. Evaluate
	// remains the narrowing authority, matching Transfer/TransferVersioned.
	EvaluateVersioned func(cell Cell, read func(Cell) (State, uint64)) State

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

	// Stats, when non-nil, receives observational counters for this solve run.
	Stats *Stats

	// UpdateObserver receives solve-local ascent updates after Join/Widen and
	// abstraction. It is observational only and cannot alter scheduling.
	UpdateObserver func(Cell, UpdateEvent[State])
}

// UpdateEvent records one candidate considered during WTO ascent. Visit is the
// number of completed visits of the destination before this update.
type UpdateEvent[State any] struct {
	Previous State
	Joined   State
	Result   State
	Visit    int
	Widened  bool
	Changed  bool
}

type cancellationGuard struct {
	token *cancellation.Token
}

func newCancellationGuard(session *cancellation.Session) *cancellationGuard {
	if session == nil {
		return nil
	}
	return &cancellationGuard{token: session.Token()}
}

func (g *cancellationGuard) err(iteration uint64) error {
	if g == nil || g.token == nil || iteration%cancellationCheckInterval != 0 {
		return nil
	}
	if g.token.Err() == nil {
		return nil
	}
	if err := g.token.Err(); err != nil {
		return errors.Join(ErrCanceled, err)
	}
	return ErrCanceled
}

func validateEquationSystem[Cell comparable, State any](sys EquationSystem[Cell, State]) {
	if sys.Transfer == nil && sys.Evaluate == nil {
		panic("solve: EquationSystem.Transfer is nil")
	}
	if sys.Transfer != nil && sys.Evaluate != nil {
		panic("solve: EquationSystem Transfer and Evaluate are mutually exclusive")
	}
	if sys.TransferVersioned != nil && sys.Transfer == nil {
		panic("solve: EquationSystem.TransferVersioned requires Transfer")
	}
	if sys.EvaluateVersioned != nil && sys.Evaluate == nil {
		panic("solve: EquationSystem.EvaluateVersioned requires Evaluate")
	}
	if sys.Lattice.Bottom == nil {
		panic("solve: EquationSystem.Lattice.Bottom is nil")
	}
	if sys.Lattice.Equal == nil {
		panic("solve: EquationSystem.Lattice.Equal is nil")
	}
	if sys.Lattice.Join == nil {
		panic("solve: EquationSystem.Lattice.Join is nil")
	}
	if sys.WidenAt != nil && sys.Lattice.Widen == nil {
		panic("solve: EquationSystem.Lattice.Widen is nil")
	}
}

// solveState is the mutable scratch of one WTO solve.
type solveState[Cell comparable, State any] struct {
	domain lattice.Lattice[State]

	// transfer/widenAt mirror the system's functions so run does not re-read
	// the struct each visit.
	transfer          func(cell Cell, read func(Cell) State, emit func(Cell, State))
	transferVersioned func(cell Cell, read func(Cell) (State, uint64), emit func(Cell, State))
	evaluate          func(cell Cell, read func(Cell) State) State
	evaluateVersioned func(cell Cell, read func(Cell) (State, uint64)) State
	widenAt           func(Cell) bool
	widenDelay        func(Cell) int
	abstract          func(Cell, State) State
	stats             *Stats
	updateObserver    func(Cell, UpdateEvent[State])
	hasWiden          bool

	// order is the canonical index of each cell, fixing deterministic
	// re-queue ordering. Only cells in sys.Cells receive an order; emitted-only
	// cells are not enqueued, so they need none.
	order map[Cell]int
	cells []Cell

	// cur is the working value of every cell touched so far.
	cur map[Cell]State
	// emittedOrder records undeclared (emitted-only) cells in first-touch order,
	// giving narrowing a deterministic iteration over cells outside Cells.
	emittedOrder []Cell
	// versions tracks solve-local revisions of cur. A narrowing replacement is
	// a replacement just like a main-worklist emit and must advance its version.
	versions    map[Cell]uint64
	nextVersion uint64
	// initial stores non-bottom declared initials so narrowing can re-apply the
	// transfer equations from the same boundary conditions without mutating cur
	// during candidate construction.
	initial map[Cell]State

	// declaredCur counts declared system cells currently present in cur. It lets
	// materialize transfer ownership of cur only when cur is exactly the declared
	// result keyset; sparse initial states and emitted-only cells otherwise fall
	// back to the filtered public result contract.
	declaredCur int

	// visits counts how many times a cell's own Transfer has run. widenChanges
	// counts strict post-visit changes that consumed WidenDelay.
	visits       map[Cell]int
	widenChanges map[Cell]int

	// active is the cell currently in Transfer; read attributes dependency
	// edges to it. activeReadSelf tracks whether the current Transfer has read
	// its own cell in this visit, and activeSelfChanged remembers whether a
	// self-emit moved the value before that self-read was observed.
	active            Cell
	activeReadSelf    bool
	activeSelfChanged bool
}

type edge[Cell comparable] struct {
	from Cell // the read cell
	to   Cell // the reader
}

func newStructuredState[Cell comparable, State any](sys EquationSystem[Cell, State], retainInitial bool) *solveState[Cell, State] {
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
	initialSparse := sys.InitialSparse

	n := len(sys.Cells)
	s := &solveState[Cell, State]{
		domain:            sys.Lattice,
		transfer:          sys.Transfer,
		transferVersioned: sys.TransferVersioned,
		evaluate:          sys.Evaluate,
		evaluateVersioned: sys.EvaluateVersioned,
		widenAt:           widenAt,
		widenDelay:        widenDelay,
		abstract:          abstract,
		stats:             sys.Stats,
		updateObserver:    sys.UpdateObserver,
		order:             make(map[Cell]int, n),
		cells:             make([]Cell, 0, n),
		cur:               make(map[Cell]State, n),
		versions:          make(map[Cell]uint64, n),
	}
	if retainInitial || sys.Lattice.Narrow != nil {
		s.initial = make(map[Cell]State, n)
	}
	// Seed initial values and the worklist in Cells order. Deduplicate so a
	// repeated cell does not appear twice in the queue or shadow its own
	// initial value.
	for _, c := range sys.Cells {
		if _, seen := s.order[c]; seen {
			continue
		}
		s.order[c] = len(s.order)
		s.cells = append(s.cells, c)
		if widenAt(c) {
			s.hasWiden = true
		}
		if initialSparse != nil {
			if value, ok := initialSparse(c); ok {
				s.cur[c] = value
				s.bumpVersion(c)
				if s.initial != nil {
					s.initial[c] = value
				}
				s.declaredCur++
			}
		} else {
			value := initial(c)
			s.cur[c] = value
			s.bumpVersion(c)
			if s.initial != nil {
				s.initial[c] = value
			}
			s.declaredCur++
		}
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
	s.bumpVersion(c)
	if _, declared := s.order[c]; declared {
		s.declaredCur++
	} else {
		s.emittedOrder = append(s.emittedOrder, c)
	}
	return v
}

func (s *solveState[Cell, State]) bumpVersion(c Cell) {
	s.nextVersion++
	s.versions[c] = s.nextVersion
}

func (s *solveState[Cell, State]) versionOf(c Cell) uint64 {
	// curOf is intentionally called first by every public read path, so a
	// missing revision here can only denote an unobserved emitted-only cell.
	return s.versions[c]
}

// emit accumulates v into cell d via Join. Contributions that arrive before d's
// own transfer has run are always exact joins; this preserves high-fan-in facts
// from being widened during the initial wave. At WidenAt cells, the first
// WidenDelay(d) strict post-visit changes are kept exact; subsequent changes
// apply Widen to the previous iterate and the joined candidate. If d changed, d
// and its readers are re-queued.
// emitStructured is the WTO ascent update. The nested schedule owns revisits,
// so it performs the identical lattice/widening update without FIFO work.
func (s *solveState[Cell, State]) emitStructured(d Cell, v State) {
	s.emitWithRequeue(d, v)
}

func (s *solveState[Cell, State]) emitWithRequeue(d Cell, v State) {
	prev := s.curOf(d)
	joined := s.domain.Join(prev, v)
	next := joined
	delayConsumed := false
	widened := false
	if s.widenAt(d) && s.visitCount(d) > 0 {
		if s.widenChangeCount(d) >= max(0, s.widenDelay(d)) {
			next = s.domain.Widen(prev, next)
			widened = true
		} else {
			delayConsumed = true
		}
	}
	next = s.abstract(d, next)
	changed := !s.domain.Equal(next, prev)
	if s.updateObserver != nil {
		s.updateObserver(d, UpdateEvent[State]{Previous: prev, Joined: joined, Result: next, Visit: s.visitCount(d), Widened: widened, Changed: changed})
	}
	if !changed {
		return
	}
	s.cur[d] = next
	s.bumpVersion(d)
	if delayConsumed {
		s.recordWidenChange(d)
	}
}

func (s *solveState[Cell, State]) visitCount(c Cell) int {
	if s.visits == nil {
		return 0
	}
	return s.visits[c]
}

func (s *solveState[Cell, State]) recordVisit(c Cell) {
	if !s.widenAt(c) {
		return
	}
	if s.visits == nil {
		s.visits = make(map[Cell]int, 1)
	}
	s.visits[c]++
}

func (s *solveState[Cell, State]) widenChangeCount(c Cell) int {
	if s.widenChanges == nil {
		return 0
	}
	return s.widenChanges[c]
}

func (s *solveState[Cell, State]) recordWidenChange(c Cell) {
	if s.widenChanges == nil {
		s.widenChanges = make(map[Cell]int, 1)
	}
	s.widenChanges[c]++
}

func (s *solveState[Cell, State]) runNarrowing(cancel *cancellationGuard) error {
	if s.domain.Narrow == nil || !s.hasWiden || len(s.cells) == 0 {
		return nil
	}
	for iteration := uint64(0); ; iteration++ {
		if err := cancel.err(iteration * uint64(cancellationCheckInterval)); err != nil {
			return err
		}
		candidate, candidateOnlyOrder, err := s.narrowingCandidate(cancel)
		if err != nil {
			return err
		}
		changed := false
		// Preserve the historical three-set narrowing coverage in deterministic
		// order: declared cells in Cells order, pre-existing emitted-only cells
		// in first-touch order, and cells created only while building this
		// candidate in their emission order. The last set must be applied in
		// this iteration: applyNarrowedCandidate materializes it in cur, which
		// would otherwise defer it until a later iteration.
		for i, c := range s.cells {
			if err := cancel.err(uint64(i)); err != nil {
				return err
			}
			if s.applyNarrowedCandidate(c, candidate) {
				changed = true
			}
		}
		for i, c := range s.emittedOrder {
			if err := cancel.err(uint64(i)); err != nil {
				return err
			}
			if s.applyNarrowedCandidate(c, candidate) {
				changed = true
			}
		}
		for i, c := range candidateOnlyOrder {
			if err := cancel.err(uint64(i)); err != nil {
				return err
			}
			if s.applyNarrowedCandidate(c, candidate) {
				changed = true
			}
		}
		if !changed {
			return nil
		}
	}
}

func (s *solveState[Cell, State]) narrowingCandidate(cancel *cancellationGuard) (map[Cell]State, []Cell, error) {
	candidate := make(map[Cell]State, len(s.cur))
	// candidateOnlyOrder records keys that are first created by an emit during
	// this candidate pass. They are absent from cur, so they cannot appear in
	// emittedOrder until applyNarrowedCandidate materializes them below.
	candidateOnlyOrder := make([]Cell, 0)
	var initialIndex uint64
	for c, value := range s.initial {
		if err := cancel.err(initialIndex); err != nil {
			return nil, nil, err
		}
		initialIndex++
		candidate[c] = value
	}
	candidateOf := func(c Cell) State {
		if value, ok := candidate[c]; ok {
			return value
		}
		value := s.domain.Bottom()
		candidate[c] = value
		return value
	}
	read := func(d Cell) State {
		return s.curOf(d)
	}
	emit := func(d Cell, v State) {
		_, candidateExists := candidate[d]
		_, declared := s.order[d]
		_, present := s.cur[d]
		if !candidateExists && !declared && !present {
			candidateOnlyOrder = append(candidateOnlyOrder, d)
		}
		prev := candidateOf(d)
		next := s.domain.Join(prev, v)
		next = s.abstract(d, next)
		candidate[d] = next
	}
	var zero Cell
	for i, c := range s.cells {
		if err := cancel.err(uint64(i)); err != nil {
			return nil, nil, err
		}
		s.active = c
		s.activeReadSelf = false
		s.activeSelfChanged = false
		if s.transfer != nil {
			s.transfer(c, read, emit)
		} else {
			value := s.evaluate(c, read)
			prev := candidateOf(c)
			candidate[c] = s.abstract(c, s.domain.Join(prev, value))
		}
		if err := cancel.err(0); err != nil {
			return nil, nil, err
		}
	}
	s.active = zero
	s.activeReadSelf = false
	s.activeSelfChanged = false
	return candidate, candidateOnlyOrder, nil
}

func (s *solveState[Cell, State]) applyNarrowedCandidate(c Cell, candidate map[Cell]State) bool {
	prev := s.curOf(c)
	nextInput, ok := candidate[c]
	if !ok {
		nextInput = s.domain.Bottom()
	}
	next := s.domain.Narrow(prev, nextInput)
	next = s.abstract(c, next)
	if !s.domain.LessOrEq(next, prev) || s.domain.Equal(next, prev) {
		return false
	}
	s.cur[c] = next
	s.bumpVersion(c)
	return true
}

func (s *solveState[Cell, State]) materialize() map[Cell]State {
	if s.declaredCur == len(s.order) && len(s.cur) == len(s.order) {
		return s.cur
	}
	out := make(map[Cell]State, len(s.order))
	for c := range s.order {
		if value, ok := s.cur[c]; ok {
			out[c] = value
		} else {
			out[c] = s.domain.Bottom()
		}
	}
	return out
}
