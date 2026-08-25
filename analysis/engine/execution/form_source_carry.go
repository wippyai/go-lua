// form_source_carry.go owns the read-free transformed carry: a row whose
// judgment rests on its candidate alone, folded onto one exact write whose
// carried prior facts pass through an owner-issued transform.

package execution

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/scalar"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// SourceCarryReducer is the semantic half of a row that reads nothing. The
// candidate is the whole of its input, so Reduce takes no cell: a rule of this
// shape answers from the directory row it is indexed by and from the cold
// knowledge its family was sealed with.
//
// It is a distinct interface from CarryReducer rather than CarryReducer with an
// ignored cell, because a fold that accepts a cell it never reads would let a
// declaration name a read the execution silently drops.
type SourceCarryReducer[V any] interface {
	Reduce() (V, structure.ReductionOutcome)
}

// FoldSourceCarry is the read-free WT fold: the row's judgment is taken from
// its candidate, every carried coordinate passes through the owner's map, and
// both land in one patch so the row publishes atomically. Ticket remains open
// for Submit.
//
// The publication holds over the invocation's own authenticated support. That
// is the honest region for this shape and not a widening: with no read there is
// no narrower evidence to hold the fact to, and the judgment is a function of
// the candidate, so it holds exactly wherever this candidate's invocation does.
// It is the same region the read-free source form publishes under.
//
// Empty support is an authenticated absent candidate rather than a refusal, and
// a row that publishes nothing carries nothing either: the prior facts of this
// Factor stay exactly as the predecessor left them.
func FoldSourceCarry[K scalar.Key, V any, R SourceCarryReducer[V]](
	ticket Ticket, reducer R,
	write CarryWrite[K, V], writes *Scratch[K, V],
) structure.ReductionOutcome {
	if writes == nil || !write.Valid() {
		return structure.Refuse
	}
	within, withinOK := ticket.Within()
	if !ticket.Valid() || !ticket.Checkpoint() || !withinOK || !within.Valid() {
		return structure.Refuse
	}
	if support.Empty(within) {
		return structure.NoCandidate
	}
	next, outcome := reducer.Reduce()
	if !outcome.Available() {
		return structure.Refuse
	}
	if outcome != structure.Concrete {
		return outcome
	}
	return PublishCarry(ticket, write, writes, within, next)
}
