package relbindgen

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// Inputs is the decoded-side view of one already-validated invocation frame.
//
// It exposes the sealed signature's own ordered slots and nothing else. There
// is no method here, and no field reachable from here, through which a
// relation, key, scope, witness, buffer, or engine structure outside the frame
// can be named. A generated decoder that tried to reach outside its frame
// would have to name an identifier this type does not provide, so the reach
// does not compile.
type Inputs struct {
	frame binding.Frame
}

// Len returns the number of declared input slots.
func (inputs Inputs) Len() int { return inputs.frame.Len() }

// PresenceAt returns the logical presence of a scalar slot's cell.
func (inputs Inputs) PresenceAt(slot int) (model.Presence, bool) {
	cell, ok := inputs.cell(slot)
	if !ok {
		return model.Presence{}, false
	}
	return cell.Presence(), true
}

// RowKeyAt returns the owner-issued row content addressed by a scalar slot.
// It is how a generated binding states that a judgment publishes at the row it
// read, without naming a relation or minting an identity.
func (inputs Inputs) RowKeyAt(slot int) (identity.ContentID, bool) {
	cell, ok := inputs.cell(slot)
	if !ok {
		return identity.ContentID{}, false
	}
	row := cell.Address().Row()
	if !row.Available() {
		return identity.ContentID{}, false
	}
	return row.Content(), true
}

// LenAt returns the delivered length of a span slot.
func (inputs Inputs) LenAt(slot int) (int, bool) {
	view, ok := inputs.frame.At(slot)
	if !ok || !view.IsSpan() {
		return 0, false
	}
	return view.Len(), true
}

func (inputs Inputs) cell(slot int) (binding.Cell, bool) {
	view, ok := inputs.frame.At(slot)
	if !ok || !view.IsScalar() {
		return binding.Cell{}, false
	}
	return view.At(0)
}

// ScalarAt decodes one scalar slot through its owner column. An absent or
// unproven cell refuses; a decoder that admits absence asks PresenceAt first.
func ScalarAt[T any](inputs Inputs, slot int, column *Column[T]) (T, bool) {
	var zero T
	cell, ok := inputs.cell(slot)
	if !ok || cell.Type() != column.Type() {
		return zero, false
	}
	return column.Decode(cell.Value())
}

// Span is a borrowed view of one delivered span slot. Values are decoded on
// access straight out of the frame, so a span costs no scratch, no copy, and
// no per-worker state: the decoder that hands one to an owner fold stays
// stateless and safe to share across solve-local workers.
//
// Presence travels beside the value exactly as owner folds expect. An absent
// coordinate is reported absent; it is never a stored domain default.
type Span[T any] struct {
	slot   binding.Slot
	column *Column[T]
}

// Len returns the number of delivered rows.
func (span Span[T]) Len() int {
	if span.column == nil {
		return 0
	}
	return span.slot.Len()
}

// At borrows one delivered row as (value, present, ok).
func (span Span[T]) At(index int) (T, bool, bool) {
	var zero T
	if span.column == nil {
		return zero, false, false
	}
	cell, ok := span.slot.At(index)
	if !ok || cell.Type() != span.column.Type() {
		return zero, false, false
	}
	presence := cell.Presence()
	if !presence.Is(model.Present) && !presence.Is(model.AuthenticatedOpaque) {
		return zero, false, true
	}
	value, decoded := span.column.Decode(cell.Value())
	if !decoded {
		return zero, false, false
	}
	return value, true, true
}

// RowKeyAt returns the owner-issued row content one delivered row is addressed
// by.
//
// A span is delivered in the mounted order of its declared key, and an owner
// fold that looks its rows up by its own identity needs to say which row each
// position carries. This is how it says so: the identity comes from the cell's
// own address, so a binding answers such a lookup from what it was delivered
// and never by minting a key or reading a relation to find one.
func (span Span[T]) RowKeyAt(index int) (identity.ContentID, bool) {
	if span.column == nil {
		return identity.ContentID{}, false
	}
	cell, ok := span.slot.At(index)
	if !ok {
		return identity.ContentID{}, false
	}
	row := cell.Address().Row()
	if !row.Available() {
		return identity.ContentID{}, false
	}
	return row.Content(), true
}

// SpanAt borrows one span slot through its owner column.
func SpanAt[T any](inputs Inputs, slot int, column *Column[T]) (Span[T], bool) {
	if !column.Available() {
		return Span[T]{}, false
	}
	view, ok := inputs.frame.At(slot)
	if !ok || !view.IsSpan() {
		return Span[T]{}, false
	}
	return Span[T]{slot: view, column: column}, true
}

// SpanAtFrame borrows one span slot of an already-validated frame. It is how a
// law reaches a delivered span without a binding around it; a decoder reaches
// the same span through its Inputs.
func SpanAtFrame[T any](frame binding.Frame, slot int, column *Column[T]) (Span[T], bool) {
	return SpanAt(Inputs{frame: frame}, slot, column)
}
