package binding

import "github.com/wippyai/go-lua/analysis/relation/semantic/signature"

// Frame is an immutable ordered sequence of scalar or span input views under
// one already-conjoined invocation scope.
type Frame struct {
	scope ScopeToken
	slots []Slot
}

func NewFrame(scope ScopeToken, slots ...Slot) (Frame, bool) {
	if !scope.Available() {
		return Frame{}, false
	}
	for _, slot := range slots {
		if !slot.Available() {
			return Frame{}, false
		}
	}
	if slots == nil {
		slots = emptySlots
	}
	return Frame{scope: scope, slots: slots}, true
}

var emptySlots = []Slot{}

func EmptyFrame(scope ScopeToken) Frame {
	if !scope.Available() {
		return Frame{}
	}
	return Frame{scope: scope, slots: emptySlots}
}
func (frame Frame) Available() bool   { return frame.scope.Available() && frame.slots != nil }
func (frame Frame) Scope() ScopeToken { return frame.scope }
func (frame Frame) Len() int          { return len(frame.slots) }

func (frame Frame) At(index int) (Slot, bool) {
	if !frame.Available() || index < 0 || index >= len(frame.slots) {
		return Slot{}, false
	}
	return frame.slots[index], true
}

// Validate checks logical signature order and the solve-local runtime fence.
// Each address authenticates its own denominator while every cell is required
// to carry the frame's already-conjoined invocation scope.
func (frame Frame) Validate(operation signature.Signature, fence Fence) bool {
	if !frame.Available() || !operation.Available() || !fence.Available() || !frame.scope.ValidFor(fence) || fence.Schema() != operation.Fence().Schema || len(frame.slots) != operation.InputLen() {
		return false
	}
	for index, slot := range frame.slots {
		input, ok := operation.InputAt(index)
		if !ok || !input.Delivery.Available() || !input.Denominator.Available() || !slot.Available() {
			return false
		}
		delivery := input.Delivery
		if delivery.IsScalar() {
			if slot.kind != scalarSlot {
				return false
			}
		} else if delivery.IsSpan() {
			if slot.kind != spanSlot {
				return false
			}
			if limit, bounded := delivery.Limit(); bounded && uint64(slot.Len()) > uint64(limit) {
				return false
			}
		} else {
			return false
		}
		var previousOrder int
		if delivery.IsSpan() {
			previousOrder = -1
			rangeWitness := slot.Witness()
			if !rangeWitness.ValidFor(fence) || !rangeWitness.Matches(input.Denominator) || rangeWitness.Key() != delivery.OrderKey() {
				return false
			}
		}
		for cellIndex := 0; cellIndex < slot.Len(); cellIndex++ {
			cell, cellOK := slot.At(cellIndex)
			if !cellOK || !cell.Available() || !cell.Address().ValidFor(fence) || !cell.Address().Scope().Same(frame.scope) || !cell.Address().Witness().Matches(input.Denominator) || cell.Address().Relation() != input.Relation || cell.Address().Column() != input.Column || cell.Type() != input.Type || (cell.Value().Available() && (!cell.Value().ValidFor(fence) || cell.Value().Type() != input.Type)) || !input.Presence.Allows(cell.Presence()) {
				return false
			}
			if delivery.IsSpan() {
				witness := cell.Address().Witness()
				if !witness.Same(slot.Witness()) {
					return false
				}
				order, orderOK := witness.membership.Index(cell.Address().Row())
				if !orderOK || order <= previousOrder {
					return false
				}
				previousOrder = order
				if delivery.IsComplete() && order != cellIndex {
					return false
				}
			}
		}
		if delivery.IsComplete() && (slot.Len() != slot.Witness().membership.Len() || (slot.Len() > 0 && previousOrder != slot.Len()-1)) {
			return false
		}
	}
	return true
}
