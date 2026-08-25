// form_selected_exact.go owns the exact publication over a selection: one
// exact prerequisite joined to the members a derived relation named, folded
// ONCE over the whole delivery, published at the row's own coordinate.
//
// It is the counterpart of the routed form and differs from it in the one way
// that matters. A routed row publishes one fact per observed member, so each
// fact is supported by exactly the member it came from and the fold is a
// cadence. This row concludes one fact FROM every member, so its support is
// what all of its reads proved together and its fold is called once.

package execution

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/scalar"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// SelectionReducer is the typed fold of a row that concludes once over a whole
// selection. It is a type parameter rather than an interface value, so the
// call an emitted family makes into its owner's judgment stays a static direct
// call.
//
// It is handed the delivery and nothing else. Which coordinate the conclusion
// publishes at, and which support it holds over, are this form's answers and
// not the fold's - a fold that could name either would be able to publish
// somewhere its declaration did not say, or over evidence it did not read.
type SelectionReducer[V any, W any] interface {
	Reduce(cells []SelectedCell[V]) (W, structure.ReductionOutcome)
}

// FoldSelectedExact performs one invocation of the exact-over-selection form.
//
// prerequisite is the support the row's exact read proved, handed in by the
// caller because that read is the caller's own. The support the conclusion is
// published under is derived HERE, from the reads this invocation actually
// consumed: an emitted family passes regions it was handed and never states
// one of its own, because the mask type it would have to name is engine
// internal and the conjunction it would have to compute is not its to compute.
//
// The conjunction is taken by entailment. A member observed at a coordinate
// this rule itself writes - which is what a recursive call site reads - is
// proved over a WIDER support than the prerequisite, because an unwritten cell
// is absent everywhere; intersecting with it leaves the running meet alone.
// A member proved over less than everything before it narrows the meet, since
// the conclusion may only hold where every read holds. Two supports neither of
// which contains the other have a meet this cannot name, and that is refused
// by name rather than published under whichever one came first.
//
// So the published fact never claims more than every read proved, and the
// refusal is reserved for the one case the ordering cannot answer.
//
// An empty selection is not a refusal and not an absent candidate by itself.
// The fold still reaches its one conclusion, over no members, and what that
// conclusion means is its own answer; the support is the prerequisite's alone,
// because that is the whole of what this invocation read.
func FoldSelectedExact[K scalar.Key, V any, W any, R SelectionReducer[V, W]](
	ticket Ticket,
	write ExactWrite[K, W],
	scratch *Scratch[K, W],
	prerequisite support.Mask,
	cells []SelectedCell[V],
	reducer R,
) structure.ReductionOutcome {
	if scratch == nil || !write.Valid() || !prerequisite.Valid() {
		return structure.Refuse
	}
	region := prerequisite
	for _, cell := range cells {
		if !cell.Region.Valid() {
			return structure.Refuse
		}
		switch {
		case region.Entails(cell.Region):
			// The running meet is already inside this member's support, so
			// intersecting with it changes nothing.
		case cell.Region.Entails(region):
			// This member proved less than everything before it, and the
			// conclusion may only hold where every read holds.
			region = cell.Region
		default:
			return structure.Refuse
		}
	}
	value, outcome := reducer.Reduce(cells)
	if !outcome.Available() {
		return structure.Refuse
	}
	if outcome != structure.Concrete {
		return outcome
	}
	if !write.Stage(ticket, scratch, region, value) || !write.Close(ticket, scratch) {
		_ = scratch.Discard(ticket)
		return structure.Refuse
	}
	return structure.Concrete
}
