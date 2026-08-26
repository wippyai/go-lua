// form_route_carry.go owns the routed carry: a routed publication whose every
// published row also moves the image at its own destination through that row's
// own owner-issued transition.
//
// The carry of a routed publication is indexed by the ROW, not by the
// candidate. A candidate that publishes at one coordinate has one image to
// carry and one transition to carry it through, which is the exact form; a
// candidate that publishes at N derived destinations has N of each, and asking
// which of them is "the" carry has no answer. One closure per published row is
// the same statement at both arities: the exact form is the one-row case, and
// an identity carry is the trivial closure at every row.

package execution

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/scalar"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/operand"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// Carry moves the image at one route's destination through that row's own
// transition, into the same patch the route stages into and before it.
//
// The transition is a property of the row and arrives with it, so it is proven
// here rather than at seal: there is no earlier moment at which a row derived
// from this invocation's candidate exists. What is proven is the one law a
// carried map owes - it must fix the Factor's declared default, because a map
// that moves the default invents a fact at every coordinate the Factor never
// wrote.
func (write RouteWrite[K, V]) Carry(
	ticket Ticket,
	scratch *RouteScratch[K, V],
	target carrier.Target,
	when support.Mask,
	apply func(V) (V, bool),
) bool {
	if !write.Valid() || scratch == nil || !ticket.Valid() || apply == nil {
		return false
	}
	work, state, within, contextOK := ticket.base()
	if !contextOK || int(write.output) >= ticket.OutputCount() {
		return false
	}
	if !write.binding.ValidTarget(target) || target.Mode() != carrier.StrongTarget {
		return false
	}
	if !when.Valid() || support.Empty(when) || when.Manager() != state.Support().Manager() ||
		!when.Entails(state.Support()) || !when.Entails(within) {
		return false
	}
	fallback, defaultOK := write.binding.Default()
	mapped, mappedOK := apply(fallback)
	if !defaultOK || !mappedOK || !write.binding.Equal(fallback, mapped) {
		return false
	}
	closure, closureOK := write.binding.TargetTransformClosure(target)
	carried, carriedOK := ticket.carriedCoverage()
	if !closureOK || !carriedOK {
		return false
	}
	if scratch.issuer == nil {
		scratch.issuer = ticket.issuer
		scratch.serial = ticket.serial
		scratch.staged = 0
	} else if !scratch.validFor(ticket) {
		return false
	}
	if scratch.patch == nil {
		scratch.patch = write.binding.Begin(work, state)
		if scratch.patch == nil {
			return false
		}
	}
	return scratch.patch.Transform(closure, carried, when, apply)
}

// RouteCarry is the transition one published row carries. It is a method value
// on the row of the derived relation that produced the route, so a family
// supplies one per route and holds no function field of its own.
type RouteCarry[V any] func(V) (V, bool)

// FoldSelectedRouteCarry is the routed fold with a per-row carry: every
// selected member is reduced, the image at that member's destination moves
// through that member's own transition, and the reduced fact is staged at the
// same destination - all into the one patch that makes the row publish
// atomically.
//
// It is FoldSelectedRoute with one vector added, and it settles the same five
// dispositions for the same reasons. A carry that refuses is a Refuse, not a
// silently skipped transition: a published route whose image did not move is a
// row that published half of what it declared.
func FoldSelectedRouteCarry[D scalar.Key, K any, V any, R RouteReducer[K, V]](
	ticket Ticket,
	write RouteWrite[D, V],
	scratch *RouteScratch[D, V],
	cells []operand.SelectedCell[V],
	members []RouteMember,
	routes []K,
	carries []RouteCarry[V],
	reducer R,
) structure.ReductionOutcome {
	if scratch == nil || !write.Valid() || len(members) != len(cells) || len(routes) != len(cells) || len(carries) != len(cells) {
		return structure.Refuse
	}
	if len(cells) == 0 {
		outcome := reducer.Empty()
		if !outcome.Available() || outcome == structure.Concrete {
			return structure.Refuse
		}
		return outcome
	}
	for index, cell := range cells {
		member := members[index]
		if !member.Routed() || cell.Tag != member.Tag() || carries[index] == nil {
			_ = write.Discard(scratch)
			return structure.Refuse
		}
		value, outcome := reducer.Reduce(routes[index], cell)
		if !outcome.Available() {
			_ = write.Discard(scratch)
			return structure.Refuse
		}
		if outcome != structure.Concrete {
			_ = write.Discard(scratch)
			return outcome
		}
		if !write.Carry(ticket, scratch, member.target, cell.Region, carries[index]) {
			_ = write.Discard(scratch)
			return structure.Refuse
		}
		if !write.Stage(ticket, scratch, member.target, cell.Region, value) {
			_ = write.Discard(scratch)
			return structure.Refuse
		}
	}
	if !write.Close(ticket, scratch) {
		_ = write.Discard(scratch)
		return structure.Refuse
	}
	return structure.Concrete
}
