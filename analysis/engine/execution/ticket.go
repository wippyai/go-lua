package execution

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/executioncatalog"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// Run owns one sealed invocation context and the reusable Ticket issuer for
// that context. inputCapacity is supplied by the sealed execution plan, not
// invented by this package; the state vector is allocated once at setup and
// copied in place on each Issue.
type Run struct {
	identity         uint64
	next             uint64
	open             uint64
	submitted        uint64
	submittedOutcome structure.ReductionOutcome
	epoch            uint64
	relationRevision uint64
	generation       uint64

	work   *carrier.Work
	base   carrier.State
	within support.Mask
	inputs []carrier.State
	// carried is the coverage of the input this rule carries, resolved by the
	// solver from its own contribution before the invocation opens. Execution
	// authenticates a transformed carry against it and never reaches back into
	// the contribution to derive one.
	carried carrier.SlotCoverage
	outputs []carrier.Patch
	drain   []carrier.Patch
	used    []bool
	// branches is the publication of a STRUCTURAL invocation: the ordinals of
	// the candidate branches this row settled. A structural row writes no fact
	// and stages no patch, so this is the whole of what it publishes. The
	// slice is retained between invocations and re-sliced rather than
	// reallocated, so a warm structural row allocates nothing.
	branches []uint32
	// branchDrain is the slice one completed invocation's branches are handed
	// over as. It aliases the staging slice's prefix until the next Issue, on
	// the same terms the patch drain buffer does.
	branchDrain []uint32

	// The issued Catalog row belongs to the same invocation authority as the
	// transaction. It is copied from sealed Program data by Issue; no worker
	// resolves a member or rule through an owner at run time.
	familyOrdinal    uint32
	localOrdinal     uint32
	inputHandles     []uint32
	outputHandles    []uint32
	outputCount      int
	ruleOrdinal      uint32
	memberOrdinal    uint32
	candidateOrdinal uint32
}

// NewRun creates one reusable family worker.  Invocation geometry is supplied
// by the immutable Catalog row at Issue, never captured by this worker.
func NewRun(inputCapacity, outputCapacity int) *Run {
	if inputCapacity < 0 || outputCapacity < 0 {
		return nil
	}
	return &Run{
		identity:    1,
		inputs:      make([]carrier.State, inputCapacity),
		outputs:     make([]carrier.Patch, outputCapacity),
		drain:       make([]carrier.Patch, outputCapacity),
		used:        make([]bool, outputCapacity),
		outputCount: outputCapacity,
	}
}

// Ticket is an opaque, one-shot value handle. It contains only the issuer,
// serial, and execution fences. Base/input States and the candidate region
// remain in Run; an Axis names an input port or the base write context when
// resolving them.
type Ticket struct {
	issuer           *Run
	serial           uint64
	epoch            uint64
	relationRevision uint64
	generation       uint64
}

// Submit is the opaque invocation's only completion operation. Run remains
// the authority; this method lets generated composition submit without
// receiving the carrier transaction or a second lifecycle wrapper.
func (ticket *Ticket) Submit(outcome structure.ReductionOutcome) bool {
	if ticket == nil || ticket.issuer == nil {
		return false
	}
	return ticket.issuer.Submit(ticket, outcome)
}

// Issue installs one sealed catalog row and current solver states into a
// reusable family worker. The row slices are immutable catalog-owned data, so
// this copies only their small addresses, never reconstructs topology.
func (run *Run) Issue(catalog *executioncatalog.Catalog, row executioncatalog.Row, work *carrier.Work, base carrier.State, within support.Mask, inputs []carrier.State, carried carrier.SlotCoverage, epoch, relationRevision, generation uint64) (Ticket, bool) {
	inputHandles, inputsOK := catalog.Inputs(row)
	outputHandles, outputsOK := catalog.Outputs(row)
	if run == nil || !inputsOK || !outputsOK || run.identity == 0 || run.open != 0 || run.submitted != 0 || run.next == ^uint64(0) || work == nil || !work.OwnsState(base) || !within.Valid() || within.Manager() != base.Support().Manager() || !within.Entails(base.Support()) || len(inputs) != len(inputHandles) || len(inputs) > len(run.inputs) || len(outputHandles) > len(run.outputs) || epoch == 0 || relationRevision == 0 || generation == 0 {
		return Ticket{}, false
	}
	for _, state := range inputs {
		if !work.OwnsState(state) || !state.Valid() || state.Support().Manager() != base.Support().Manager() {
			return Ticket{}, false
		}
	}
	run.work = work
	run.base = base
	run.within = within
	run.carried = carried
	run.epoch = epoch
	run.relationRevision = relationRevision
	run.generation = generation
	copy(run.inputs, inputs)
	run.familyOrdinal, run.localOrdinal = row.FamilyOrdinal(), row.LocalOrdinal()
	run.inputHandles = inputHandles
	run.outputHandles = outputHandles
	run.outputCount = len(outputHandles)
	run.ruleOrdinal, run.memberOrdinal, run.candidateOrdinal = row.RuleOrdinal(), row.MemberOrdinal(), row.CandidateOrdinal()
	for index := range run.outputs {
		run.outputs[index] = carrier.Patch{}
		run.used[index] = false
	}
	run.branches = run.branches[:0]
	run.next++
	run.open = run.next
	return Ticket{issuer: run, serial: run.next, epoch: epoch, relationRevision: relationRevision, generation: generation}, true
}

func (run *Run) clearInvocation() {
	if run == nil {
		return
	}
	run.work = nil
	run.base = carrier.State{}
	run.within = support.Mask{}
	run.carried = carrier.SlotCoverage{}
	for index := range run.inputs {
		run.inputs[index] = carrier.State{}
	}
	run.epoch, run.relationRevision, run.generation = 0, 0, 0
	run.familyOrdinal, run.localOrdinal = 0, 0
	run.inputHandles, run.outputHandles = nil, nil
	run.outputCount = len(run.outputs)
}

// carriedCoverage is the coverage of this invocation's carried input.
func (ticket Ticket) carriedCoverage() (carrier.SlotCoverage, bool) {
	if !ticket.Valid() {
		return carrier.SlotCoverage{}, false
	}
	return ticket.issuer.carried, true
}

// Open reports whether Run has one live opaque invocation.  It is an engine
// continuation fence, not a second scheduler state.
func (run *Run) Open() bool { return run != nil && run.open != 0 }

// Pending reports whether Submit has completed an invocation and its patches
// and outcome still await the engine continuation's Drain.
func (run *Run) Pending() bool { return run != nil && run.submitted != 0 }

// Abort revokes the current invocation or drops a submitted result.  It is
// the one fail-closed cancellation edge used by the engine continuation.
func (run *Run) Abort() bool {
	if run == nil || run.identity == 0 || run.open == 0 && run.submitted == 0 {
		return false
	}
	run.discardOutputs()
	run.open = 0
	run.submitted = 0
	run.submittedOutcome = structure.Refuse
	run.clearInvocation()
	return true
}

// Valid reports whether a Ticket still names its live issuer serial and
// complete invocation fences. It is safe on a zero Ticket.
func (ticket Ticket) Valid() bool {
	return ticket.issuer != nil && ticket.issuer.identity != 0 && ticket.serial != 0 && ticket.issuer.open == ticket.serial && ticket.epoch != 0 && ticket.relationRevision != 0 && ticket.generation != 0 && ticket.epoch == ticket.issuer.epoch && ticket.relationRevision == ticket.issuer.relationRevision && ticket.generation == ticket.issuer.generation && ticket.issuer.contextValid()
}

// RuleOrdinal, MemberOrdinal, and CandidateOrdinal expose only the sealed
// row coordinates while the ticket is live.  Once Submit/Close/Abort revokes
// the ticket, every coordinate read refuses with no stale metadata leak.
func (ticket Ticket) RuleOrdinal() (uint32, bool) {
	if !ticket.Valid() {
		return 0, false
	}
	return ticket.issuer.ruleOrdinal, true
}

func (ticket Ticket) MemberOrdinal() (uint32, bool) {
	if !ticket.Valid() {
		return 0, false
	}
	return ticket.issuer.memberOrdinal, true
}

func (ticket Ticket) CandidateOrdinal() (uint32, bool) {
	if !ticket.Valid() {
		return 0, false
	}
	return ticket.issuer.candidateOrdinal, true
}

// LocalOrdinal is this invocation's position inside the family that runs it.
// It is the one address a family needs to select the sealed row it executes,
// and it is what makes an installed family implementable outside this package:
// an installer seals its rows in the order it was handed them and answers the
// local ordinal of each, so the address it reads back is the one it minted.
func (ticket Ticket) LocalOrdinal() (uint32, bool) {
	if !ticket.Valid() {
		return 0, false
	}
	return ticket.issuer.localOrdinal, true
}

// Owns reports whether this Run issued the ticket. A worker holds the Run its
// family was built for, so this is how an installed family outside this
// package refuses a ticket from another lane rather than executing it.
func (run *Run) Owns(ticket Ticket) bool {
	return run != nil && ticket.Valid() && ticket.issuer == run
}

func (ticket Ticket) familyLocal() (uint32, uint32, bool) {
	if !ticket.Valid() {
		return 0, 0, false
	}
	return ticket.issuer.familyOrdinal, ticket.issuer.localOrdinal, true
}

func (ticket Ticket) InputCount() int {
	if !ticket.Valid() {
		return 0
	}
	return len(ticket.issuer.inputHandles)
}

func (ticket Ticket) OutputCount() int {
	if !ticket.Valid() {
		return 0
	}
	return len(ticket.issuer.outputHandles)
}

// Within returns the authenticated input support region of this live
// invocation. It is consumed by the canonical execution/product refinement;
// callers cannot construct or replace the region because Ticket remains the
// sole issuer authority.
func (ticket Ticket) Within() (support.Mask, bool) {
	if !ticket.Valid() {
		return support.Mask{}, false
	}
	return ticket.issuer.within, true
}

// Checkpoint samples the live evaluator epoch that authenticated this
// invocation. Execution-owned structural traversals use it to stop before
// publishing a partial refinement when the carrier epoch is revoked.
func (ticket Ticket) Checkpoint() bool {
	if !ticket.Valid() {
		return false
	}
	return ticket.issuer.work.Checkpoint()
}

func (ticket Ticket) InputHandleAt(index int) (uint32, bool) {
	if !ticket.Valid() || index < 0 || index >= len(ticket.issuer.inputHandles) {
		return 0, false
	}
	return ticket.issuer.inputHandles[index], true
}

func (ticket Ticket) OutputHandleAt(index int) (uint32, bool) {
	if !ticket.Valid() || index < 0 || index >= len(ticket.issuer.outputHandles) {
		return 0, false
	}
	return ticket.issuer.outputHandles[index], true
}

// Close consumes a Ticket exactly once. A copied handle observes the same
// issuer open serial and is revoked with the original.
func (ticket *Ticket) Close() bool {
	if ticket == nil || !ticket.Valid() || ticket.issuer.hasOutput() {
		return false
	}
	ticket.issuer.open = 0
	return true
}

func (run *Run) contextValid() bool {
	if run == nil || run.identity == 0 || run.work == nil || !run.work.OwnsState(run.base) || !run.within.Valid() || run.within.Manager() != run.base.Support().Manager() || !run.within.Entails(run.base.Support()) {
		return false
	}
	for _, state := range run.inputs {
		if !run.work.OwnsState(state) || !state.Valid() || state.Support().Manager() != run.base.Support().Manager() {
			return false
		}
	}
	return true
}

func (ticket Ticket) input(port uint16) (*carrier.Work, carrier.State, support.Mask, bool) {
	if !ticket.Valid() || int(port) >= len(ticket.issuer.inputs) {
		return nil, carrier.State{}, support.Mask{}, false
	}
	return ticket.issuer.work, ticket.issuer.inputs[port], ticket.issuer.within, true
}

func (ticket Ticket) base() (*carrier.Work, carrier.State, support.Mask, bool) {
	if !ticket.Valid() {
		return nil, carrier.State{}, support.Mask{}, false
	}
	return ticket.issuer.work, ticket.issuer.base, ticket.issuer.within, true
}

func (run *Run) hasOutput() bool {
	for _, used := range run.used[:run.outputCount] {
		if used {
			return true
		}
	}
	return false
}

// Activate stages one candidate branch of this invocation as settled.
//
// The branch is addressed by its ORDINAL in the cold branch set the issuance
// enumerated for this row. That is the only address a nested member set's rows
// have - it is what the relation's own Ordinal carrier names them by - so
// nothing here mints a coordinate, names a Factor, or resolves a member. Which
// mounted activation member an ordinal stands for was settled once, cold, by
// the engine that mounted it.
//
// One branch settles once. A second call for the same ordinal is two
// dispositions for one branch, which no order between them could resolve.
func (run *Run) Activate(ticket *Ticket, branch int) bool {
	if run == nil || ticket == nil || ticket.issuer != run || !ticket.Valid() {
		return false
	}
	if branch < 0 || uint64(branch) > uint64(^uint32(0)) {
		return false
	}
	ordinal := uint32(branch)
	for _, staged := range run.branches {
		if staged == ordinal {
			return false
		}
	}
	run.branches = append(run.branches, ordinal)
	return true
}

// Submit is the one final reducer boundary. It owns the fixed outcome and
// atomically transitions the live Ticket into Run's submitted state after all
// independent output slots have been sealed. Patches stay private until
// Drain copies them into an engine-owned destination. Outcome policy is not
// interpreted here; even AuthenticatedOpaque is transported unchanged for the
// sealed compiled plan to decide.
func (run *Run) Submit(ticket *Ticket, outcome structure.ReductionOutcome) bool {
	if run == nil || ticket == nil || ticket.issuer != run || !ticket.Valid() {
		return false
	}
	if !outcome.Available() {
		_ = run.Abort()
		return false
	}
	if outcome != structure.Concrete {
		run.discardOutputs()
		// A disposition that concludes no fact concludes no branch either: an
		// invocation that settled nothing has nothing staged to publish.
		run.branches = run.branches[:0]
		return run.submitTicket(ticket, outcome)
	}
	if outcome == structure.Concrete {
		for _, used := range run.used[:run.outputCount] {
			if !used {
				run.discardOutputs()
				run.submitTicket(ticket, structure.Refuse)
				return false
			}
		}
		run.submitTicket(ticket, outcome)
		return true
	}
	run.discardOutputs()
	run.submitTicket(ticket, outcome)
	return true
}

func (run *Run) submitTicket(ticket *Ticket, outcome structure.ReductionOutcome) bool {
	if run == nil || ticket == nil || ticket.issuer != run || !ticket.Valid() {
		return false
	}
	run.open = 0
	run.submitted = ticket.serial
	run.submittedOutcome = outcome
	return true
}

// Drain copies submitted carrier patches and the exact submitted outcome into
// caller-owned engine storage, then returns Run to idle. It refuses a second
// drain, a short destination, or an Issue attempted before this copy. A
// disposition with no surviving patches accepts a nil destination and returns
// count zero.
func (run *Run) Drain(destination []carrier.Patch) (structure.ReductionOutcome, int, bool) {
	if run == nil || run.submitted == 0 {
		return structure.Refuse, 0, false
	}
	count := 0
	if run.hasOutput() {
		if len(destination) < run.outputCount {
			return structure.Refuse, 0, false
		}
		copy(destination, run.outputs[:run.outputCount])
		count = run.outputCount
	}
	outcome := run.submittedOutcome
	for index := range run.outputs {
		run.outputs[index] = carrier.Patch{}
		run.used[index] = false
	}
	run.submitted = 0
	// Copied into its own buffer for the same reason the patches are: the
	// staging slice is reused by the next invocation, and a handed-over view
	// that aliased it would be rewritten underneath its reader.
	run.branchDrain = append(run.branchDrain[:0], run.branches...)
	run.branches = run.branches[:0]
	run.submittedOutcome = structure.Refuse
	run.clearInvocation()
	return outcome, count, true
}

// Consume is Drain against Run's sealed drain buffers. The returned patch and
// branch slices alias those buffers until the next Issue and allocate nothing
// on the hot path.
//
// A row publishes patches or branches and never both: a fact-writing row has
// no branch set, and a structural row publishes no Factor surface at all. The
// two are returned side by side because they are one drain - one invocation
// settles one disposition, whichever way it publishes it.
func (run *Run) Consume() (structure.ReductionOutcome, []carrier.Patch, []uint32, bool) {
	if run == nil {
		return structure.Refuse, nil, nil, false
	}
	outcome, count, ok := run.Drain(run.drain)
	if !ok {
		return outcome, nil, nil, false
	}
	branches := run.branchDrain
	if len(branches) == 0 {
		branches = nil
	}
	if count == 0 {
		return outcome, nil, branches, true
	}
	return outcome, run.drain[:count], branches, true
}

func (run *Run) discardOutputs() {
	if run == nil || run.work == nil {
		return
	}
	for index, used := range run.used[:run.outputCount] {
		if used {
			_ = run.work.Discard(run.outputs[index])
			run.outputs[index] = carrier.Patch{}
			run.used[index] = false
		}
	}
}
