package index

import (
	"github.com/wippyai/go-lua/analysis/engine"
	calldomain "github.com/wippyai/go-lua/domain/call"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/pack"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// Selected is one delivered selection slot: the value a frame's selection
// answered for one tag, and the three dispositions that answer distinguishes.
//
// A slot is refused when the selection could not be read at all, missing when
// the selection was read and holds no row for the tag, and found otherwise. A
// found slot is present when the row carries a value and absent when the row
// proves there is none. The three stay distinct because a raw access answers
// differently for each, and a binding that collapsed them would answer for a
// reading its owner did not make.
type Selected[V any] struct {
	value   V
	present bool
	found   bool
	valid   bool
}

// NewSelected delivers one row the selection found, present or proven absent.
func NewSelected[V any](value V, present bool) Selected[V] {
	return Selected[V]{value: value, present: present, found: true, valid: true}
}

// NewMissingSelected delivers a selection that was read and holds no row for
// the tag.
func NewMissingSelected[V any]() Selected[V] { return Selected[V]{valid: true} }

// NewRefusedSelected delivers a selection that could not be read.
func NewRefusedSelected[V any]() Selected[V] { return Selected[V]{} }

// Value returns the delivered value. It carries meaning only for a found and
// present slot.
func (selected Selected[V]) Value() V { return selected.value }

// Present reports whether the found row carries a value.
func (selected Selected[V]) Present() bool { return selected.present }

// Found reports whether the selection holds a row for the tag.
func (selected Selected[V]) Found() bool { return selected.found }

// Valid reports whether the selection was read at all.
func (selected Selected[V]) Valid() bool { return selected.valid }

func (rule *RawGetRule) fold(frame engine.Frame[valuedomain.Value, Index]) engine.RuleResult[valuedomain.Value] {
	operand, ok := engine.Operand(frame)
	if !ok || !rule.owns(operand) {
		return engine.RuleResult[valuedomain.Value]{}
	}
	receiverCells, receiverOK := engine.ReadValue(frame, rule.receiver)
	keys, keyOK := engine.ReadValue(frame, rule.key)
	calls, callOK := engine.ReadValue(frame, rule.call)
	heaps, heapOK := engine.ReadValue(frame, rule.heapRead)
	packs, packOK := engine.ReadValue(frame, rule.packRead)
	sources, sourceOK := engine.ReadValue(frame, rule.sourceRead)
	if !receiverOK || !keyOK || !callOK || !heapOK || !packOK || !sourceOK || receiverCells.Count() != 1 {
		return engine.RuleResult[valuedomain.Value]{}
	}
	receiver, receiverPresent, receiverAvailable := receiverCells.At(0)
	if !receiverAvailable {
		return engine.RuleResult[valuedomain.Value]{}
	}
	if !receiverPresent {
		if !transferSelectionsEmpty(frame, keys, calls, heaps, packs, sources) {
			return engine.RuleResult[valuedomain.Value]{}
		}
		return engine.NoCandidate(frame)
	}
	scratch := rule.takeScratch()
	defer rule.putScratch(scratch)
	view, ok := transferRawGetView(frame, operand, keys, calls, heaps, packs, sources, scratch)
	if !ok {
		return engine.RuleResult[valuedomain.Value]{}
	}
	result, contributed, valid := rule.reduce(operand, receiver, view)
	if !valid {
		return engine.RuleResult[valuedomain.Value]{}
	}
	if !contributed {
		return engine.NoCandidate(frame)
	}
	return engine.Staged(frame, result)
}

func transferSelectionsEmpty(
	frame engine.Frame[valuedomain.Value, Index],
	keys engine.Selection[uint64, engine.OrderedCells[valuedomain.Value]],
	calls engine.Selection[uint64, engine.OrderedCells[calldomain.Value]],
	heaps engine.Selection[heapdomain.RawRouteTag, engine.OrderedCells[heapdomain.Value]],
	packs engine.Selection[heapdomain.RawPayloadTag, engine.OrderedCells[pack.Value]],
	sources engine.Selection[RawSourceTag, engine.OrderedCells[valuedomain.Value]],
) bool {
	keyCount, keyOK := engine.SelectionCount(frame, keys)
	callCount, callOK := engine.SelectionCount(frame, calls)
	heapCount, heapOK := engine.SelectionCount(frame, heaps)
	packCount, packOK := engine.SelectionCount(frame, packs)
	sourceCount, sourceOK := engine.SelectionCount(frame, sources)
	return keyOK && callOK && heapOK && packOK && sourceOK && keyCount == 0 && callCount == 0 && heapCount == 0 && packCount == 0 && sourceCount == 0
}

func transferRawGetView(
	frame engine.Frame[valuedomain.Value, Index], operand Index,
	keys engine.Selection[uint64, engine.OrderedCells[valuedomain.Value]],
	calls engine.Selection[uint64, engine.OrderedCells[calldomain.Value]],
	heaps engine.Selection[heapdomain.RawRouteTag, engine.OrderedCells[heapdomain.Value]],
	packs engine.Selection[heapdomain.RawPayloadTag, engine.OrderedCells[pack.Value]],
	sources engine.Selection[RawSourceTag, engine.OrderedCells[valuedomain.Value]],
	scratch *RawGetScratch,
) (RawGetFrame, bool) {
	if scratch == nil {
		return RawGetFrame{}, false
	}
	view := RawGetFrame{Scratch: scratch}
	var ok bool
	view.KeyCount, ok = engine.SelectionCount(frame, keys)
	if !ok {
		return RawGetFrame{}, false
	}
	view.CallCount, ok = engine.SelectionCount(frame, calls)
	if !ok {
		return RawGetFrame{}, false
	}
	view.HeapCount, ok = engine.SelectionCount(frame, heaps)
	if !ok {
		return RawGetFrame{}, false
	}
	view.PackCount, ok = engine.SelectionCount(frame, packs)
	if !ok {
		return RawGetFrame{}, false
	}
	view.SourceCount, ok = engine.SelectionCount(frame, sources)
	if !ok {
		return RawGetFrame{}, false
	}
	if !buildTransferIndex(frame, calls, view.CallCount, &scratch.call) ||
		!buildTransferIndex(frame, heaps, view.HeapCount, &scratch.heap) ||
		!buildTransferIndex(frame, packs, view.PackCount, &scratch.pack) ||
		!buildTransferIndex(frame, sources, view.SourceCount, &scratch.value) {
		return RawGetFrame{}, false
	}
	if _, dynamic := operand.DynamicKey(); dynamic {
		view.Key = transferSelectionValue(frame, keys, nil, uint64(1))
		if !view.Key.valid || !view.Key.found {
			return RawGetFrame{}, false
		}
	}
	view.Call = func(tag uint64) Selected[calldomain.Value] {
		return transferSelectionValue(frame, calls, &scratch.call, tag)
	}
	view.Heap = func(tag heapdomain.RawRouteTag, _ heapdomain.Key) Selected[heapdomain.Value] {
		return transferSelectionValue(frame, heaps, &scratch.heap, tag)
	}
	view.Pack = func(tag heapdomain.RawPayloadTag) Selected[pack.Value] {
		return transferSelectionValue(frame, packs, &scratch.pack, tag)
	}
	view.Source = func(tag RawSourceTag) Selected[valuedomain.Value] {
		return transferSelectionValue(frame, sources, &scratch.value, tag)
	}
	return view, true
}

func transferSelectionValue[Out any, O any, S any, Tag interface {
	~uint8 | ~uint16 | ~uint32 | ~uint64
}](
	frame engine.Frame[Out, O], selection engine.Selection[Tag, engine.OrderedCells[S]], index *rawSelectionIndex, want Tag,
) Selected[S] {
	ordinal := 0
	if index == nil {
		count, ok := engine.SelectionCount(frame, selection)
		if !ok || count != 1 {
			return Selected[S]{}
		}
	} else {
		found, ok := index.ordinal(uint64(want))
		if !ok {
			return Selected[S]{valid: true}
		}
		ordinal = found
	}
	tag, cells, selected := engine.SelectionAt(frame, selection, ordinal)
	if !selected || tag != want || cells.Count() != 1 {
		return Selected[S]{}
	}
	value, present, available := cells.At(0)
	if !available {
		return Selected[S]{}
	}
	return Selected[S]{value: value, present: present, found: true, valid: true}
}

func buildTransferIndex[Out any, O any, S any, Tag interface {
	~uint8 | ~uint16 | ~uint32 | ~uint64
}](frame engine.Frame[Out, O], selection engine.Selection[Tag, engine.OrderedCells[S]], count int, index *rawSelectionIndex) bool {
	return index.build(count, func(ordinal int) (uint64, bool) {
		tag, cells, selected := engine.SelectionAt(frame, selection, ordinal)
		if !selected || cells.Count() != 1 {
			return 0, false
		}
		_, _, available := cells.At(0)
		return uint64(tag), available
	})
}

func (rule *RawGetRule) reduce(operand Index, receiver valuedomain.Value, view RawGetFrame) (valuedomain.Value, bool, bool) {
	if rule == nil || !rule.owns(operand) {
		return valuedomain.Value{}, false, false
	}
	return rule.runtime.topology.RawGetReduce(operand, receiver, view)
}
