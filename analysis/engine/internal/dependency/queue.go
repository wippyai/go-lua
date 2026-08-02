package dependency

import (
	"sync/atomic"

	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// queuePhase separates a cancellable fixed-point evaluation from its terminal
// publication cut. It is transaction control, never semantic solver state.
type queuePhase uint32

const (
	queueClosed queuePhase = iota
	queueOpening
	queueOpen
	queueCanceled
	queueFrozen
)

// queueLease is an unforgeable lifetime capability for exactly one Queue Open.
// A stale Scratch retains its old pointer, so it cannot cancel a later reuse.
type queueLease struct {
	phase    atomic.Uint32
	complete atomic.Bool
}

type participantState struct{ phase atomic.Uint32 }

const (
	participantJoined uint32 = iota
	participantPrepared
	participantSealed
)

// Participant is an opaque Queue-generation capability. Fiber obtains one at
// candidate construction, prepares it before Queue.FreezeTerminal, seals it
// after that cut, and observes Closed before releasing its joint carrier.
type Participant struct {
	queue *Queue
	lease *queueLease
	state *participantState
}

// Frozen is the one-use terminal capability minted by Queue.FreezeTerminal.
// It is deliberately opaque: only the Queue generation that crossed the
// shared Guard publication cut can complete participants or close that generation.
// Normal transaction code never recovers from a failure after receiving it.
type Frozen struct {
	queue *Queue
	lease *queueLease
}

// Closed proves that one exact frozen Queue generation completed every
// participant and released its immutable results together.
type Closed struct {
	queue *Queue
	lease *queueLease
}

// Prepare records this exact participant at Queue's fixed-point boundary.
func (participant Participant) Prepare() bool {
	if participant.queue == nil || participant.lease == nil || participant.state == nil || !participant.state.phase.CompareAndSwap(participantJoined, participantPrepared) {
		return false
	}
	if participant.queue.prepare(participant.lease) {
		return true
	}
	participant.state.phase.CompareAndSwap(participantPrepared, participantJoined)
	return false
}

// Seal completes this exact participant through the Queue's one frozen
// terminal capability. The only normal caller is Fiber finalization, after
// it has already proven every prepared typed Factor commit. A violated
// lifecycle is an internal bug, not a recoverable publication branch.
func (participant Participant) Seal(frozen Frozen) {
	if participant.queue == nil || participant.lease == nil || participant.state == nil || !frozen.matches(participant.queue, participant.lease) || !participant.state.phase.CompareAndSwap(participantPrepared, participantSealed) {
		panic("dependency: invalid participant terminal seal")
	}
	frozen.seal()
}

// Closed reports Queue's final release state for this exact generation.
func (participant Participant) Closed() bool {
	return participant.queue != nil && participant.lease != nil && participant.state != nil && participant.state.phase.Load() == participantSealed && participant.queue.lease.Load() == participant.lease && participant.lease.releasable()
}

// Cancel abandons this participant's Queue generation only while Queue is
// still open. It cannot alter Fiber or Factor candidate storage after Queue
// Freeze has won the shared publication cut.
func (participant Participant) Cancel() bool {
	return participant.queue != nil && participant.lease != nil && participant.state != nil && participant.queue.cancel(participant.lease)
}

// Abandoned reports that this exact Queue generation cannot publish: either a
// participant cancelled while open, or Queue discarded it before Freeze. It is
// deliberately false after Freeze and after successful Close, so late Fiber
// cleanup cannot mutate a generation already committed to publication.
func (participant Participant) Abandoned() bool {
	if participant.queue == nil || participant.lease == nil || participant.state == nil || participant.queue.lease.Load() != participant.lease {
		return false
	}
	switch queuePhase(participant.lease.phase.Load()) {
	case queueCanceled:
		return true
	case queueClosed:
		return !participant.lease.complete.Load()
	default:
		return false
	}
}

// SameGeneration reports whether an opaque Scratch Commit belongs to this
// Participant's exact Queue opening. It intentionally proves only the shared
// Queue generation—not a particular participant registration—so Fiber can
// bind its one joint participant to each typed Factor Scratch without exposing
// Queue leases outside dependency.
func (participant Participant) SameGeneration(commit Commit) bool {
	return participant.queue != nil && participant.lease != nil && participant.state != nil && commit.queue == participant.queue && commit.lease != nil && commit.lease == participant.lease
}

func (lease *queueLease) frozen() bool {
	return lease != nil && lease.phase.Load() == uint32(queueFrozen)
}
func (lease *queueLease) releasable() bool {
	return lease != nil && lease.complete.Load() && lease.phase.Load() == uint32(queueClosed)
}
func (lease *queueLease) cancel() bool {
	return lease != nil && lease.phase.CompareAndSwap(uint32(queueOpen), uint32(queueCanceled))
}

// Queue is a transaction-owned dense Equation work queue. The cardinality is a
// structural compiler input, not a semantic resource limit. One Solver owns
// registration, draining, preparation, freezing, sealing, and closure; only
// cancellation may arrive asynchronously.
type Queue struct {
	present   []uint64
	equations []Equation
	head      uint32
	tail      uint32
	pending   uint32
	limit     uint32

	lease atomic.Pointer[queueLease]

	guards       *guard.Work
	participants uint32
	prepared     uint32
	sealed       uint32
}

// NewQueue constructs reusable dense storage for exactly equationCount
// compiled applications.
func NewQueue(equationCount uint32) *Queue {
	return &Queue{present: make([]uint64, (uint64(equationCount)+63)/64), equations: make([]Equation, equationCount), limit: equationCount}
}

// Open starts one transaction. It rejects reentry and retains allocated queue
// storage from prior completed transactions.
func (queue *Queue) Open() bool {
	if queue == nil {
		return false
	}
	prior := queue.lease.Load()
	if prior != nil && prior.phase.Load() != uint32(queueClosed) {
		return false
	}
	lease := &queueLease{}
	lease.phase.Store(uint32(queueOpening))
	if !queue.lease.CompareAndSwap(prior, lease) {
		return false
	}
	queue.head = 0
	queue.tail = 0
	queue.pending = 0
	queue.guards = nil
	queue.participants = 0
	queue.prepared = 0
	queue.sealed = 0
	lease.phase.Store(uint32(queueOpen))
	return true
}

// usable reports whether Queue can accept exact wake delivery.
func (queue *Queue) usable(lease *queueLease) bool {
	return queue != nil && lease != nil && lease.phase.Load() == uint32(queueOpen) && queue.lease.Load() == lease
}

// validEquation rejects a compiled application outside the exact dense
// cardinality.
func (queue *Queue) validEquation(equation Equation) bool {
	return queue != nil && uint64(equation) < uint64(queue.limit)
}

// join binds one candidate to this exact open transaction and its one shared
// Guard Work. The Solver is the sole caller; cancellation does not mutate
// registration counts.
func (queue *Queue) join(guards *guard.Work) (*queueLease, bool) {
	if queue == nil || guards == nil {
		return nil, false
	}
	lease := queue.lease.Load()
	if !queue.usable(lease) {
		return nil, false
	}
	first := queue.guards == nil
	if queue.guards == nil {
		queue.guards = guards
	} else if queue.guards != guards {
		return nil, false
	}
	queue.participants++
	if !queue.usable(lease) {
		queue.participants--
		if first && queue.participants == 0 {
			queue.guards = nil
		}
		return nil, false
	}
	return lease, true
}

// Join registers one non-Factor participant with Queue's same Guard Work.
// Fiber uses it for its one joint vector/Guard-DAG candidate lifecycle; it
// must not create a second Freeze or Close protocol.
func (queue *Queue) Join(guards *guard.Work) (Participant, bool) {
	lease, valid := queue.join(guards)
	if !valid {
		return Participant{}, false
	}
	return Participant{queue: queue, lease: lease, state: &participantState{}}, true
}

// cancel stops only the matching open generation. It is safe for an
// asynchronous caller and cannot affect a later Queue reuse.
func (queue *Queue) cancel(lease *queueLease) bool {
	return lease.cancel()
}

// enqueue deduplicates one currently pending dense Equation.
func (queue *Queue) enqueue(lease *queueLease, equation Equation) bool {
	if !queue.usable(lease) || !queue.validEquation(equation) {
		return false
	}
	word, bit := uint64(equation)/64, uint64(1)<<(uint64(equation)%64)
	if queue.present[word]&bit != 0 {
		return true
	}
	if queue.pending == queue.limit {
		return false
	}
	queue.present[word] |= bit
	queue.equations[queue.tail] = equation
	queue.tail++
	if queue.tail == queue.limit {
		queue.tail = 0
	}
	queue.pending++
	return true
}

// Seed schedules one initially compiled Equation through the transaction's sole
// FIFO/dedup path. The parent Solver calls it after Open and before evaluation;
// dependency wakes use enqueue above, so initial and changed work have exactly
// the same generation, range, cancellation, order, and deduplication laws.
func (queue *Queue) Seed(equation Equation) bool {
	if queue == nil {
		return false
	}
	return queue.enqueue(queue.lease.Load(), equation)
}

// Next drains one deterministic FIFO Equation. A returned Equation is no longer
// pending and may be enqueued again by later exact changes.
func (queue *Queue) Next() (Equation, bool) {
	if queue == nil {
		return 0, false
	}
	lease := queue.lease.Load()
	if !queue.usable(lease) || queue.pending == 0 {
		return 0, false
	}
	equation := queue.equations[queue.head]
	queue.equations[queue.head] = 0
	queue.head++
	if queue.head == queue.limit {
		queue.head = 0
	}
	queue.pending--
	word, bit := uint64(equation)/64, uint64(1)<<(uint64(equation)%64)
	queue.present[word] &^= bit
	return equation, true
}

// prepare records one candidate at the fixed-point boundary.
func (queue *Queue) prepare(lease *queueLease) bool {
	if !queue.usable(lease) || queue.pending != 0 || queue.prepared == queue.participants {
		return false
	}
	queue.prepared++
	return true
}

// Freeze is the one irreversible publication cut. It succeeds only while the
// exact shared Guard Work remains open, every registered candidate is
// prepared, and no exact wake remains. Once the phase CAS wins it seals that
// exact Guard Work before returning, then Factors may seal. A concurrent
// cancellation either wins before this cut (nothing can publish) or loses
// after it (every candidate can seal).
// FreezeTerminal is the production publication cut. It returns the opaque
// capability that must be consumed by the sole terminal finalizer.
func (queue *Queue) FreezeTerminal() (Frozen, bool) {
	if queue == nil {
		return Frozen{}, false
	}
	lease := queue.lease.Load()
	if !queue.usable(lease) || queue.guards == nil || !queue.guards.Open() || queue.pending != 0 || queue.prepared != queue.participants {
		return Frozen{}, false
	}
	if !lease.phase.CompareAndSwap(uint32(queueOpen), uint32(queueFrozen)) {
		return Frozen{}, false
	}
	queue.guards.Seal()
	return Frozen{queue: queue, lease: lease}, true
}

func (queue *Queue) frozen(lease *queueLease) bool {
	return queue != nil && lease != nil && lease.phase.Load() == uint32(queueFrozen) && queue.lease.Load() == lease
}

func (frozen Frozen) matches(queue *Queue, lease *queueLease) bool {
	return frozen.queue == queue && frozen.lease == lease && queue != nil && queue.frozen(lease)
}

func (frozen Frozen) seal() {
	if !frozen.matches(frozen.queue, frozen.lease) || frozen.queue.sealed >= frozen.queue.participants {
		panic("dependency: invalid frozen Queue seal")
	}
	frozen.queue.sealed++
}

// Close completes the one frozen transaction after Fiber has sealed every
// participant. It is the final Queue-owned terminal transition; ordinary
// publication code receives Closed rather than another bool it could return
// from after Freeze.
func (frozen Frozen) Close() Closed {
	if !frozen.matches(frozen.queue, frozen.lease) || frozen.queue.sealed != frozen.queue.participants || frozen.queue.pending != 0 {
		panic("dependency: incomplete frozen Queue close")
	}
	queue, lease := frozen.queue, frozen.lease
	queue.guards = nil
	lease.phase.Store(uint32(queueClosed))
	// This is deliberately the final release store. A concurrent Scratch.Release
	// cannot observe completion until the Queue is already closed and has shed
	// every transaction-owned reference.
	lease.complete.Store(true)
	return Closed{queue: queue, lease: lease}
}

func (closed Closed) matches(queue *Queue, lease *queueLease) bool {
	return closed.queue == queue && closed.lease == lease && queue != nil && queue.lease.Load() == lease && lease.releasable()
}

// Discard abandons an evaluation before publication. Frozen work is terminal:
// it must complete through Seal and Close, never discard a published Guard.
func (queue *Queue) Discard() {
	if queue == nil {
		return
	}
	lease := queue.lease.Load()
	if lease == nil {
		return
	}
	phase := lease.phase.Load()
	if phase != uint32(queueOpen) && phase != uint32(queueCanceled) {
		return
	}
	for queue.pending != 0 {
		equation := queue.equations[queue.head]
		word, bit := uint64(equation)/64, uint64(1)<<(uint64(equation)%64)
		queue.present[word] &^= bit
		queue.equations[queue.head] = 0
		queue.head++
		if queue.head == queue.limit {
			queue.head = 0
		}
		queue.pending--
	}
	queue.tail = queue.head
	queue.guards = nil
	queue.participants = 0
	queue.prepared = 0
	queue.sealed = 0
	lease.phase.Store(uint32(queueClosed))
}
