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
// Termination: the widening phase imposes no numeric iteration cap. It relies
// entirely on the lattice contract — Join is monotone and a cell's value only
// ever moves up the order, and Widen at WidenAt cells guarantees every ascending
// chain is eventually stationary. A domain whose Widen is not a true widening
// (does not satisfy ACC) can make Solve run forever; that is a domain-law bug,
// not a concern the solver papers over with a cap. The optional narrowing phase
// is deliberately bounded by defaultNarrowIterations.
package solve

import (
	"context"
	"errors"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
)

// ErrCanceled reports that a context stopped a fixed-point run before it
// converged. It wraps the context cause, when one is available, so callers can
// also use errors.Is(err, context.Canceled) or errors.Is(err,
// context.DeadlineExceeded).
var ErrCanceled = errors.New("solve: canceled")

const cancellationCheckInterval = cancellation.EveryCheap

// Stats holds caller-owned observational counters for one or more Solve runs.
// Solve never retains ownership beyond the call and does not synchronize access.
type Stats struct {
	TransferCalls int
}

// defaultNarrowIterations bounds the decreasing pass after a widened fixpoint
// stabilizes. Two passes is the standard practical compromise: it recovers
// bounds lost to widening at loop heads while preserving unconditional
// termination independent of the domain's descending-chain height.
const defaultNarrowIterations = 2

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
	// Revisions are solve-local and have no semantic meaning outside this Solve
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
//
// Solve panics with deterministic "solve: ..." messages if sys omits a
// function hook required by the solver contract.
func Solve[Cell comparable, State any](sys EquationSystem[Cell, State]) map[Cell]State {
	validateEquationSystem(sys)
	s := newState(sys)
	s.run()
	s.runNarrowingWithoutCancellation()
	return s.materialize()
}

// SolveWithVersions computes the same solution as Solve and additionally
// returns the final solve-local revision for each declared cell. The revision
// changes whenever the solver replaces a cell, including during narrowing.
// It is intended only for validating ephemeral observations before a result is
// published; callers must never serialize or cache it across solves.
func SolveWithVersions[Cell comparable, State any](sys EquationSystem[Cell, State]) (map[Cell]State, map[Cell]uint64) {
	validateEquationSystem(sys)
	s := newState(sys)
	s.run()
	s.runNarrowingWithoutCancellation()
	return s.materialize(), s.materializeVersions()
}

// SolveContext computes the converged solution of sys, stopping cleanly when
// ctx is canceled. A canceled solve returns no result map, so callers cannot
// accidentally treat a partially converged worklist as a solution.
func SolveContext[Cell comparable, State any](ctx context.Context, sys EquationSystem[Cell, State]) (map[Cell]State, error) {
	validateEquationSystem(sys)
	cancel := newCancellationGuard(cancellation.FromContext(ctx))
	if err := cancel.err(0); err != nil {
		return nil, err
	}
	s := newState(sys)
	if err := s.runWithCancellation(cancel); err != nil {
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

// SolveContextWithVersions is the cancelable counterpart of SolveWithVersions.
func SolveContextWithVersions[Cell comparable, State any](ctx context.Context, sys EquationSystem[Cell, State]) (map[Cell]State, map[Cell]uint64, error) {
	validateEquationSystem(sys)
	cancel := newCancellationGuard(cancellation.FromContext(ctx))
	if err := cancel.err(0); err != nil {
		return nil, nil, err
	}
	s := newState(sys)
	if err := s.runWithCancellation(cancel); err != nil {
		return nil, nil, err
	}
	if err := s.runNarrowing(cancel); err != nil {
		return nil, nil, err
	}
	if err := cancel.err(0); err != nil {
		return nil, nil, err
	}
	return s.materialize(), s.materializeVersions(), nil
}

// Session owns an ascending, queue-empty checkpoint for one stable equation
// system.  It is deliberately local to its caller: it retains mutable solver
// history (cells, widening counters, revisions, and discovered dependencies),
// and must never be placed in a value cache.
//
// Publish narrows a scratch clone.  Consequently neither publishing nor a
// canceled publish can mutate the checkpoint used by Resume.
type Session[Cell comparable, State any] struct {
	s *solveState[Cell, State]
}

// NewSession creates a session with the ordinary initial FIFO.  Call Ascend to
// establish its first checkpoint before calling Publish or Resume.
func NewSession[Cell comparable, State any](sys EquationSystem[Cell, State]) *Session[Cell, State] {
	validateEquationSystem(sys)
	return &Session[Cell, State]{s: newState(sys)}
}

// ReplaceTransfer swaps the dynamic equation layer for a subsequent outer
// iteration.  Static shape, initials, lattice, and widening policy belong to
// the session identity and are intentionally not replaceable here.
func (r *Session[Cell, State]) ReplaceTransfer(transfer func(Cell, func(Cell) State, func(Cell, State)), versioned func(Cell, func(Cell) (State, uint64), func(Cell, State)), stats *Stats) {
	if r == nil || r.s == nil {
		return
	}
	r.s.transfer = transfer
	r.s.transferVersioned = versioned
	r.s.evaluate = nil
	r.s.evaluateVersioned = nil
	r.s.stats = stats
}

// ReplaceEvaluate swaps a session to direct, self-owned equations. It is the
// direct counterpart of ReplaceTransfer; switching either spelling clears the
// other so a resumed session never retains an ambiguous stale binding.
func (r *Session[Cell, State]) ReplaceEvaluate(evaluate func(Cell, func(Cell) State) State, versioned func(Cell, func(Cell) (State, uint64)) State, stats *Stats) {
	if r == nil || r.s == nil {
		return
	}
	r.s.transfer = nil
	r.s.transferVersioned = nil
	r.s.evaluate = evaluate
	r.s.evaluateVersioned = versioned
	r.s.stats = stats
}

// Ascend drains the normal worklist and leaves a pre-narrowing checkpoint.
func (r *Session[Cell, State]) Ascend(ctx context.Context) error {
	if r == nil || r.s == nil {
		return nil
	}
	cancel := newCancellationGuard(cancellation.FromContext(ctx))
	if err := cancel.err(0); err != nil {
		return err
	}
	return r.s.runWithCancellation(cancel)
}

// Resume forces the supplied declared cells through the ordinary FIFO, then
// drains it.  Duplicate and out-of-order callers are normalized to Cells order
// so the persisted schedule remains deterministic.
func (r *Session[Cell, State]) Resume(ctx context.Context, cells []Cell) error {
	if r == nil || r.s == nil {
		return nil
	}
	seen := make(map[Cell]struct{}, len(cells))
	for _, c := range cells {
		if _, declared := r.s.order[c]; declared {
			seen[c] = struct{}{}
		}
	}
	for _, c := range r.s.cells {
		if _, ok := seen[c]; ok {
			r.s.enqueue(c)
		}
	}
	return r.Ascend(ctx)
}

// Publish returns a narrowed materialization of the current checkpoint.  The
// returned maps are scratch-owned and may be changed by the caller.
func (r *Session[Cell, State]) Publish(ctx context.Context) (map[Cell]State, map[Cell]uint64, error) {
	if r == nil || r.s == nil {
		return nil, nil, nil
	}
	cancel := newCancellationGuard(cancellation.FromContext(ctx))
	if err := cancel.err(0); err != nil {
		return nil, nil, err
	}
	scratch := r.s.cloneForPublish()
	if err := scratch.runNarrowing(cancel); err != nil {
		return nil, nil, err
	}
	if err := cancel.err(0); err != nil {
		return nil, nil, err
	}
	return scratch.materialize(), scratch.materializeVersions(), nil
}

// CheckpointCells returns a copy of the exact pre-narrowing declared cells.
// It is primarily useful for differential tests and diagnostics.
func (r *Session[Cell, State]) CheckpointCells() map[Cell]State {
	if r == nil || r.s == nil {
		return nil
	}
	return r.s.materializeCopy()
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

// solveState is the mutable scratch of one Solve run.
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

	// dependents[d] is the set of cells whose Transfer has read d. When d's
	// value changes, every dependent is re-queued. Stored as a canonical-order
	// slice plus a membership set so the edge set dedups and the hot requeue path
	// iterates without allocating or sorting.
	dependents map[Cell][]Cell
	dependEdge map[edge[Cell]]struct{}

	// queue is the FIFO worklist; inQueue dedups membership.
	queue   []Cell
	inQueue map[Cell]struct{}

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

func newState[Cell comparable, State any](sys EquationSystem[Cell, State]) *solveState[Cell, State] {
	return newStateWithFIFO(sys, true, true)
}

func newStructuredState[Cell comparable, State any](sys EquationSystem[Cell, State], retainInitial bool) *solveState[Cell, State] {
	return newStateWithFIFO(sys, false, retainInitial)
}

func newStateWithFIFO[Cell comparable, State any](sys EquationSystem[Cell, State], fifo, retainInitial bool) *solveState[Cell, State] {
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
		order:             make(map[Cell]int, n),
		cells:             make([]Cell, 0, n),
		cur:               make(map[Cell]State, n),
		versions:          make(map[Cell]uint64, n),
	}
	if retainInitial || sys.Lattice.Narrow != nil {
		s.initial = make(map[Cell]State, n)
	}
	if fifo {
		s.dependents = make(map[Cell][]Cell)
		s.dependEdge = make(map[edge[Cell]]struct{})
		s.queue = make([]Cell, 0, n)
		s.inQueue = make(map[Cell]struct{}, n)
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
		if fifo {
			s.enqueue(c)
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

// enqueue appends a cell to the FIFO unless it is already pending.
func (s *solveState[Cell, State]) enqueue(c Cell) {
	if _, pending := s.inQueue[c]; pending {
		return
	}
	s.inQueue[c] = struct{}{}
	s.queue = append(s.queue, c)
}

// recordDependency notes that the active cell read d, so a later change to d
// re-queues the active cell. The edge set dedups; dependents stays sorted by
// canonical Cells index so requeueChanged can run on every value change without
// building a transient sorted copy.
func (s *solveState[Cell, State]) recordDependency(d Cell) {
	e := edge[Cell]{from: d, to: s.active}
	if e.from == e.to {
		// Self-reads are tracked separately from the dependency graph so the
		// solver can distinguish a real self-read from a no-op self-emit.
		return
	}
	if _, ok := s.dependEdge[e]; ok {
		return
	}
	s.dependEdge[e] = struct{}{}
	deps := s.dependents[d]
	activeIndex := s.indexOf(s.active)
	insertAt := sort.Search(len(deps), func(i int) bool {
		return s.indexOf(deps[i]) > activeIndex
	})
	deps = append(deps, s.active)
	copy(deps[insertAt+1:], deps[insertAt:])
	deps[insertAt] = s.active
	s.dependents[d] = deps
}

// emit accumulates v into cell d via Join. Contributions that arrive before d's
// own transfer has run are always exact joins; this preserves high-fan-in facts
// from being widened during the initial wave. At WidenAt cells, the first
// WidenDelay(d) strict post-visit changes are kept exact; subsequent changes
// apply Widen to the previous iterate and the joined candidate. If d changed, d
// and its readers are re-queued.
func (s *solveState[Cell, State]) emit(d Cell, v State) {
	s.emitWithRequeue(d, v, true)
}

// emitStructured is the WTO ascent update. The nested schedule owns revisits,
// so it performs the identical lattice/widening update without FIFO work.
func (s *solveState[Cell, State]) emitStructured(d Cell, v State) {
	s.emitWithRequeue(d, v, false)
}

func (s *solveState[Cell, State]) emitWithRequeue(d Cell, v State, requeue bool) {
	prev := s.curOf(d)
	next := s.domain.Join(prev, v)
	delayConsumed := false
	if s.widenAt(d) && s.visitCount(d) > 0 {
		if s.widenChangeCount(d) >= max(0, s.widenDelay(d)) {
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
	s.bumpVersion(d)
	if delayConsumed {
		s.recordWidenChange(d)
	}
	if requeue {
		s.requeueChanged(d)
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

// requeueChanged re-queues a cell whose value moved and every cell that read
// it, in deterministic order. A changed cell is re-queued only when it is a
// system cell (present in Cells) or when it is the active cell and that active
// transfer has actually read itself in this visit. A cell that is merely
// emitted into and has no equation of its own is never visited, per the
// EquationSystem contract.
func (s *solveState[Cell, State]) requeueChanged(d Cell) {
	if d == s.active {
		if s.activeReadSelf {
			s.enqueue(d)
		} else {
			s.activeSelfChanged = true
		}
	} else if _, isCell := s.order[d]; isCell {
		s.enqueue(d)
	}
	deps := s.dependents[d]
	if len(deps) == 0 {
		return
	}
	for _, dep := range deps {
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
	_ = s.runWithCancellation(nil)
}

func (s *solveState[Cell, State]) runWithCancellation(cancel *cancellationGuard) error {
	read := func(d Cell) State {
		if d == s.active {
			s.activeReadSelf = true
			if s.activeSelfChanged {
				s.enqueue(d)
			}
		}
		s.recordDependency(d)
		return s.curOf(d)
	}
	emit := func(d Cell, v State) {
		s.emit(d, v)
	}
	readVersioned := func(d Cell) (State, uint64) {
		value := read(d)
		return value, s.versionOf(d)
	}

	var iteration uint64
	for len(s.queue) > 0 {
		if err := cancel.err(iteration); err != nil {
			return err
		}
		iteration++
		c := s.queue[0]
		s.queue = s.queue[1:]
		delete(s.inQueue, c)

		s.active = c
		s.activeReadSelf = false
		s.activeSelfChanged = false
		if s.stats != nil {
			s.stats.TransferCalls++
		}
		if s.transfer != nil {
			if s.transferVersioned != nil {
				s.transferVersioned(c, readVersioned, emit)
			} else {
				s.transfer(c, read, emit)
			}
		} else if s.evaluateVersioned != nil {
			s.emit(c, s.evaluateVersioned(c, readVersioned))
		} else {
			s.emit(c, s.evaluate(c, read))
		}
		// Transfer callbacks cannot return errors. Check at their commit boundary
		// so a callback that noticed cancellation cannot publish its partial work.
		if err := cancel.err(0); err != nil {
			return err
		}
		s.recordVisit(c)
	}
	return cancel.err(iteration)
}

func (s *solveState[Cell, State]) runNarrowingWithoutCancellation() {
	s.runNarrowing(nil)
}

func (s *solveState[Cell, State]) runNarrowing(cancel *cancellationGuard) error {
	if s.domain.Narrow == nil || !s.hasWiden || len(s.cells) == 0 {
		return nil
	}
	for i := 0; i < defaultNarrowIterations; i++ {
		if err := cancel.err(uint64(i * cancellationCheckInterval)); err != nil {
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
	return cancel.err(0)
}

func cloneMap[K comparable, V any](in map[K]V) map[K]V {
	if in == nil {
		return nil
	}
	out := make(map[K]V, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// cloneForPublish copies all mutable maps, including versions.  State values
// themselves are conventionally persistent snapshots, so shallow map copies
// are the required ownership boundary here.
func (s *solveState[Cell, State]) cloneForPublish() *solveState[Cell, State] {
	copy := *s
	copy.cells = append([]Cell(nil), s.cells...)
	copy.emittedOrder = append([]Cell(nil), s.emittedOrder...)
	copy.order = cloneMap(s.order)
	copy.cur = cloneMap(s.cur)
	copy.versions = cloneMap(s.versions)
	copy.initial = cloneMap(s.initial)
	copy.visits = cloneMap(s.visits)
	copy.widenChanges = cloneMap(s.widenChanges)
	copy.dependents = make(map[Cell][]Cell, len(s.dependents))
	for k, v := range s.dependents {
		copy.dependents[k] = append([]Cell(nil), v...)
	}
	copy.dependEdge = cloneMap(s.dependEdge)
	copy.queue = nil
	copy.inQueue = make(map[Cell]struct{})
	return &copy
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

func (s *solveState[Cell, State]) materializeCopy() map[Cell]State {
	out := make(map[Cell]State, len(s.order))
	for _, c := range s.cells {
		out[c] = s.curOf(c)
	}
	return out
}

func (s *solveState[Cell, State]) materializeVersions() map[Cell]uint64 {
	out := make(map[Cell]uint64, len(s.order))
	for c := range s.order {
		out[c] = s.versions[c]
	}
	return out
}
