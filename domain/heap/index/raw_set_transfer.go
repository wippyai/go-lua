package index

import (
	"github.com/wippyai/go-lua/analysis/engine"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/pack"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

func (rule *RawSetRule) fold(frame engine.Frame[heapdomain.Value, Index]) engine.RuleResult[heapdomain.Value] {
	operand, ok := engine.Operand(frame)
	if !ok || !rule.owns(operand) {
		return engine.RuleResult[heapdomain.Value]{}
	}
	receiverCells, receiverOK := engine.ReadValue(frame, rule.receiver)
	keys, keyOK := engine.ReadValue(frame, rule.key)
	heaps, heapOK := engine.ReadValue(frame, rule.heapRead)
	packs, packOK := engine.ReadValue(frame, rule.packRead)
	sources, sourceOK := engine.ReadValue(frame, rule.source)
	if !receiverOK || !keyOK || !heapOK || !packOK || !sourceOK || receiverCells.Count() != 1 {
		return engine.RuleResult[heapdomain.Value]{}
	}
	_, receiverPresent, receiverAvailable := receiverCells.At(0)
	if !receiverAvailable {
		return engine.RuleResult[heapdomain.Value]{}
	}
	descriptor, descriptorOK := rule.payloadForWrite(operand)
	if !descriptorOK {
		return engine.RuleResult[heapdomain.Value]{}
	}
	scratch := rule.takeSetScratch()
	defer rule.putSetScratch(scratch)
	view, viewOK := transferRawSetView(frame, operand, keys, heaps, packs, sources, scratch)
	if !viewOK || !rawSetSelectionShape(operand, descriptor.row, view) {
		return engine.RuleResult[heapdomain.Value]{}
	}
	// An empty Heap route is the explicit no-candidate disposition. This
	// covers absent receivers, selected-but-absent dynamic keys, and
	// definitely invalid dynamic nil/NaN keys; Pack/Value selectors must
	// be empty downstream of that route.
	if view.HeapCount == 0 {
		if _, dynamic := operand.DynamicKey(); dynamic {
			if receiverPresent && view.KeyCount != 1 || !receiverPresent && view.KeyCount != 0 {
				return engine.RuleResult[heapdomain.Value]{}
			}
			// A live receiver's no-route branch is still authenticated by
			// one exact dynamic-key selection. Its cell may be absent only
			// because every downstream selection is empty; routed Heap
			// mutation remains stricter below.
			if receiverPresent && (!view.Key.valid || !view.Key.found) {
				return engine.RuleResult[heapdomain.Value]{}
			}
		}
		return engine.NoSelection(frame, heaps)
	}
	if !receiverPresent {
		return engine.RuleResult[heapdomain.Value]{}
	}
	return engine.Routed(frame, heaps, func(tag heapdomain.RawRouteTag, cells engine.OrderedCells[heapdomain.Value]) (heapdomain.Value, bool) {
		if cells.Count() != 1 {
			return heapdomain.Value{}, false
		}
		fact, present, available := cells.At(0)
		if !available {
			return heapdomain.Value{}, false
		}
		if !present {
			return rule.heapSchema().Bottom(), true
		}
		return rule.mutateRoute(operand, tag, fact, view)
	})
}

func transferRawSetView(
	frame engine.Frame[heapdomain.Value, Index], operand Index,
	keys engine.Selection[uint64, engine.OrderedCells[valuedomain.Value]],
	heaps engine.Selection[heapdomain.RawRouteTag, engine.OrderedCells[heapdomain.Value]],
	packs engine.Selection[heapdomain.RawPayloadTag, engine.OrderedCells[pack.Value]],
	sources engine.Selection[RawSourceTag, engine.OrderedCells[valuedomain.Value]],
	scratch *rawSetScratch,
) (RawSetFrame, bool) {
	if scratch == nil {
		return RawSetFrame{}, false
	}
	view := RawSetFrame{}
	var ok bool
	view.KeyCount, ok = engine.SelectionCount(frame, keys)
	if !ok {
		return RawSetFrame{}, false
	}
	view.HeapCount, ok = engine.SelectionCount(frame, heaps)
	if !ok {
		return RawSetFrame{}, false
	}
	view.PackCount, ok = engine.SelectionCount(frame, packs)
	if !ok {
		return RawSetFrame{}, false
	}
	view.SourceCount, ok = engine.SelectionCount(frame, sources)
	if !ok {
		return RawSetFrame{}, false
	}
	if !buildTransferIndex(frame, packs, view.PackCount, &scratch.pack) ||
		!buildTransferIndex(frame, sources, view.SourceCount, &scratch.source) {
		return RawSetFrame{}, false
	}
	if _, dynamic := operand.DynamicKey(); dynamic {
		if view.KeyCount > 1 {
			return RawSetFrame{}, false
		}
		if view.KeyCount == 1 {
			view.Key = transferSelectionValue(frame, keys, nil, uint64(1))
			if !view.Key.valid || !view.Key.found {
				return RawSetFrame{}, false
			}
		}
	} else if view.KeyCount != 0 {
		return RawSetFrame{}, false
	}
	view.Pack = func(tag heapdomain.RawPayloadTag) rawSelected[pack.Value] {
		return transferSelectionValue(frame, packs, &scratch.pack, tag)
	}
	view.Source = func(tag RawSourceTag) rawSelected[valuedomain.Value] {
		return transferSelectionValue(frame, sources, &scratch.source, tag)
	}
	return view, true
}

func rawSetSelectionShape(access Index, descriptor rawPayload, view RawSetFrame) bool {
	if view.KeyCount < 0 || view.HeapCount < 0 || view.PackCount < 0 || view.SourceCount < 0 {
		return false
	}
	if descriptor.kind != rawPayloadFixed && descriptor.kind != rawPayloadTail && descriptor.kind != rawPayloadNil {
		return false
	}
	if _, dynamic := access.DynamicKey(); dynamic {
		if view.KeyCount > 1 || view.HeapCount > 0 && view.KeyCount != 1 {
			return false
		}
		if view.KeyCount == 1 && (!view.Key.valid || !view.Key.found) {
			return false
		}
		// A routed write must be downstream of the one present dynamic-key
		// observation. A selected-but-absent key lawfully settles only the
		// explicit all-empty no-route branch; it cannot carry a Heap route.
		if view.HeapCount > 0 && (!view.Key.valid || !view.Key.found || !view.Key.present) {
			return false
		}
	} else if view.KeyCount != 0 {
		return false
	}
	if view.HeapCount == 0 {
		return view.PackCount == 0 && view.SourceCount == 0
	}
	wantPack := 0
	switch descriptor.kind {
	case rawPayloadFixed, rawPayloadNil:
		// These payloads have no Pack frontier.
	case rawPayloadTail:
		wantPack = 1
	default:
		return false
	}
	return view.PackCount == wantPack && view.SourceCount == int(descriptor.sourceCount)
}

func (rule *RawSetRule) mutateRoute(access Index, route heapdomain.RawRouteTag, fact heapdomain.Value, view RawSetFrame) (heapdomain.Value, bool) {
	if !rule.owns(access) || !fact.Valid() {
		return heapdomain.Value{}, false
	}
	return rule.topology.RawSetMutateRoute(access, route, fact, view)
}

func (rule *RawSetRule) takeSetScratch() *rawSetScratch {
	value, ok := rule.scratch.Get().(*rawSetScratch)
	if !ok || value == nil {
		return &rawSetScratch{}
	}
	return value
}

func (rule *RawSetRule) putSetScratch(value *rawSetScratch) {
	if value != nil {
		rule.scratch.Put(value)
	}
}
