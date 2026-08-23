package execution

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/factbinding"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/scalar"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
)

// Scratch is typed, caller-owned reusable storage for one invocation lane.
// Its cursor and candidate are concrete engine-private values: there are no
// interface, callback, map, or fallback-growth fields.
type Scratch[K scalar.Key, V any] struct {
	issuer     *Run
	serial     uint64
	epoch      uint64
	generation uint64
	closed     bool

	value   V
	present bool
	row     carrier.ObservationRow
	view    factbinding.Observation[V]
	current bool

	readBinding *factbinding.Binding[K, V]
	readUnit    carrier.Unit
	readPort    uint16
	readOpen    bool
	readSummary bool
	cursor      factbinding.DirectObservation[K, V]

	writeBinding *factbinding.Binding[K, V]
	writeTarget  carrier.Target
	patch        *factbinding.Patch[K, V]
}

func (scratch *Scratch[K, V]) reset(ticket Ticket) bool {
	if scratch == nil || !ticket.Valid() || scratch.issuer != nil && scratch.issuer.open == scratch.serial {
		return false
	}
	scratch.issuer = ticket.issuer
	scratch.serial = ticket.serial
	scratch.epoch = ticket.epoch
	scratch.generation = ticket.generation
	scratch.closed = false
	var zero V
	scratch.value = zero
	scratch.present = false
	scratch.row = carrier.ObservationRow{}
	scratch.view = factbinding.Observation[V]{}
	scratch.current = false
	scratch.readBinding = nil
	scratch.readUnit = carrier.Unit{}
	scratch.readPort = 0
	scratch.readOpen = false
	scratch.readSummary = false
	scratch.cursor = factbinding.DirectObservation[K, V]{}
	scratch.writeBinding = nil
	scratch.writeTarget = carrier.Target{}
	scratch.patch = nil
	return true
}

func (scratch *Scratch[K, V]) validFor(ticket Ticket) bool {
	return scratch != nil && !scratch.closed && ticket.Valid() && scratch.issuer == ticket.issuer && scratch.serial == ticket.serial && scratch.epoch == ticket.epoch && scratch.generation == ticket.generation
}

func (scratch *Scratch[K, V]) finish() {
	if scratch == nil {
		return
	}
	var zero V
	scratch.value = zero
	scratch.present = false
	scratch.row = carrier.ObservationRow{}
	scratch.view = factbinding.Observation[V]{}
	scratch.current = false
	scratch.readBinding = nil
	scratch.readUnit = carrier.Unit{}
	scratch.readPort = 0
	scratch.readOpen = false
	scratch.readSummary = false
	scratch.cursor = factbinding.DirectObservation[K, V]{}
	scratch.writeBinding = nil
	scratch.writeTarget = carrier.Target{}
	scratch.patch = nil
	scratch.issuer = nil
	scratch.serial = 0
	scratch.epoch = 0
	scratch.generation = 0
	scratch.closed = true
}

// Value returns the typed value selected by the latest available read row.
// The boolean reports row availability; Present distinguishes sparse absence.
func (scratch *Scratch[K, V]) Value() (V, bool) {
	if scratch == nil || scratch.closed || !scratch.current {
		var zero V
		return zero, false
	}
	return scratch.value, true
}

// Present reports whether the current available row contains a stored value.
func (scratch *Scratch[K, V]) Present() bool {
	return scratch != nil && !scratch.closed && scratch.current && scratch.present
}

// Observation returns the canonical generation-bound view selected by the
// latest available read row. Exact reads expose a one-entry view; summary
// reads expose their sealed declared sequence. The view refuses after the
// read cursor or its Ticket is closed.
func (scratch *Scratch[K, V]) Observation() (factbinding.Observation[V], bool) {
	if scratch == nil || scratch.closed || !scratch.current || !scratch.view.Valid() {
		return factbinding.Observation[V]{}, false
	}
	return scratch.view, true
}

// View is the short alias for Observation used by cursor-style consumers.
func (scratch *Scratch[K, V]) View() (factbinding.Observation[V], bool) {
	return scratch.Observation()
}

// Discard closes the current read cursor and drops an unaccepted write patch.
// It is the explicit fail-closed cleanup used when generated execution cannot
// complete its typed fold. The Ticket remains open so Run.Submit can perform
// the one final lifecycle transition.
func (scratch *Scratch[K, V]) Discard(ticket Ticket) bool {
	if scratch == nil || !scratch.validFor(ticket) {
		return false
	}
	ok := true
	if scratch.readOpen {
		ok = scratch.cursor.Close() && ok
		scratch.readOpen = false
	}
	if scratch.patch != nil {
		ok = scratch.patch.Discard() && ok
		scratch.patch = nil
	}
	scratch.finish()
	return ok
}

// Region returns the authenticated support row selected by the latest read.
// It returns no region until a cursor has reported an available row.
func (scratch *Scratch[K, V]) Region() (support.Mask, bool) {
	if scratch == nil || scratch.closed || !scratch.current {
		return support.Mask{}, false
	}
	region := scratch.row.Region()
	return region, region.Valid()
}
