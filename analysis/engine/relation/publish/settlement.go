package publish

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/apply"
	applycontribution "github.com/wippyai/go-lua/analysis/engine/relation/apply/contribution"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/transaction"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
)

// The application is the semantic-to-state contract redeemed by the
// publication door.  Publication accepts the concrete apply.Application so
// there is one semantic result authority rather than an injectable facade.
//
// A result is not a cell.  In particular, NoCandidate, NoSelection, Opaque,
// and Refused must remain distinguishable after publication.
// request is the complete input to one publication attempt. It keeps the
// semantic application, which owns the exact input provenance, and the one
// optional mount-issued widening permit together. A caller cannot supply a
// separate outcome, destination batch, or lineage sidecar and have the door
// reconcile them later.
//
// The permit is zero for ordinary writes.  A non-zero permit is valid only
// when the mounted authority captured by the Door contains the exact same
// recurrence-head evidence.  The transaction must redeem this opaque value;
// it must not infer widening from a bool or dependency argument.
type request struct {
	application apply.Application
	widening    witness.WideningPermit
}

// newRequest validates the semantic/result relationship. The semantic object
// remains a live immutable lease (the proposal buffer can be reset after
// publication); Publish revalidates it immediately before redeeming the
// request.
func newRequest(application apply.Application, widening witness.WideningPermit) (request, bool) {
	input := request{application: application, widening: widening}
	if !input.validApplication() {
		return request{}, false
	}
	return input, input.Available()
}

func (request request) validApplication() bool {
	if !request.application.Available() {
		return false
	}
	result := request.application.Outcome()
	if !result.Available() {
		return false
	}
	if result.Code == outcome.Refused {
		_, hasBatch := request.application.Proposals()
		return !hasBatch
	}
	batch, ok := request.application.Proposals()
	return ok && batch.Available() && batch.Outcome() == result
}

// Available repeats the lease and cardinality checks after construction.
// This is intentionally strict: a reset proposal buffer invalidates the
// whole request instead of turning its cells into an empty result.
func (request request) Available() bool {
	if !request.validApplication() {
		return false
	}
	result := request.application.Outcome()
	if result.Code == outcome.Refused {
		return true
	}
	batch, ok := request.application.Proposals()
	if !ok {
		return false
	}
	if !result.Code.Publishes() && batch.Len() != 0 {
		return false
	}
	return true
}

// Widening returns the exact opaque permit, if this invocation is a certified
// recurrence head.  An unavailable value means ordinary non-widening writes;
// callers cannot inspect or manufacture the permit's evidence.
func (request request) Widening() witness.WideningPermit {
	if !request.Available() {
		return witness.WideningPermit{}
	}
	// A permit is a write capability, not part of the semantic outcome.
	// ProposalBatch is the upstream write-intent authority.  Terminal
	// outcomes (including Refused, which has no batch) and publishing outcomes
	// with an empty batch must therefore erase an otherwise supplied permit
	// before it can reach state/transaction. The evaluator cannot accidentally
	// make a no-write outcome depend on recurrence admission.
	proposals, ok := request.application.Proposals()
	if !ok || proposals.Len() == 0 {
		return witness.WideningPermit{}
	}
	return request.widening
}

// batch returns the application-owned proposal lease paired with its
// application-owned provenance. It is the only place the publication layer
// constructs the transaction input; callers cannot submit a second
// outcome/batch/lineage tuple.
func (request request) batch(mounted witness.Mounted) (transaction.SubmissionBatch, bool) {
	if !request.Available() {
		return transaction.SubmissionBatch{}, false
	}
	result := request.application.Outcome()
	proposals, hasBatch := request.application.Proposals()
	if result.Code == outcome.Refused {
		return transaction.SubmissionBatch{}, !hasBatch
	}
	if !hasBatch || !proposals.Available() || proposals.Outcome() != result {
		return transaction.SubmissionBatch{}, false
	}
	// The mounted plan is the one declaration authority for the contribution
	// subset. The publication door is the only place that classifies an
	// Application into contribution transitions; callers cannot carry a
	// parallel contribution sidecar.
	contributions, ok := applycontribution.TransitionsForApplication(mounted, request.application)
	if !ok {
		return transaction.SubmissionBatch{}, false
	}
	batch, batchOK := transaction.NewSubmissionBatch(request.application, request.Widening(), contributions)
	return batch, batchOK
}

// Settlement is the authenticated result of one publication attempt.  It
// always preserves the semantic outcome and exact immutable state roots.  A
// successful no-write outcome has Base == Next and no Delta; a committed
// ascent has a delta whose roots are exactly Base and Next.
//
// The zero value is unavailable.  There is no bool/error projection here:
// callers must inspect Available and Outcome, so a valid Refused result is
// not confused with an invalid publication attempt.
type Settlement struct {
	result   outcome.Result
	base     database.Version
	next     database.Version
	delta    database.Delta
	hasDelta bool
	sealed   bool
}

// Available authenticates the outcome/root relationship.  No-write
// settlements require exact root sharing; committed settlements require a
// valid database delta with matching predecessor/successor roots.
func (settlement Settlement) Available() bool {
	if settlement.sealed {
		return true
	}
	return settlement.valid()
}

func (settlement Settlement) valid() bool {
	if !settlement.result.Available() || !settlement.base.Available() || !settlement.next.Available() || !settlement.base.Fence().Same(settlement.next.Fence()) {
		return false
	}
	if settlement.hasDelta {
		return settlement.delta.Available() && settlement.delta.Base().Same(settlement.base) && settlement.delta.Next().Same(settlement.next)
	}
	return settlement.base.Same(settlement.next)
}

func sealSettlement(settlement Settlement) Settlement {
	if settlement.valid() {
		settlement.sealed = true
	}
	return settlement
}

// Outcome returns the exact semantic disposition, including Refused and its
// refusal identity.  Invalid settlements expose the unavailable zero value.
func (settlement Settlement) Outcome() outcome.Result {
	if !settlement.Available() {
		return outcome.Result{}
	}
	return settlement.result
}

// Base returns the exact immutable predecessor root.
func (settlement Settlement) Base() database.Version {
	if !settlement.Available() {
		return database.Version{}
	}
	return settlement.base
}

// Next returns the exact immutable successor root.  For no-write outcomes it
// is the same root as Base, not a fabricated empty state.
func (settlement Settlement) Next() database.Version {
	if !settlement.Available() {
		return database.Version{}
	}
	return settlement.next
}

// Delta returns the database sparse delta only when a state ascent was
// committed.  The second result distinguishes an absent delta from an
// invalid zero value.
func (settlement Settlement) Delta() (database.Delta, bool) {
	if !settlement.Available() || !settlement.hasDelta {
		return database.Delta{}, false
	}
	return settlement.delta, true
}

// Changed reports whether the publication committed a state ascent.
func (settlement Settlement) Changed() bool {
	return settlement.Available() && settlement.hasDelta
}

func noWrite(result outcome.Result, base database.Version) (Settlement, bool) {
	if !result.Available() || result.Code.Publishes() || !base.Available() {
		return Settlement{}, false
	}
	settlement := sealSettlement(Settlement{result: result, base: base, next: base})
	return settlement, settlement.Available()
}

func committed(result outcome.Result, base, next database.Version, delta database.Delta) (Settlement, bool) {
	if !result.Available() || !result.Code.Publishes() || !base.Available() || !next.Available() || !delta.Available() {
		return Settlement{}, false
	}
	settlement := sealSettlement(Settlement{result: result, base: base, next: next, delta: delta, hasDelta: true})
	return settlement, settlement.Available()
}

func publishingNoWrite(result outcome.Result, base database.Version) (Settlement, bool) {
	if !result.Available() || !result.Code.Publishes() || !base.Available() {
		return Settlement{}, false
	}
	settlement := sealSettlement(Settlement{result: result, base: base, next: base})
	return settlement, settlement.Available()
}
