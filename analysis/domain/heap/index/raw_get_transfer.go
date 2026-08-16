package index

import (
	calldomain "github.com/wippyai/go-lua/analysis/domain/call"
	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/domain/materialization"
	"github.com/wippyai/go-lua/analysis/domain/pack"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	"github.com/wippyai/go-lua/analysis/engine"
)

// rawGetView is the read-only input to the one raw-get reduction. Transfer
// and evidence provide different authenticated observation capabilities, but
// both execute this same domain reduction.
type rawGetView struct {
	scratch     *rawGetScratch
	key         rawSelected[valuedomain.Value]
	keyCount    int
	callCount   int
	call        func(uint64) rawSelected[calldomain.Value]
	heapCount   int
	heap        func(heapdomain.RawRouteTag, heapdomain.Key) rawSelected[heapdomain.Value]
	packCount   int
	pack        func(heapdomain.RawPayloadTag) rawSelected[pack.Value]
	sourceCount int
	source      func(rawSourceTag) rawSelected[valuedomain.Value]
}

type rawSelected[V any] struct {
	value   V
	present bool
	found   bool
	valid   bool
}

func (rule *RawGetRule) transfer(access engine.Access[valuedomain.Value, Access]) bool {
	operand, ok := engine.Operand(access)
	if !ok || !rule.owns(operand) {
		return false
	}
	return engine.Product(access, func(row engine.Row) bool {
		receiverCells, receiverOK := engine.ReadValue(access, row, rule.receiver)
		keys, keyOK := engine.ReadValue(access, row, rule.key)
		calls, callOK := engine.ReadValue(access, row, rule.call)
		heaps, heapOK := engine.ReadValue(access, row, rule.heapRead)
		packs, packOK := engine.ReadValue(access, row, rule.packRead)
		sources, sourceOK := engine.ReadValue(access, row, rule.sourceRead)
		if !receiverOK || !keyOK || !callOK || !heapOK || !packOK || !sourceOK || receiverCells.Count() != 1 {
			return false
		}
		receiver, receiverPresent, receiverAvailable := receiverCells.At(0)
		if !receiverAvailable {
			return false
		}
		if !receiverPresent {
			return transferSelectionsEmpty(access, row, keys, calls, heaps, packs, sources) && engine.NoCandidate(access, row)
		}
		scratch := rule.takeScratch()
		defer rule.putScratch(scratch)
		view, ok := transferRawGetView(access, row, operand, keys, calls, heaps, packs, sources, scratch)
		if !ok {
			return false
		}
		result, contributed, valid := rule.reduce(operand, receiver, view)
		if !valid {
			return false
		}
		if !contributed {
			return engine.NoCandidate(access, row)
		}
		return engine.StageValue(access, row, result)
	})
}

func transferSelectionsEmpty(
	access engine.Access[valuedomain.Value, Access],
	row engine.Row,
	keys engine.Selection[uint64, engine.OrderedCells[valuedomain.Value]],
	calls engine.Selection[uint64, engine.OrderedCells[calldomain.Value]],
	heaps engine.Selection[heapdomain.RawRouteTag, engine.OrderedCells[heapdomain.Value]],
	packs engine.Selection[heapdomain.RawPayloadTag, engine.OrderedCells[pack.Value]],
	sources engine.Selection[rawSourceTag, engine.OrderedCells[valuedomain.Value]],
) bool {
	keyCount, keyOK := engine.SelectionCount(access, row, keys)
	callCount, callOK := engine.SelectionCount(access, row, calls)
	heapCount, heapOK := engine.SelectionCount(access, row, heaps)
	packCount, packOK := engine.SelectionCount(access, row, packs)
	sourceCount, sourceOK := engine.SelectionCount(access, row, sources)
	return keyOK && callOK && heapOK && packOK && sourceOK && keyCount == 0 && callCount == 0 && heapCount == 0 && packCount == 0 && sourceCount == 0
}

func transferRawGetView(
	access engine.Access[valuedomain.Value, Access], row engine.Row, operand Access,
	keys engine.Selection[uint64, engine.OrderedCells[valuedomain.Value]],
	calls engine.Selection[uint64, engine.OrderedCells[calldomain.Value]],
	heaps engine.Selection[heapdomain.RawRouteTag, engine.OrderedCells[heapdomain.Value]],
	packs engine.Selection[heapdomain.RawPayloadTag, engine.OrderedCells[pack.Value]],
	sources engine.Selection[rawSourceTag, engine.OrderedCells[valuedomain.Value]],
	scratch *rawGetScratch,
) (rawGetView, bool) {
	if scratch == nil {
		return rawGetView{}, false
	}
	view := rawGetView{scratch: scratch}
	var ok bool
	view.keyCount, ok = engine.SelectionCount(access, row, keys)
	if !ok {
		return rawGetView{}, false
	}
	view.callCount, ok = engine.SelectionCount(access, row, calls)
	if !ok {
		return rawGetView{}, false
	}
	view.heapCount, ok = engine.SelectionCount(access, row, heaps)
	if !ok {
		return rawGetView{}, false
	}
	view.packCount, ok = engine.SelectionCount(access, row, packs)
	if !ok {
		return rawGetView{}, false
	}
	view.sourceCount, ok = engine.SelectionCount(access, row, sources)
	if !ok {
		return rawGetView{}, false
	}
	if !buildTransferIndex(access, row, calls, view.callCount, &scratch.call) ||
		!buildTransferIndex(access, row, heaps, view.heapCount, &scratch.heap) ||
		!buildTransferIndex(access, row, packs, view.packCount, &scratch.pack) ||
		!buildTransferIndex(access, row, sources, view.sourceCount, &scratch.value) {
		return rawGetView{}, false
	}
	if _, dynamic := operand.DynamicKey(); dynamic {
		view.key = transferSelectionValue(access, row, keys, nil, uint64(1))
		if !view.key.valid || !view.key.found {
			return rawGetView{}, false
		}
	}
	view.call = func(tag uint64) rawSelected[calldomain.Value] {
		return transferSelectionValue(access, row, calls, &scratch.call, tag)
	}
	view.heap = func(tag heapdomain.RawRouteTag, _ heapdomain.Key) rawSelected[heapdomain.Value] {
		return transferSelectionValue(access, row, heaps, &scratch.heap, tag)
	}
	view.pack = func(tag heapdomain.RawPayloadTag) rawSelected[pack.Value] {
		return transferSelectionValue(access, row, packs, &scratch.pack, tag)
	}
	view.source = func(tag rawSourceTag) rawSelected[valuedomain.Value] {
		return transferSelectionValue(access, row, sources, &scratch.value, tag)
	}
	return view, true
}

func transferSelectionValue[Out any, O any, S any, Tag interface {
	~uint8 | ~uint16 | ~uint32 | ~uint64
}](
	access engine.Access[Out, O], row engine.Row, selection engine.Selection[Tag, engine.OrderedCells[S]], index *rawSelectionIndex, want Tag,
) rawSelected[S] {
	ordinal := 0
	if index == nil {
		count, ok := engine.SelectionCount(access, row, selection)
		if !ok || count != 1 {
			return rawSelected[S]{}
		}
	} else {
		found, ok := index.ordinal(uint64(want))
		if !ok {
			return rawSelected[S]{valid: true}
		}
		ordinal = found
	}
	tag, cells, selected := engine.SelectionAt(access, row, selection, ordinal)
	if !selected || tag != want || cells.Count() != 1 {
		return rawSelected[S]{}
	}
	value, present, available := cells.At(0)
	if !available {
		return rawSelected[S]{}
	}
	return rawSelected[S]{value: value, present: present, found: true, valid: true}
}

func buildTransferIndex[Out any, O any, S any, Tag interface {
	~uint8 | ~uint16 | ~uint32 | ~uint64
}](access engine.Access[Out, O], row engine.Row, selection engine.Selection[Tag, engine.OrderedCells[S]], count int, index *rawSelectionIndex) bool {
	return index.build(count, func(ordinal int) (uint64, bool) {
		tag, cells, selected := engine.SelectionAt(access, row, selection, ordinal)
		if !selected || cells.Count() != 1 {
			return 0, false
		}
		_, _, available := cells.At(0)
		return uint64(tag), available
	})
}

func (rule *RawGetRule) reduce(operand Access, receiver valuedomain.Value, view rawGetView) (valuedomain.Value, bool, bool) {
	if rule == nil || !rule.owns(operand) || view.scratch == nil || view.call == nil || view.heap == nil || view.pack == nil || view.source == nil {
		return valuedomain.Value{}, false, false
	}
	result, any := rule.valueSchema().Bottom(), false
	selectorCount := 0
	if !rule.visitKeySelectors(operand, view, func(heapdomain.KeySelector) bool {
		selectorCount++
		return true
	}) {
		return valuedomain.Value{}, false, false
	}
	if selectorCount == 0 {
		return result, false, view.callCount == 0 && view.heapCount == 0 && view.packCount == 0 && view.sourceCount == 0
	}

	callCount := 0
	if rule.runtime.visitCallDemand == nil || !rule.runtime.visitCallDemand(receiver, func(_ calldomain.Key, tag uint64) bool {
		selected := view.call(tag)
		if !selected.valid || !selected.found {
			return false
		}
		callCount++
		return true
	}) || callCount != view.callCount {
		return valuedomain.Value{}, false, false
	}
	census := rawGetCensus{scratch: view.scratch}
	heapCount := 0
	callStateValid := true
	callState := func(_ calldomain.Key, tag uint64) (calldomain.Value, bool) {
		selected := view.call(tag)
		if !selected.valid || !selected.found {
			callStateValid = false
			return calldomain.Value{}, false
		}
		return selected.value, selected.present
	}
	valid := rule.runtime.visitReceiver(receiver, callState, func(route Route) bool {
		switch route.Kind() {
		case RouteRoot:
			key, role, rooted := route.Root()
			if !rooted {
				return false
			}
			tag, tagged := rule.heapSchema().RouteTag(key, role)
			if !tagged {
				return false
			}
			selected := view.heap(tag, key)
			if !selected.valid || !selected.found {
				return false
			}
			heapCount++
			if !selected.present {
				return true
			}
			return rule.visitKeySelectors(operand, view, func(selector heapdomain.KeySelector) bool {
				return rule.applyHeap(tag, selected.value, selector, view, &census, &result, &any)
			})
		case RouteUnknown:
			return rule.joinPresentTop(&result, &any)
		case RouteOther:
			return true
		default:
			return false
		}
	})
	if !valid || !callStateValid || heapCount != view.heapCount || census.pack != view.packCount || census.source != view.sourceCount {
		return valuedomain.Value{}, false, false
	}
	return result, any, true
}

func (rule *RawGetRule) visitKeySelectors(operand Access, view rawGetView, visit func(heapdomain.KeySelector) bool) bool {
	if visit == nil {
		return false
	}
	if _, dynamic := operand.DynamicKey(); !dynamic {
		if view.keyCount != 0 {
			return false
		}
		selector, ok := rule.keySelector(operand)
		return ok && visit(selector)
	}
	if view.keyCount != 1 || !view.key.valid || !view.key.found {
		return false
	}
	if !view.key.present {
		return true
	}
	selectors := rule.runtime.topology.selectors
	return selectors != nil && selectors.Visit(view.key.value, visit)
}

type rawGetCensus struct {
	scratch *rawGetScratch
	pack    int
	source  int
}

func (rule *RawGetRule) applyHeap(tag heapdomain.RawRouteTag, fact heapdomain.Value, selector heapdomain.KeySelector, view rawGetView, census *rawGetCensus, result *valuedomain.Value, any *bool) bool {
	if rule.runtime.visitRawRoute == nil {
		return false
	}
	return rule.runtime.visitRawRoute(tag, fact, selector, func(raw heapdomain.RawAccess) bool {
		if raw.IsTop() {
			return rule.joinPresentTop(result, any)
		}
		cell, ok := raw.Cell()
		if !ok {
			return false
		}
		for index := 0; index < cell.PresentCount(); index++ {
			present, ok := cell.PresentAt(index)
			if !ok || !rule.applyPresent(tag, raw, present, view, census, result, any) {
				return false
			}
		}
		return true
	})
}

func (rule *RawGetRule) applyPresent(route heapdomain.RawRouteTag, raw heapdomain.RawAccess, present heapdomain.Present, view rawGetView, census *rawGetCensus, result *valuedomain.Value, any *bool) bool {
	valueContainment, _, ok := present.Containment()
	if !ok {
		return false
	}
	if _, _, boot := raw.InitialPayload(present); boot {
		value, ok := rule.bootInitial(route, raw, present)
		return ok && rule.reduceAndJoin(valueContainment, value, result, any)
	}
	tag, ok := raw.PayloadTag(present)
	if !ok {
		return false
	}
	payload, ok := rule.payloadAt(tag)
	if !ok || !rule.requirePayload(tag, payload, view, census) {
		return false
	}
	switch payload.kind {
	case rawPayloadNil:
		return true
	case rawPayloadInitial:
		return false
	case rawPayloadFixed:
		tags, tagsOK := rule.runtime.topology.catalog.sourceTags(tag)
		if !tagsOK || len(tags) != 1 {
			return false
		}
		selected := view.source(tags[0])
		return selected.valid && selected.found && (!selected.present || rule.reduceAndJoin(valueContainment, selected.value, result, any))
	case rawPayloadTail:
		selected := view.pack(tag)
		if !selected.valid || !selected.found {
			return false
		}
		if !selected.present {
			return true
		}
		root, rootOK := payload.payload.Root()
		selection, selectionOK := payload.payload.Selection()
		values, valuesOK := payload.payload.Values()
		observation, observed := rule.packSchema().ObserveScalar(root, selected.value, values, selection)
		if !rootOK || !selectionOK || !valuesOK || !observed {
			return false
		}
		if observation.IsBottom() {
			return true
		}
		if observation.IsTop() {
			return rule.reduceAndJoin(valueContainment, rule.valueSchema().Top(), result, any)
		}
		for index := 0; index < observation.Count(); index++ {
			scalar, ok := observation.At(index)
			if !ok || !rule.applyScalar(view, tag, payload, scalar, valueContainment, result, any) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func (rule *RawGetRule) applyScalar(view rawGetView, payloadTag heapdomain.RawPayloadTag, payload rawPayload, scalar pack.Scalar, containment heapdomain.Containment, result *valuedomain.Value, any *bool) bool {
	if scalar.Kind() == pack.ScalarEndpoint {
		// An exact Pack endpoint is a symbolic reference to one existing Link
		// value, not a value fact embedded in Pack.  Resolve it through Pack's
		// sealed source projection, then use RawGet's already-declared Value
		// selector.  This keeps Value as the sole authority for the fact and
		// preserves the endpoint identity without a body-wide Pack aggregate.
		source, sourceOK := rule.packSchema().ScalarSource(scalar)
		selected := rule.sourceValue(view, payloadTag, source)
		if !sourceOK || !selected.valid || !selected.found {
			return false
		}
		if !selected.present {
			return true
		}
		return rule.reduceAndJoin(containment, selected.value, result, any)
	}
	kinds, kindsOK := payload.payload.ScalarMayRuntimeKinds(scalar)
	value, valueOK := rule.valueSchema().ForRuntimeKinds(kinds)
	return kindsOK && valueOK && rule.reduceAndJoin(containment, value, result, any)
}

func (rule *RawGetRule) sourceValue(view rawGetView, payloadTag heapdomain.RawPayloadTag, want pack.SemanticSource) rawSelected[valuedomain.Value] {
	tag, found := rule.sourceTag(payloadTag, want)
	if !found {
		return rawSelected[valuedomain.Value]{}
	}
	if _, ok := rule.sourceAt(tag); !ok {
		return rawSelected[valuedomain.Value]{}
	}
	return view.source(tag)
}

func (rule *RawGetRule) requirePayload(tag heapdomain.RawPayloadTag, payload rawPayload, view rawGetView, census *rawGetCensus) bool {
	if census == nil || census.scratch == nil {
		return false
	}
	if payload.kind == rawPayloadTail && !marked(census.scratch.payload, uint64(tag)) {
		selected := view.pack(tag)
		if !selected.valid || !selected.found {
			return false
		}
		mark(census.scratch.payload, uint64(tag))
		census.pack++
	}
	tags, tagsOK := rule.runtime.topology.catalog.sourceTags(tag)
	if !tagsOK {
		return false
	}
	for _, sourceTag := range tags {
		if marked(census.scratch.source, uint64(sourceTag)) {
			continue
		}
		selected := view.source(sourceTag)
		if !selected.valid || !selected.found {
			return false
		}
		mark(census.scratch.source, uint64(sourceTag))
		census.source++
	}
	return true
}

func (rule *RawGetRule) reduceAndJoin(containment heapdomain.Containment, value valuedomain.Value, result *valuedomain.Value, any *bool) bool {
	values := rule.valueSchema()
	var stored valuedomain.Value
	var ok bool
	switch containment.Kind() {
	case heapdomain.ContainmentNone:
		stored, ok = values.FilterStoredNone(value)
	case heapdomain.ContainmentUnknown:
		stored, ok = values.FilterStoredUnknown(value)
	case heapdomain.ContainmentExact:
		reference, referenceOK := containment.Reference()
		if !referenceOK {
			return false
		}
		var selector valuedomain.Atom
		if root, role, allocation := reference.Key(); allocation && root.Kind() == heapdomain.RootAllocation {
			selector, ok = values.Allocation(root, role)
		} else if rootID, role, boot := reference.BootID(); boot && role == materialization.Exact {
			selector, ok = values.BootID(rootID)
		}
		if ok {
			stored, ok = values.FilterStoredExact(value, selector)
		}
	default:
		return false
	}
	if !ok {
		return false
	}
	present, ok := values.FilterPresent(stored)
	if !ok {
		return false
	}
	if values.Equal(present, values.Bottom()) {
		return true
	}
	return rule.join(result, any, present)
}

func (rule *RawGetRule) join(result *valuedomain.Value, any *bool, value valuedomain.Value) bool {
	next, ok := rule.valueSchema().Join(*result, value)
	if !ok {
		return false
	}
	*result, *any = next, true
	return true
}

func (rule *RawGetRule) joinPresentTop(result *valuedomain.Value, any *bool) bool {
	present, ok := rule.valueSchema().FilterPresent(rule.valueSchema().Top())
	return ok && rule.join(result, any, present)
}
