// form_route.go owns the WR write: one bounded routed publication whose
// destinations come from the selected relation the writer owns.
//
// A routed write is one output, not N. Every route stages into the same patch
// and the invocation seals it once, so the routed row drains exactly one patch
// and its Result count is one - the same shape an exact write has. The route
// count is bounded by the denominator the join declared; nothing here caps it
// with a constant of its own.

package execution

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/factbinding"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/scalar"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/operand"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// RouteWrite is one immutable typed routed-write descriptor. It names the
// output slot and the binding whose presealed strong targets its routes are
// drawn from. It holds no route: routes are a property of a row.
type RouteWrite[K scalar.Key, V any] struct {
	binding *factbinding.Binding[K, V]
	output  uint16
}

// NewRouteWrite seals one routed output against a typed binding.
func NewRouteWrite[K scalar.Key, V any](binding *factbinding.Binding[K, V], output uint16) (RouteWrite[K, V], bool) {
	if binding == nil {
		return RouteWrite[K, V]{}, false
	}
	return RouteWrite[K, V]{binding: binding, output: output}, true
}

// Valid proves the write still names a live typed binding.
func (write RouteWrite[K, V]) Valid() bool { return write.binding != nil }

// RouteScratch is the reusable per-worker patch a routed write stages into. One
// patch spans every route of one row, which is what makes a routed publication
// one output rather than a patch per destination.
type RouteScratch[K scalar.Key, V any] struct {
	issuer *Run
	serial uint64
	patch  *factbinding.Patch[K, V]
	staged int
}

func (scratch *RouteScratch[K, V]) validFor(ticket Ticket) bool {
	return scratch != nil && ticket.Valid() && scratch.issuer == ticket.issuer && scratch.serial == ticket.serial
}

// Stage writes one route's value at that route's own authenticated support
// region. The region belongs to the selected member the route was read at, so a
// routed write never publishes under a support row it did not observe.
func (write RouteWrite[K, V]) Stage(
	ticket Ticket,
	scratch *RouteScratch[K, V],
	target carrier.Target,
	when support.Mask,
	value V,
) bool {
	if !write.Valid() || scratch == nil || !ticket.Valid() {
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
	if !scratch.patch.WriteRouted(target, when, value) {
		return false
	}
	scratch.staged++
	return true
}

// Close seals every staged route into the Run-owned output slot. It refuses a
// row that staged nothing: a routed publication with no route is the row's
// NoSelection, concluded before any patch is opened, not an empty write.
func (write RouteWrite[K, V]) Close(ticket Ticket, scratch *RouteScratch[K, V]) bool {
	if !write.Valid() || scratch == nil || !scratch.validFor(ticket) || scratch.patch == nil || scratch.staged == 0 {
		return false
	}
	run := ticket.issuer
	if run == nil || int(write.output) >= run.outputCount || run.used[write.output] {
		return false
	}
	work, _, _, contextOK := ticket.base()
	if !contextOK {
		return false
	}
	patch, accepted := scratch.patch.Accept(work)
	if !accepted {
		scratch.finish()
		return false
	}
	run.outputs[write.output] = patch
	run.used[write.output] = true
	scratch.finish()
	return true
}

// Discard drops an unaccepted routed patch. It is the fail-closed cleanup a
// row takes when any of its routes did not settle Concrete.
func (write RouteWrite[K, V]) Discard(scratch *RouteScratch[K, V]) bool {
	if scratch == nil {
		return false
	}
	ok := true
	if scratch.patch != nil {
		ok = scratch.patch.Discard()
	}
	scratch.finish()
	return ok
}

func (scratch *RouteScratch[K, V]) finish() {
	if scratch == nil {
		return
	}
	scratch.issuer = nil
	scratch.serial = 0
	scratch.patch = nil
	scratch.staged = 0
}

// RouteReducer is the typed semantic half of one J/WR family. K is the
// destination carrier issued by the route relation; V is the fact carrier
// observed at that route. Both are consumed as type parameters and
// instantiated with the family's own concrete type, so every call below is a
// static direct call: there is no interface value, no closure and no function
// field per route, per cell, or per member.
//
// Reduce receives the exact owner-issued destination coordinate paired with
// the selected cell that the same relation member produced. The coordinate is
// supplied by the caller-owned route vector; it is never reconstructed from a
// dense position, a tag, or RouteMember's opaque write target.
//
// Empty answers the row whose derived relation produced no route at all -
// which is the one place a fold can settle the explicitly empty selection,
// because it is the only place that knows the selection is empty rather than
// unread.
type RouteReducer[K any, V any] interface {
	Reduce(route K, cell operand.SelectedCell[V]) (V, structure.ReductionOutcome)
	Empty() structure.ReductionOutcome
}

// FoldSelectedRoute is the J/WR fold: every selected member is reduced and
// staged at its own route destination, and the row settles one of the five
// declared dispositions. The routes, cells, and members are the three halves
// of one materialized relation - the destination projection, observed cell,
// and authenticated route member all come from the same ordered member row -
// so a destination is never paired with a fact observed somewhere else.
//
// Concrete requires every route to be Concrete. A row cannot publish half a
// strong write, so the first route that settles anything else settles the whole
// row and the staged patch is discarded. An empty derived relation never opens
// a patch and settles whatever the reducer concludes about its own empty
// selection, which must not be Concrete: there is no coordinate to publish at.
func FoldSelectedRoute[D scalar.Key, K any, V any, R RouteReducer[K, V]](
	ticket Ticket,
	write RouteWrite[D, V],
	scratch *RouteScratch[D, V],
	cells []operand.SelectedCell[V],
	members []RouteMember,
	routes []K,
	reducer R,
) structure.ReductionOutcome {
	if scratch == nil || !write.Valid() || len(members) != len(cells) || len(routes) != len(cells) {
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
		// The cell was observed at this member's own coordinate, so it carries
		// this member's tag. A disagreement means the cells and the members are
		// two materializations of the relation rather than one, which is the
		// re-derivation this form exists to forbid.
		if !member.Routed() || cell.Tag != member.Tag() {
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
