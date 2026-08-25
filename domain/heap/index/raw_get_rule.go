package index

import (
	"sync"

	"github.com/wippyai/go-lua/analysis/engine"
	calldomain "github.com/wippyai/go-lua/domain/call"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/pack"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// RawGetRule is the sole Value-producing judgment for an executable typed Read
// candidate. It owns selection only; Heap and Pack remain the semantic owners
// of storage and Lua adjustment respectively.
type RawGetRule struct {
	receiver   engine.Read[engine.OrderedCells[valuedomain.Value]]
	key        engine.Read[engine.Selection[uint64, engine.OrderedCells[valuedomain.Value]]]
	call       engine.Read[engine.Selection[uint64, engine.OrderedCells[calldomain.Value]]]
	heapRead   engine.Read[engine.Selection[heapdomain.RawRouteTag, engine.OrderedCells[heapdomain.Value]]]
	packRead   engine.Read[engine.Selection[heapdomain.RawPayloadTag, engine.OrderedCells[pack.Value]]]
	sourceRead engine.Read[engine.Selection[RawSourceTag, engine.OrderedCells[valuedomain.Value]]]

	scratch sync.Pool
	runtime *rawGetRuntime
}

// rawGetRuntime is the sealed owner projection used by the receipt-native
// lane.  It contains no Rule/Composition or Link catalog; all structural
// tables are frozen before binding and all live reads route through typed
// owner callbacks.
type rawGetRuntime struct {
	topology        *Topology
	values          *valuedomain.Schema
	calls           *calldomain.Algebra
	heap            heapdomain.Schema
	visitCallDemand func(valuedomain.Value, func(calldomain.Key, uint64) bool) bool
	callRoute       func(engine.SelectorContext, calldomain.Key, uint64) bool
	valueRoute      func(engine.SelectorContext, valuedomain.Coordinate, uint64) bool
	sourceRoute     func(engine.SelectorContext, valuedomain.Coordinate, RawSourceTag) bool
	heapRoute       func(engine.SelectorContext, heapdomain.Key, heapdomain.RawRouteTag) bool
	packRoute       func(engine.SelectorContext, pack.Root, heapdomain.RawPayloadTag) bool
	visitReceiver   func(valuedomain.Value, CallState, func(Route) bool) bool
	visitRawRoute   func(heapdomain.RawRouteTag, heapdomain.Value, heapdomain.KeySelector, func(heapdomain.RawAccess) bool) bool
	selectorForSlot func(heapdomain.Slot) (heapdomain.KeySelector, bool)
}

func (rule *RawGetRule) valueSchema() *valuedomain.Schema {
	if rule == nil || rule.runtime == nil {
		return nil
	}
	return rule.runtime.values
}
func (rule *RawGetRule) callAlgebra() *calldomain.Algebra {
	if rule == nil || rule.runtime == nil {
		return nil
	}
	return rule.runtime.calls
}
func (rule *RawGetRule) heapSchema() heapdomain.Schema {
	if rule == nil || rule.runtime == nil {
		return heapdomain.Schema{}
	}
	return rule.runtime.heap
}
func (rule *RawGetRule) packSchema() *pack.Schema {
	if rule == nil || rule.runtime == nil || rule.runtime.topology == nil {
		return nil
	}
	return rule.runtime.topology.packs
}
func (rule *RawGetRule) payloadAt(tag heapdomain.RawPayloadTag) (rawPayload, bool) {
	if rule == nil || rule.runtime == nil || rule.runtime.topology == nil || rule.runtime.topology.catalog == nil {
		return rawPayload{}, false
	}
	return payloadAt(rule.runtime.topology.catalog.payloads, tag)
}
func (rule *RawGetRule) sourceAt(tag RawSourceTag) (rawSource, bool) {
	if rule == nil || rule.runtime == nil || rule.runtime.topology == nil || rule.runtime.topology.catalog == nil {
		return rawSource{}, false
	}
	return sourceAt(rule.runtime.topology.catalog.sources, tag)
}

// bootInitial reads the sealed Target boot-slot receipt selected by this route
// and Present tuple. Value issued the fact at Seal; the hot lane resolves it
// from Topology's own cold table and never reopens the Value schema, so the
// rule consumes nothing its declared footprint does not name.
func (rule *RawGetRule) bootInitial(route heapdomain.RawRouteTag, raw heapdomain.RawAccess, present heapdomain.Present) (valuedomain.Value, bool) {
	tag, ok := raw.PayloadTag(present)
	if !ok {
		return valuedomain.Value{}, false
	}
	return rule.bootInitialAt(route, tag)
}

func (rule *RawGetRule) bootInitialAt(route heapdomain.RawRouteTag, payload heapdomain.RawPayloadTag) (valuedomain.Value, bool) {
	if rule == nil || rule.runtime == nil || rule.runtime.topology == nil || rule.runtime.topology.catalog == nil || route == 0 || payload == 0 {
		return valuedomain.Value{}, false
	}
	value, ok := rule.runtime.topology.catalog.bootInitials[rawBootInitial{route: route, payload: payload}]
	return value, ok
}

func (rule *RawGetRule) sourceTag(payload heapdomain.RawPayloadTag, source pack.SemanticSource) (RawSourceTag, bool) {
	if rule == nil || rule.runtime == nil || rule.runtime.topology == nil || rule.runtime.topology.catalog == nil {
		return 0, false
	}
	return rule.runtime.topology.catalog.sourceTag(payload, source)
}

type rawGetScratch struct {
	payload []uint64
	source  []uint64
	call    rawSelectionIndex
	heap    rawSelectionIndex
	pack    rawSelectionIndex
	value   rawSelectionIndex
}

func bitWords(count int) int {
	if count <= 0 {
		return 0
	}
	return (count + 63) / 64
}

func (rule *RawGetRule) owns(access Index) bool {
	if !rule.valid() || !access.valid() || access.topology != rule.runtime.topology || !access.Read() {
		return false
	}
	return rule.runtime.values == rule.runtime.topology.values && rule.runtime.heap == rule.runtime.topology.heap && rule.runtime.calls == rule.runtime.topology.calls
}

// valid is the runtime declaration fence for RawGet.  The Heap owner and
// Topology must retain the exact same immutable Heap Schema handle; matching
// ContentID values are insufficient because independently sealed same-Link
// schemas issue distinct owner-bound coordinates.
func (rule *RawGetRule) valid() bool {
	return rule != nil && rule.runtime != nil && rule.runtime.topology != nil && rule.runtime.topology.valid() && rule.runtime.values != nil && rule.runtime.calls != nil && rule.runtime.heap.Valid()
}

func (rule *RawGetRule) locateKey(context engine.SelectorContext, access Index) bool {
	_, receiverPresent, receiverValid := selectorSingle(context, rule.receiver)
	if !rule.owns(access) || !receiverValid {
		return false
	}
	if !receiverPresent {
		return true
	}
	coordinate, dynamic := access.DynamicKey()
	if !dynamic {
		return true
	}
	return rule.runtime.valueRoute != nil && rule.runtime.valueRoute(context, coordinate, uint64(1))
}

func (rule *RawGetRule) locateCall(context engine.SelectorContext, access Index) bool {
	receiver, present, valid := selectorSingle(context, rule.receiver)
	if !rule.owns(access) || !valid {
		return false
	}
	if !present {
		return true
	}
	keyValid := false
	if !rule.visitContextKeySelectors(context, access, func(heapdomain.KeySelector) bool {
		keyValid = true
		return true
	}) {
		return false
	}
	if !keyValid {
		return true
	}
	return rule.runtime.visitCallDemand(receiver, func(key calldomain.Key, tag uint64) bool {
		return rule.callsRoute(context, key, tag)
	})
}

func (rule *RawGetRule) locateHeap(context engine.SelectorContext, access Index) bool {
	receiver, present, valid := selectorSingle(context, rule.receiver)
	if !rule.owns(access) || !valid {
		return false
	}
	if !present {
		return true
	}
	keyValid := false
	if !rule.visitContextKeySelectors(context, access, func(heapdomain.KeySelector) bool {
		keyValid = true
		return true
	}) {
		return false
	}
	if !keyValid {
		return true
	}
	selectedCalls, callsOK := engine.SelectorRead(context, rule.call)
	if !callsOK {
		return false
	}
	callCount, counted := engine.SelectorSelectionCount(context, selectedCalls)
	if !counted {
		return false
	}
	scratch := rule.takeScratch()
	defer rule.putScratch(scratch)
	if !scratch.call.build(callCount, func(ordinal int) (uint64, bool) {
		tag, cells, selected := engine.SelectorSelectionAt(context, selectedCalls, ordinal)
		if !selected || cells.Count() != 1 {
			return 0, false
		}
		_, _, available := cells.At(0)
		return tag, available
	}) {
		return false
	}
	stateValid := true
	state := func(_ calldomain.Key, tag uint64) (calldomain.Value, bool) {
		selected := selectorSelectionValue(context, selectedCalls, &scratch.call, tag)
		if !selected.valid || !selected.found {
			stateValid = false
			return calldomain.Value{}, false
		}
		return selected.value, selected.present
	}
	visited := rule.runtime.visitReceiver(receiver, state, func(route Route) bool {
		key, role, rooted := route.Root()
		if !rooted {
			return true
		}
		tag, tagged := rule.heapSchema().RouteTag(key, role)
		return tagged && rule.runtime.heapRoute != nil && rule.runtime.heapRoute(context, key, tag)
	})
	return visited && stateValid
}

func selectorSingle[V any](context engine.SelectorContext, read engine.Read[engine.OrderedCells[V]]) (V, bool, bool) {
	var zero V
	cells, ok := engine.SelectorRead(context, read)
	if !ok || cells.Count() != 1 {
		return zero, false, false
	}
	return cells.At(0)
}

func selectorSelectionValue[V any, Tag interface {
	~uint8 | ~uint16 | ~uint32 | ~uint64
}](
	context engine.SelectorContext,
	selection engine.Selection[Tag, engine.OrderedCells[V]],
	index *rawSelectionIndex,
	want Tag,
) rawSelected[V] {
	ordinal, ok := index.ordinal(uint64(want))
	if !ok {
		return rawSelected[V]{valid: true}
	}
	tag, cells, selected := engine.SelectorSelectionAt(context, selection, ordinal)
	if !selected || tag != want || cells.Count() != 1 {
		return rawSelected[V]{}
	}
	value, present, available := cells.At(0)
	if !available {
		return rawSelected[V]{}
	}
	return rawSelected[V]{value: value, present: present, found: true, valid: true}
}

func (rule *RawGetRule) keySelector(access Index) (heapdomain.KeySelector, bool) {
	slot, ok := access.Slot()
	if !ok {
		return heapdomain.KeySelector{}, false
	}
	if rule.runtime.selectorForSlot != nil {
		return rule.runtime.selectorForSlot(slot)
	}
	return heapdomain.KeySelector{}, false
}

func (rule *RawGetRule) callsRoute(context engine.SelectorContext, key calldomain.Key, tag uint64) bool {
	return rule != nil && rule.runtime != nil && rule.runtime.callRoute != nil && rule.runtime.callRoute(context, key, tag)
}
