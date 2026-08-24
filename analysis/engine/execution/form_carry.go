// form_carry.go owns the WT form: one exact read folded onto one exact write
// whose carried prior fact passes through an owner-issued transform.

package execution

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/factbinding"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/scalar"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// CarryWrite is one immutable typed transformed-carry write: the row's own
// exact write, plus the sealed closure of carried coordinates and the
// owner-issued map their prior facts pass through.
//
// The map is sealed here rather than supplied per invocation for two reasons.
// It is a property of the rule, not of a row, so nothing per row needs to
// carry it; and it owes one law that can only be proven once - it must fix the
// Factor's declared default, because a map that moves the default invents a
// fact at every coordinate the Factor never wrote.
type CarryWrite[K scalar.Key, V any] struct {
	write   ExactWrite[K, V]
	closure factbinding.TransformClosure[K, V]
	carry   func(V) (V, bool)
}

// NewCarryWrite seals one transformed carry against a typed binding: the row's
// write target, the carried target closure, and the owner's map.
func NewCarryWrite[K scalar.Key, V any](binding *factbinding.Binding[K, V], target carrier.Target, output uint16, carried []carrier.Target, carry func(V) (V, bool)) (CarryWrite[K, V], bool) {
	write, writeOK := NewExactWrite(binding, target, output)
	if !writeOK || carry == nil || len(carried) == 0 {
		return CarryWrite[K, V]{}, false
	}
	closure, closureOK := binding.TransformClosure(carried)
	if !closureOK {
		return CarryWrite[K, V]{}, false
	}
	fallback, defaultOK := binding.Default()
	mapped, mappedOK := carry(fallback)
	if !defaultOK || !mappedOK || !binding.Equal(fallback, mapped) {
		return CarryWrite[K, V]{}, false
	}
	return CarryWrite[K, V]{write: write, closure: closure, carry: carry}, true
}

// Valid proves the write still names a live declared Target and a sealed map.
func (write CarryWrite[K, V]) Valid() bool {
	return write.write.Valid() && write.carry != nil
}

// Carry applies the sealed map over every carried coordinate of this
// invocation, into the same patch the row write stages into and before it. The
// carried coverage is the one the solver resolved from its own contribution and
// handed to the invocation; nothing here reopens the contribution plane.
func (write CarryWrite[K, V]) Carry(ticket Ticket, scratch *Scratch[K, V], when support.Mask) bool {
	if !write.Valid() || scratch == nil {
		return false
	}
	carried, carriedOK := ticket.carriedCoverage()
	patch, patchOK := write.write.begin(ticket, scratch)
	if !carriedOK || !patchOK {
		return false
	}
	return patch.Transform(write.closure, carried, when, write.carry)
}

// Stage writes the row's own fact at its exact coordinate.
func (write CarryWrite[K, V]) Stage(ticket Ticket, scratch *Scratch[K, V], when support.Mask, value V) bool {
	return write.Valid() && write.write.Stage(ticket, scratch, when, value)
}

// Close seals the one patch holding both the transformed carry and the row.
func (write CarryWrite[K, V]) Close(ticket Ticket, scratch *Scratch[K, V]) bool {
	return write.Valid() && write.write.Close(ticket, scratch)
}

// CarryReducer is the domain judgment of one transformed-carry row. It is a
// type parameter instantiated with the family's own concrete types, so the
// call below is a static direct call: no interface value, no closure and no
// function field per row.
//
// The read fact and the published fact are separate type parameters. A rule
// that folds a foreign axis onto its own reads one domain's value and writes
// another's, and there is no sense in which those are one type; a rule that
// reads the Factor it writes instantiates both with the same one.
//
// Reduce answers the fact this row publishes at its own coordinate from the
// one cell it read. Sparse absence is handed over rather than hidden, because
// whether an unwritten predecessor is a candidate at all is the domain's
// judgment and not the fold's.
type CarryReducer[R, V any] interface {
	Reduce(read R, present bool) (V, structure.ReductionOutcome)
}

// FoldCarry is the WT fold: one exact cell is reduced to the fact the row
// publishes, every carried coordinate passes through the owner's map, and both
// land in one patch so the row publishes atomically. An empty cursor is
// NoCandidate, as it is for the identity fold; any other cursor failure is
// Refuse. Ticket remains open for Submit.
//
// The read and the write are sealed against their own Factors and carry their
// own lanes. The read lane belongs to the Factor being read - foreign or the
// writing one - because a cursor is typed by the binding it steps through;
// the write lane holds the one patch both the row and the carry land in. The
// support region the write publishes under is the region the read reported,
// so a foreign read does not widen what the row claims.
func FoldCarry[RK scalar.Key, RV any, K scalar.Key, V any, R CarryReducer[RV, V]](
	ticket Ticket, reducer R,
	read ExactRead[RK, RV], reads *Scratch[RK, RV],
	write CarryWrite[K, V], writes *Scratch[K, V],
) structure.ReductionOutcome {
	if reads == nil || writes == nil || !read.Valid() || !write.Valid() {
		return structure.Refuse
	}
	switch read.Read(ticket, reads) {
	case ReadAvailable:
	case ReadExhausted:
		if read.Close(ticket, reads) {
			return structure.NoCandidate
		}
		return structure.Refuse
	default:
		_ = reads.Discard(ticket)
		return structure.Refuse
	}
	region, regionOK := reads.Region()
	value, valueOK := reads.Value()
	present := reads.Present()
	if !read.Close(ticket, reads) || !regionOK || !valueOK {
		_ = reads.Discard(ticket)
		return structure.Refuse
	}
	next, outcome := reducer.Reduce(value, present)
	if !outcome.Available() {
		return structure.Refuse
	}
	if outcome != structure.Concrete {
		// A row that publishes nothing carries nothing either: the prior facts
		// of this Factor stay exactly as the predecessor left them.
		return outcome
	}
	return PublishCarry(ticket, write, writes, region, next)
}

// PublishCarry moves every carried coordinate through the row's sealed
// transition and stages one already-reduced fact at the row's own coordinate,
// both in the one patch that makes the row publish atomically.
//
// It is the publication half of the carry form, separated from the reading
// half for the same reason PublishExact is: FoldCarry reduces exactly one
// exact cell, so a rule whose carry is fed by anything else - a product of two
// exact reads, an exact read beside a whole vector - had no fold at all. A
// family that performs its own reads reaches its publication through this, and
// the two halves compose.
//
// region is the support the reduced fact holds over, which is the region the
// reads it came from reported. The Ticket stays open for Submit.
func PublishCarry[K scalar.Key, V any](
	ticket Ticket,
	write CarryWrite[K, V],
	scratch *Scratch[K, V],
	region support.Mask,
	value V,
) structure.ReductionOutcome {
	if scratch == nil || !write.Valid() || !region.Valid() {
		return structure.Refuse
	}
	if !write.Carry(ticket, scratch, region) || !write.Stage(ticket, scratch, region, value) || !write.Close(ticket, scratch) {
		_ = scratch.Discard(ticket)
		return structure.Refuse
	}
	return structure.Concrete
}
