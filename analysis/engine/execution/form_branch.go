// form_branch.go owns the A form's fold: the cadence that settles every
// candidate branch of one trigger.
//
// It is the structural sibling of FoldSelectedRoute. Both are invoked once per
// member of a many-valued delivery, and they differ in what a member's verdict
// means. A routed member's fold PUBLISHES a fact, so one member concluding
// anything but Concrete abandons the whole row - a row cannot publish half a
// relation. A branch's fold decides whether that branch ACTIVATES, and a
// branch the trigger does not name is the ordinary case rather than a
// refusal: the other branches still settle, and the row still concludes.

package execution

import (
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// BranchReducer concludes the disposition of one candidate branch.
//
// It returns no value because a structural publication writes no fact: its
// whole result is the disposition, which is why a structural fold declares no
// output carrier at all.
//
// It receives no cell either. A branch carries no fact any judgment consumes -
// the trigger's own value and the branch's identity settle it - and a branch
// has no coordinate of its own to be read at, so the ordinal is the whole of
// what one invocation is handed about it. Everything else the fold needs is
// the trigger's, sealed on the reducer.
type BranchReducer interface {
	// Reduce answers whether the branch at this ordinal is one the trigger
	// names. Concrete activates it, NoSelection leaves it unsettled, and any
	// other declared disposition ends the row.
	Reduce(branch uint64) structure.ReductionOutcome
	// Empty is the row's disposition when the trigger declares no branch at
	// all. A trigger with no route to instantiate is a real answer, so the
	// reducer states it rather than the form assuming one.
	Empty() structure.ReductionOutcome
}

// FoldBranchSet settles every branch of one trigger and publishes the ordinals
// of those that activated.
//
// A trigger that names none of its branches still CONCLUDES: it is a trigger
// that instantiates nothing, which stays admitted on its own declaration, and
// is not the same statement as a trigger whose evaluation refused. That is why
// an unnamed branch is skipped rather than propagated - the distinction the
// hand lane spells as an empty locator batch.
// The branch COUNT is the census the issuance pass enumerated for this trigger
// through the relation's own owner. Walking it costs nothing per branch beyond
// the reducer's own answer: an activation over a program-wide body table is
// therefore O(the targets a value names), not O(the bodies that exist.)
func FoldBranchSet[R BranchReducer](run *Run, ticket *Ticket, branches int, reducer R) structure.ReductionOutcome {
	if run == nil || ticket == nil || !ticket.Valid() || !run.Owns(*ticket) || branches < 0 {
		return structure.Refuse
	}
	if branches == 0 {
		// The empty answer may not claim to have settled branches: there were
		// none to settle, so a Concrete here would be a publication with
		// nothing behind it.
		outcome := reducer.Empty()
		if !outcome.Available() || outcome == structure.Concrete {
			return structure.Refuse
		}
		return outcome
	}
	for index := 0; index < branches; index++ {
		outcome := reducer.Reduce(uint64(index))
		if !outcome.Available() {
			return structure.Refuse
		}
		switch outcome {
		case structure.Concrete:
			// The ordinal is the branch's own address in the cold set the
			// issuance enumerated, which is the index this walk is at.
			if !run.Activate(ticket, index) {
				return structure.Refuse
			}
		case structure.NoSelection:
			// A branch this trigger does not name. The row is unaffected.
		default:
			return outcome
		}
	}
	return structure.Concrete
}
