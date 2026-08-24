package execution

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/factbinding"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/scalar"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
)

// ReadStatus is the cursor-level result of one direct read step. It is kept
// separate from the fold outcome: sparse absence is still an available row,
// while NoCandidate is a rule-level reducer decision made by generated code.
type ReadStatus uint8

const (
	ReadRefuse ReadStatus = iota
	ReadAvailable
	ReadExhausted
)

// Valid reports whether status is one of the three cursor dispositions.
func (status ReadStatus) Valid() bool { return status <= ReadExhausted }

// ExactRead is one immutable typed exact-read descriptor. It is sealed with a
// rule family and contains no epoch, Run, callback, or member state. A worker
// authenticates it against the opaque Ticket at execution time.
type ExactRead[K scalar.Key, V any] struct {
	binding *factbinding.Binding[K, V]
	unit    carrier.Unit
	port    uint16
}

// NewExactRead seals one exact Unit and input port for a typed family.
func NewExactRead[K scalar.Key, V any](binding *factbinding.Binding[K, V], unit carrier.Unit, port uint16) (ExactRead[K, V], bool) {
	if binding == nil || !binding.ValidUnit(unit) || unit.Kind() != carrier.ExactUnit {
		return ExactRead[K, V]{}, false
	}
	return ExactRead[K, V]{binding: binding, unit: unit, port: port}, true
}

// Valid proves that the read surface still names a live declared exact Unit.
func (axis ExactRead[K, V]) Valid() bool {
	return axis.binding != nil && axis.binding.ValidUnit(axis.unit) && axis.unit.Kind() == carrier.ExactUnit
}

func (axis ExactRead[K, V]) context(ticket Ticket) (*carrier.Work, carrier.State, support.Mask, bool) {
	if !axis.Valid() || !ticket.Valid() {
		return nil, carrier.State{}, support.Mask{}, false
	}
	return ticket.input(axis.port)
}

// Read advances the callback-free exact cursor by one canonical row. A
// successful step always reports ReadAvailable, even when Scratch.Present is
// false for sparse absence. The caller decides whether that row is a selected
// candidate or a default.
func (axis ExactRead[K, V]) Read(ticket Ticket, scratch *Scratch[K, V]) ReadStatus {
	work, state, within, ok := axis.context(ticket)
	if !ok {
		return ReadRefuse
	}
	return axis.readWithin(ticket, scratch, work, state, within)
}

// ReadWithin advances the same canonical exact cursor over one authenticated
// Product source region. The region must be a subset of the Ticket input
// support; the cursor, partition order, and sparse presence semantics remain
// owned by factbinding rather than by a family-specific refinement loop.
func (axis ExactRead[K, V]) ReadWithin(ticket Ticket, scratch *Scratch[K, V], within support.Mask) ReadStatus {
	work, state, ticketWithin, ok := axis.context(ticket)
	if !ok || !within.Valid() || within.Manager() != ticketWithin.Manager() || !within.Entails(ticketWithin) {
		return ReadRefuse
	}
	return axis.readWithin(ticket, scratch, work, state, within)
}

func (axis ExactRead[K, V]) readWithin(ticket Ticket, scratch *Scratch[K, V], work *carrier.Work, state carrier.State, within support.Mask) ReadStatus {
	if scratch == nil || !within.Entails(state.Support()) {
		return ReadRefuse
	}
	if scratch.readOpen {
		sameWithin := scratch.readWithin.SameHandle(within) || scratch.readWithin.Equal(within)
		if scratch.readSummary || !scratch.validFor(ticket) || scratch.readBinding != axis.binding || !scratch.readUnit.Same(axis.unit) || scratch.readPort != axis.port || !sameWithin {
			return ReadRefuse
		}
	} else {
		if !scratch.reset(ticket) {
			return ReadRefuse
		}
		scratch.readBinding = axis.binding
		scratch.readUnit = axis.unit
		scratch.readPort = axis.port
		scratch.readWithin = within
		scratch.readSummary = false
		if !axis.binding.BeginDirectObservation(&scratch.cursor, work, state, axis.unit, within) {
			scratch.finish()
			return ReadRefuse
		}
		scratch.readOpen = true
	}

	var zero V
	scratch.value = zero
	scratch.present = false
	scratch.row = carrier.ObservationRow{}
	scratch.current = false
	row, view, status := scratch.cursor.Step()
	if status == factbinding.DirectObservationExhausted {
		return ReadExhausted
	}
	if status != factbinding.DirectObservationAvailable {
		return ReadRefuse
	}
	if view.Count() != 1 {
		return ReadRefuse
	}
	entry, entryOK := view.At(0)
	if !entryOK || !row.Region().Valid() {
		return ReadRefuse
	}
	value, present := entry.Read()
	scratch.row = row
	scratch.view = view
	scratch.value = value
	scratch.present = present
	scratch.current = true
	return ReadAvailable
}

// Close closes only this read cursor. It leaves Ticket open so independent
// ExactWrite instances can stage against the same invocation.
func (axis ExactRead[K, V]) Close(ticket Ticket, scratch *Scratch[K, V]) bool {
	if !axis.Valid() || scratch == nil || !scratch.validFor(ticket) || !scratch.readOpen || scratch.readSummary || scratch.readBinding != axis.binding || !scratch.readUnit.Same(axis.unit) || scratch.readPort != axis.port {
		return false
	}
	closed := scratch.cursor.Close()
	scratch.readOpen = false
	if closed {
		var zero V
		scratch.value = zero
		scratch.present = false
		scratch.row = carrier.ObservationRow{}
		scratch.view = factbinding.Observation[V]{}
		scratch.current = false
	}
	return closed
}

// SummaryExactRead is the immutable typed summary-read surface. It is kept
// separate from ExactRead so exact callers retain their one-entry hot path and
// never pay summary-form branching. The sealed Unit kind is the only form
// authority; no output axis or domain callback is involved.
type SummaryRead[K scalar.Key, V any] struct {
	binding *factbinding.Binding[K, V]
	unit    carrier.Unit
	port    uint16
}

// NewSummaryExactRead binds one sealed SummaryUnit to one dense input port.
// Carrier capabilities remain engine-internal, so external code can consume
// a SummaryExactRead but cannot mint one.
func NewSummaryRead[K scalar.Key, V any](binding *factbinding.Binding[K, V], unit carrier.Unit, port uint16) (SummaryRead[K, V], bool) {
	if binding == nil || !binding.ValidUnit(unit) || unit.Kind() != carrier.SummaryUnit {
		return SummaryRead[K, V]{}, false
	}
	return SummaryRead[K, V]{binding: binding, unit: unit, port: port}, true
}

// Valid proves that the read surface still names a live declared SummaryUnit.
func (axis SummaryRead[K, V]) Valid() bool {
	return axis.binding != nil && axis.binding.ValidUnit(axis.unit) && axis.unit.Kind() == carrier.SummaryUnit
}

func (axis SummaryRead[K, V]) context(ticket Ticket) (*carrier.Work, carrier.State, support.Mask, bool) {
	if !axis.Valid() || !ticket.Valid() {
		return nil, carrier.State{}, support.Mask{}, false
	}
	return ticket.input(axis.port)
}

// Read advances the callback-free summary cursor by one canonical row. Every
// emitted row is available even when one or more declared entries are sparse
// absent; the Observation view preserves each value/presence pair in sealed
// declaration order.
func (axis SummaryRead[K, V]) Read(ticket Ticket, scratch *Scratch[K, V]) ReadStatus {
	work, state, within, ok := axis.context(ticket)
	if !ok || scratch == nil || !within.Entails(state.Support()) {
		return ReadRefuse
	}
	if scratch.readOpen {
		if !scratch.readSummary || !scratch.validFor(ticket) || scratch.readBinding != axis.binding || !scratch.readUnit.Same(axis.unit) || scratch.readPort != axis.port {
			return ReadRefuse
		}
	} else {
		if !scratch.reset(ticket) {
			return ReadRefuse
		}
		scratch.readBinding = axis.binding
		scratch.readUnit = axis.unit
		scratch.readPort = axis.port
		scratch.readSummary = true
		if !axis.binding.BeginDirectObservation(&scratch.cursor, work, state, axis.unit, within) {
			scratch.finish()
			return ReadRefuse
		}
		scratch.readOpen = true
	}

	var zero V
	scratch.value = zero
	scratch.present = false
	scratch.row = carrier.ObservationRow{}
	scratch.view = factbinding.Observation[V]{}
	scratch.current = false
	row, view, status := scratch.cursor.Step()
	switch status {
	case factbinding.DirectObservationExhausted:
		return ReadExhausted
	case factbinding.DirectObservationAvailable:
		if !view.Valid() || !row.Region().Valid() {
			return ReadRefuse
		}
	default:
		return ReadRefuse
	}
	scratch.row = row
	scratch.view = view
	scratch.current = true
	return ReadAvailable
}

// Close closes only this summary-read cursor. It leaves Ticket open so an
// independent ExactWrite can stage against the same invocation.
func (axis SummaryRead[K, V]) Close(ticket Ticket, scratch *Scratch[K, V]) bool {
	if !axis.Valid() || scratch == nil || !scratch.validFor(ticket) || !scratch.readOpen || !scratch.readSummary || scratch.readBinding != axis.binding || !scratch.readUnit.Same(axis.unit) || scratch.readPort != axis.port {
		return false
	}
	closed := scratch.cursor.Close()
	scratch.readOpen = false
	if closed {
		var zero V
		scratch.value = zero
		scratch.present = false
		scratch.row = carrier.ObservationRow{}
		scratch.view = factbinding.Observation[V]{}
		scratch.current = false
	}
	return closed
}

// ExactWrite is one immutable typed authored-write descriptor. It is independent
// of reads and names the invocation's base predecessor, so zero-read/
// bootstrap and heterogeneous read/write rules use the same primitive.
type ExactWrite[K scalar.Key, V any] struct {
	binding *factbinding.Binding[K, V]
	target  carrier.Target
	output  uint16
}

// NewExactWrite binds one declared Target to one fixed output slot in Run. The
// target's physical slot need not equal any ExactRead slot.
func NewExactWrite[K scalar.Key, V any](binding *factbinding.Binding[K, V], target carrier.Target, output uint16) (ExactWrite[K, V], bool) {
	if binding == nil || !binding.ValidTarget(target) {
		return ExactWrite[K, V]{}, false
	}
	return ExactWrite[K, V]{binding: binding, target: target, output: output}, true
}

// BindInvocationExactRead authenticates a typed exact axis against the sealed
// input-handle ordinal of a Run. The caller supplies the already bound typed
// Factor operation and Unit; this function performs no key, topology, or
// schema reconstruction.
func BindInvocationExactRead[K scalar.Key, V any](readIndex int, binding *factbinding.Binding[K, V], unit carrier.Unit) (ExactRead[K, V], bool) {
	if readIndex < 0 || readIndex > int(^uint16(0)) {
		return ExactRead[K, V]{}, false
	}
	return NewExactRead(binding, unit, uint16(readIndex))
}

// BindInvocationSummaryAxis is the summary-form counterpart of
// BindInvocationExactRead. The sealed Unit kind selects summary semantics.
func BindInvocationSummaryRead[K scalar.Key, V any](readIndex int, binding *factbinding.Binding[K, V], unit carrier.Unit) (SummaryRead[K, V], bool) {
	if readIndex < 0 || readIndex > int(^uint16(0)) {
		return SummaryRead[K, V]{}, false
	}
	return NewSummaryRead(binding, unit, uint16(readIndex))
}

// BindInvocationExactWrite authenticates a typed output axis against the
// sealed output-handle ordinal of a Run.
func BindInvocationExactWrite[K scalar.Key, V any](outputIndex int, binding *factbinding.Binding[K, V], target carrier.Target) (ExactWrite[K, V], bool) {
	if outputIndex < 0 || outputIndex > int(^uint16(0)) {
		return ExactWrite[K, V]{}, false
	}
	return NewExactWrite(binding, target, uint16(outputIndex))
}

// Valid proves that the write surface still names a live declared Target and
// an allocated Run output slot.
func (axis ExactWrite[K, V]) Valid() bool {
	return axis.binding != nil && axis.binding.ValidTarget(axis.target)
}

func (axis ExactWrite[K, V]) context(ticket Ticket) (*carrier.Work, carrier.State, support.Mask, bool) {
	if !axis.Valid() || !ticket.Valid() {
		return nil, carrier.State{}, support.Mask{}, false
	}
	return ticket.base()
}

func (axis ExactWrite[K, V]) scratch(ticket Ticket, scratch *Scratch[K, V]) bool {
	if scratch == nil {
		return false
	}
	if scratch.issuer == nil {
		if !scratch.reset(ticket) {
			return false
		}
	} else if !scratch.validFor(ticket) {
		return false
	}
	if scratch.writeBinding != nil && (scratch.writeBinding != axis.binding || !scratch.writeTarget.Same(axis.target)) {
		return false
	}
	scratch.writeBinding = axis.binding
	scratch.writeTarget = axis.target
	return true
}

// begin opens this invocation's one write transaction over the lane's own
// reusable storage, or returns the one already open. Every write of one
// invocation - the row and any transformed carry - shares it, so the row
// publishes atomically.
func (axis ExactWrite[K, V]) begin(ticket Ticket, scratch *Scratch[K, V]) (*factbinding.Patch[K, V], bool) {
	work, state, _, ok := axis.context(ticket)
	if !ok || int(axis.output) >= ticket.OutputCount() || !axis.scratch(ticket, scratch) {
		return nil, false
	}
	if scratch.patch == nil {
		scratch.patch = axis.binding.BeginInto(&scratch.patchScratch, work, state)
	}
	return scratch.patch, scratch.patch != nil
}

// Stage writes one typed value at an explicitly authenticated support region.
// It does not require a prior ExactRead step. The region must be admitted by
// both the base predecessor and this invocation's within region.
func (axis ExactWrite[K, V]) Stage(ticket Ticket, scratch *Scratch[K, V], when support.Mask, value V) bool {
	_, state, within, ok := axis.context(ticket)
	if !ok || !when.Valid() || support.Empty(when) || when.Manager() != state.Support().Manager() || !when.Entails(state.Support()) || !when.Entails(within) {
		return false
	}
	patch, patchOK := axis.begin(ticket, scratch)
	if !patchOK {
		return false
	}
	return patch.Write(axis.target, when, value)
}

// Close seals this independent output into the Run-owned output slot. It does
// not consume Ticket; Run.Submit performs the one final outcome decision and
// atomically closes the invocation after every output is sealed.
func (axis ExactWrite[K, V]) Close(ticket Ticket, scratch *Scratch[K, V]) bool {
	if !axis.Valid() || scratch == nil {
		return false
	}
	run := ticket.issuer
	if !scratch.validFor(ticket) || run == nil || int(axis.output) >= run.outputCount || scratch.writeBinding != axis.binding || !scratch.writeTarget.Same(axis.target) || run.used[axis.output] {
		return false
	}
	work, _, _, ok := axis.context(ticket)
	if !ok {
		return false
	}
	if scratch.readOpen {
		if !scratch.cursor.Close() {
			if scratch.patch != nil {
				_ = scratch.patch.Discard()
			}
			scratch.finish()
			return false
		}
		scratch.readOpen = false
	}
	if scratch.patch == nil {
		return false
	}
	patch, accepted := scratch.patch.Accept(work)
	if !accepted {
		scratch.finish()
		return false
	}
	run.outputs[axis.output] = patch
	run.used[axis.output] = true
	scratch.finish()
	return true
}
