package publish

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/apply"
	applycontribution "github.com/wippyai/go-lua/analysis/engine/relation/apply/contribution"
	applydifferential "github.com/wippyai/go-lua/analysis/engine/relation/apply/differential"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/store"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/transaction"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
)

// PublishDifferential redeems one signed Before/After Apply transport through
// the publication Door.  The differential classifier owns the contribution
// subset; transaction owns the complete candidate and database.Commit remains
// the sole root publication operation.
//
// A differential with no contribution transitions is still an ordinary
// After publication.  This is important for ordinary output columns: their
// positive admission, widening, and no-write semantics remain exactly those
// of Publish.  A Before-only ordinary proposal is not a current-state write
// and is refused rather than being laundered through that positive path.
//
// Widening is meaningful only when the present After side carries a live,
// non-empty proposal batch.  In particular, a caller cannot attach a permit
// to a Before-only removal and make it look like an After write.
func (door Door) PublishDifferential(
	base database.Version,
	scratch *store.ReadScratch,
	value applydifferential.Differential,
	widening witness.WideningPermit,
) Settlement {
	if !door.Available() || !door.acceptsBase(base) || !value.Available() || !value.Fence().Same(door.mounted.RuntimeFence()) {
		return Settlement{}
	}

	transitions, transitionsOK := applycontribution.TransitionsForDifferential(door.mounted, value)
	if !transitionsOK {
		return Settlement{}
	}

	before, beforeOK := value.Before()
	after, afterOK := value.After()
	if !beforeOK && !afterOK {
		return Settlement{}
	}

	beforeWrites := differentialWrites(before, beforeOK)
	afterWrites := differentialWrites(after, afterOK)

	if len(transitions) == 0 {
		// The signed path has no contribution authority to redeem.  An
		// After-only ordinary result is the existing positive publication
		// contract, including its authenticated outcome and one commit.
		if afterOK {
			if !afterWrites {
				return door.Publish(base, nil, after, witness.WideningPermit{})
			}
			return door.Publish(base, scratch, after, widening)
		}
		// Before-only ordinary writes are historical evidence, not a current
		// state proposal. A terminal/no-write Before can still be retained as
		// an ordinary no-write settlement.
		if beforeWrites {
			return Settlement{}
		}
		return door.Publish(base, nil, before, witness.WideningPermit{})
	}

	// Every side that contributes a proposal to a signed transition must carry
	// an authenticated publishing outcome. This prevents a forged terminal
	// outcome from authorizing a state write while retaining opaque rows exactly
	// like produced rows.
	if (beforeWrites && !publishes(before)) || (afterWrites && !publishes(after)) {
		return Settlement{}
	}
	result, resultOK := publishingDifferentialOutcome(before, beforeOK, after, afterOK)
	if !resultOK {
		return Settlement{}
	}

	// Only the actual write-bearing After may redeem recurrence widening. A
	// Before-only removal always reaches transaction with the ordinary zero
	// permit, even if its caller supplied unrelated recurrence evidence.
	admittedWidening := witness.WideningPermit{}
	if afterWrites {
		admittedWidening = widening
	}
	batch, batchOK := transaction.NewDifferentialSubmissionBatch(value, admittedWidening, transitions)
	if !batchOK || !batch.Available() || scratch == nil || !scratch.Available() {
		return Settlement{}
	}
	prepared, preparedOK := transaction.Prepare(base, door.geometry, scratch, batch)
	if !preparedOK || !prepared.Available() {
		return Settlement{}
	}
	next, delta, committedOK := database.Commit(prepared)
	if !committedOK {
		return Settlement{}
	}
	if !delta.Available() {
		if !next.Same(base) {
			return Settlement{}
		}
		settlement, settlementOK := publishingNoWrite(result, base)
		if !settlementOK {
			return Settlement{}
		}
		return settlement
	}
	settlement, settlementOK := committed(result, base, next, delta)
	if !settlementOK {
		return Settlement{}
	}
	return settlement
}

// differentialWrites identifies write intent from the retained side itself.
// It deliberately does not infer a write from an outcome code: a publishing
// no-op (Produced or Opaque) carries an authenticated empty ProposalBatch and
// must not redeem widening or state transaction resources.
func differentialWrites(application apply.Application, present bool) bool {
	if !present || !application.Available() {
		return false
	}
	proposals, ok := application.Proposals()
	return ok && proposals.Available() && proposals.Len() != 0
}

func publishes(application apply.Application) bool {
	return application.Available() && application.Outcome().Code.Publishes()
}

// publishingDifferentialOutcome retains an authenticated publishing result
// from one of the actual Differential sides. After is preferred when both
// sides publish because it is the successor observation; Before is retained
// for a legitimate Before-only removal. No synthetic result exists.
func publishingDifferentialOutcome(
	before apply.Application,
	beforeOK bool,
	after apply.Application,
	afterOK bool,
) (outcome.Result, bool) {
	if afterOK && publishes(after) {
		return after.Outcome(), true
	}
	if beforeOK && publishes(before) {
		return before.Outcome(), true
	}
	return outcome.Result{}, false
}
