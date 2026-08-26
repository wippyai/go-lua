// form_selected.go owns the J read: a join whose coordinates are resolved per
// row from an earlier read's result, delivered as one ordered Selection under
// the mandatory read contract.
//
// The read is callback-free. A family hands this primitive the bounded ordered
// member set its sealed relation derived for one candidate row, and gets back
// one cell per member with the contract's substitutions already applied.
// Member order, sparse absence and opaque widening are decided here, once, so
// a Fold holds no positional assumption and has no absent branch to get wrong.

package execution

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/factbinding"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/scalar"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/operand"
	ruleplan "github.com/wippyai/go-lua/analysis/schema/rule/plan"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
)

// SelectedCoordinate is one member of a row's derived selection: the sealed
// exact Unit the member is observed at, paired with the owner-issued tag that
// names it. Both come from the relation the join declared; nothing here is
// minted at solve time.
type SelectedCoordinate struct {
	Unit carrier.Unit
	Tag  uint64
}

// SelectedRead is one immutable typed selected-read descriptor. It is sealed
// with a rule family and holds no epoch, Run, candidate, or owner capability.
// The contract clauses are copied from the sealed plan, never re-derived.
type SelectedRead[K scalar.Key, V any] struct {
	binding      *factbinding.Binding[K, V]
	port         uint16
	order        ruleprogram.Order
	multiplicity ruleprogram.Multiplicity
	policy       ReadCellPolicy[V]
}

// declaredCellPolicy derives one read's substitutions from its declared
// contract and the bound Factor's own algebra endpoints. Sparse and OnOpaque
// are the only two clauses that can seal one, so the derivation is total over
// them: a FactorDefault read fills an unwritten coordinate from the Factor's
// own Default, and a PropagateAuthenticated read carries the Factor's own Top
// as what a widened delivery becomes. Neither endpoint is invented here - both
// come from the same binding the read is sealed against - so the policy is a
// function of the contract, not a second declaration a caller could disagree
// with it about.
//
// Whether a delivery IS widened is the read axis's own statement and not this
// derivation's: a selection widens its whole delivery, while an exact read
// names one observed coordinate and widens only where its locator reports an
// opaque alternative.
//
// It serves both read axes. A selected read seals it internally because its
// cursor delivers the cell; an exact read leaves the delivery to its caller,
// so the caller seals the same derivation over the same binding rather than
// spelling a substitution of its own.
func declaredCellPolicy[K scalar.Key, V any](binding *factbinding.Binding[K, V], contract ruleplan.ReadContract) (ReadCellPolicy[V], bool) {
	var defaulted bool
	var fallback, top V
	if contract.Sparse == ruleprogram.SparseDefault {
		value, ok := binding.Default()
		if !ok {
			return ReadCellPolicy[V]{}, false
		}
		defaulted, fallback = true, value
	}
	if contract.OnOpaque == ruleprogram.OnOpaquePropagateAuthenticated {
		value, ok := binding.Top()
		if !ok {
			return ReadCellPolicy[V]{}, false
		}
		top = value
	}
	return NewReadCellPolicy(defaulted, fallback, top), true
}

// NewSelectedRead seals one selected read against a typed binding and the
// contract its plan row declared. Summary and Complete clauses are refused
// here: this read delivers one cell per member, and a multiplicity that says
// otherwise is a contract this read cannot carry.
//
// The sealed policy is declaredCellPolicy's derivation over contract and
// binding; the policy argument is not read. A caller cannot seal a
// substitution the contract did not declare, and the contract's Sparse and
// OnOpaque clauses are the one authority over what a delivered cell holds.
func NewSelectedRead[K scalar.Key, V any](
	binding *factbinding.Binding[K, V],
	port uint16,
	contract ruleplan.ReadContract,
	_ ReadCellPolicy[V],
) (SelectedRead[K, V], bool) {
	if binding == nil || !contract.Order.Available() || !contract.Sparse.Available() ||
		!contract.OnOpaque.Available() || !contract.Multiplicity.Available() {
		return SelectedRead[K, V]{}, false
	}
	if contract.Order != ruleprogram.OrderCanonical && contract.Order != ruleprogram.OrderByTag {
		return SelectedRead[K, V]{}, false
	}
	if contract.Multiplicity == ruleprogram.MultiplicityMany {
		return SelectedRead[K, V]{}, false
	}
	policy, sealed := declaredCellPolicy[K, V](binding, contract)
	if !sealed {
		return SelectedRead[K, V]{}, false
	}
	// A selection over an authenticated-propagating contract is widened for
	// its whole delivery: the members it spans are drawn from an alternative
	// set the read did not observe, so every cell is the Factor's Top. An
	// exact read names one observed coordinate instead, so it carries the
	// substitution alone and widens only where its own locator says the
	// alternative is opaque.
	if contract.OnOpaque == ruleprogram.OnOpaquePropagateAuthenticated {
		policy = policy.Widen()
	}
	return SelectedRead[K, V]{
		binding:      binding,
		port:         port,
		order:        contract.Order,
		multiplicity: contract.Multiplicity,
		policy:       policy,
	}, true
}

// Valid proves the read still names a live typed binding.
func (read SelectedRead[K, V]) Valid() bool { return read.binding != nil }

// Order is the member order this read was sealed under.
func (read SelectedRead[K, V]) Order() ruleprogram.Order { return read.order }

// Ordered reports whether coordinates are in the presentation order this
// read's sealed contract declares. It is the whole of the ReadOrder clause: the
// order is verified once, over the derived member set, rather than restated as
// positional comparisons inside a Fold.
//
// Canonical order is ascending by exact Unit and then by tag, which is the
// order every selected read has always had. ByTag ranks by tag alone, so a
// repeated tag admits no member order at all and refuses the read. The derived
// relation is an ordered member set, so this verifies the order it already
// carries instead of sorting a copy of it.
func (read SelectedRead[K, V]) Ordered(members []RouteMember) bool {
	if !read.Valid() {
		return false
	}
	for index := range members {
		coordinate := members[index].coordinate
		if coordinate.Unit == (carrier.Unit{}) || !read.binding.ValidUnit(coordinate.Unit) ||
			coordinate.Unit.Kind() != carrier.ExactUnit {
			return false
		}
		if index == 0 {
			continue
		}
		previous := members[index-1].coordinate
		switch read.order {
		case ruleprogram.OrderByTag:
			if coordinate.Tag <= previous.Tag {
				return false
			}
		default:
			if previous.Unit.Same(coordinate.Unit) {
				if coordinate.Tag <= previous.Tag {
					return false
				}
				continue
			}
			if !previous.Unit.Less(coordinate.Unit) {
				return false
			}
		}
	}
	return true
}

// Width reports whether a member count is one this read can deliver into
// caller-owned storage of the given sealed capacity.
//
// Multiplicity bounds the cells of ONE member, not the number of members: a
// selected route join declares MultiplicityOne because each member is observed
// at one exact coordinate and yields one cell, while the number of members is
// bounded by the denominator the join declared. Reading multiplicity as a
// member-count cap would refuse every routed row with more than one
// destination, which is the ordinary case.
func (read SelectedRead[K, V]) Width(count, capacity int) bool {
	return read.Valid() && count >= 0 && count <= capacity
}

// Observe materializes one row's Selection into cells. members is the bounded
// ordered member set the family's sealed relation derived for this candidate,
// each carrying the coordinate it is observed at beside the destination it
// publishes to; cells is caller-owned, seal-sized storage that must be at
// least as wide. Nothing is allocated here: one cursor is opened and closed
// per member against storage the worker already owns.
//
// A member that reports no row, or more than the one entry an exact coordinate
// has, refuses the whole read rather than delivering a short Selection.
func (read SelectedRead[K, V]) Observe(
	ticket Ticket,
	scratch *SelectedScratch[K, V],
	members []RouteMember,
	cells []operand.SelectedCell[V],
) ReadStatus {
	if !read.Valid() || scratch == nil || !ticket.Valid() {
		return ReadRefuse
	}
	if !read.Ordered(members) || !read.Width(len(members), len(cells)) {
		return ReadRefuse
	}
	work, state, within, contextOK := ticket.input(read.port)
	if !contextOK || work == nil || !within.Entails(state.Support()) {
		return ReadRefuse
	}
	if !scratch.begin(ticket, read.binding) {
		return ReadRefuse
	}
	for index, member := range members {
		value, present, region, ok := scratch.observe(work, state, within, member.coordinate.Unit)
		if !ok {
			scratch.finish()
			return ReadRefuse
		}
		delivered, deliveredPresent := read.policy.Cell(value, present)
		cells[index] = operand.SelectedCell[V]{Value: delivered, Present: deliveredPresent, Tag: member.coordinate.Tag, Region: region}
	}
	if !scratch.close() {
		return ReadRefuse
	}
	if len(members) == 0 {
		return ReadExhausted
	}
	return ReadAvailable
}

// SelectedScratch is the reusable per-worker cursor a selected read steps its
// members through. It is separate from Scratch because Scratch is sealed to one
// coordinate per invocation: a Selection observes several sealed coordinates
// inside one row, so it owns a cursor it may reopen.
type SelectedScratch[K scalar.Key, V any] struct {
	issuer  *Run
	serial  uint64
	binding *factbinding.Binding[K, V]
	cursor  factbinding.DirectObservation[K, V]
	open    bool
}

func (scratch *SelectedScratch[K, V]) begin(ticket Ticket, binding *factbinding.Binding[K, V]) bool {
	if scratch == nil || binding == nil || !ticket.Valid() || scratch.open {
		return false
	}
	scratch.issuer = ticket.issuer
	scratch.serial = ticket.serial
	scratch.binding = binding
	scratch.open = true
	return true
}

// observe steps one sealed coordinate and returns its single entry. The cursor
// is closed before the next coordinate opens, so one member's observation never
// outlives its own step.
func (scratch *SelectedScratch[K, V]) observe(
	work *carrier.Work,
	state carrier.State,
	within support.Mask,
	unit carrier.Unit,
) (V, bool, support.Mask, bool) {
	var zero V
	if scratch == nil || !scratch.open || scratch.binding == nil {
		return zero, false, support.Mask{}, false
	}
	if !scratch.binding.BeginDirectObservation(&scratch.cursor, work, state, unit, within) {
		return zero, false, support.Mask{}, false
	}
	row, view, status := scratch.cursor.Step()
	if status != factbinding.DirectObservationAvailable || view.Count() != 1 || !row.Region().Valid() {
		_ = scratch.cursor.Close()
		return zero, false, support.Mask{}, false
	}
	entry, entryOK := view.At(0)
	if !entryOK {
		_ = scratch.cursor.Close()
		return zero, false, support.Mask{}, false
	}
	value, present := entry.Read()
	region := row.Region()
	if !scratch.cursor.Close() {
		return zero, false, support.Mask{}, false
	}
	return value, present, region, true
}

func (scratch *SelectedScratch[K, V]) close() bool {
	if scratch == nil || !scratch.open {
		return false
	}
	scratch.finish()
	return true
}

func (scratch *SelectedScratch[K, V]) finish() {
	if scratch == nil {
		return
	}
	scratch.issuer = nil
	scratch.serial = 0
	scratch.binding = nil
	scratch.cursor = factbinding.DirectObservation[K, V]{}
	scratch.open = false
}
