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
// Every delivered member is observed at the window the invocation opened, so
// the prerequisite's region and the members' regions are one region, and the
// conjunction of them all is that region. This holds the delivery to exactly
// that: a member carrying a support of its own is a delivery this form has no
// way to reduce to a single conclusion, and it is refused by name rather than
// published under whichever region happened to come first. That refusal is
// what keeps the published fact from ever claiming more than every read proved.
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
		if !cell.Region.Valid() || !cell.Region.Equal(region) {
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
