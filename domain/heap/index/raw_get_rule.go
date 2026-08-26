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
	selectorForSlot func(heapdomain.Slot) (heapdomain.KeySelector, bool)
}

func (rule *RawGetRule) heapSchema() heapdomain.Schema {
	if rule == nil || rule.runtime == nil {
		return heapdomain.Schema{}
	}
	return rule.runtime.heap
}

// bootInitialAt reads the sealed Target boot-slot receipt for one route and
// payload tag. Value issued the fact at Seal, and the owner resolves it from
// its own cold table without reopening the Value schema, so this states where
// the read comes from and never how it is performed.
func (rule *RawGetRule) bootInitialAt(route heapdomain.RawRouteTag, payload heapdomain.RawPayloadTag) (valuedomain.Value, bool) {
	if rule == nil || rule.runtime == nil {
		return valuedomain.Value{}, false
	}
	return rule.runtime.topology.catalogBootInitial(route, payload)
}

// RawGetScratch is the solve-local reuse a raw-get reduction reads through.
// It holds no fact and no route: it is the marking and index storage one
// invocation reuses so a frontier is walked once, sized from the catalog it
// will be walked against.
type RawGetScratch struct {
	payload []uint64
	source  []uint64
	call    rawSelectionIndex
	heap    rawSelectionIndex
	pack    rawSelectionIndex
	value   rawSelectionIndex
}

// NewRawGetScratch opens the reuse storage one caller's reductions share. It
// is sized from the topology's own catalog, so a scratch is never sized
// against a frontier other than the one it marks.
func NewRawGetScratch(topology *Topology) (*RawGetScratch, bool) {
	if topology == nil || !topology.valid() || topology.catalog == nil {
		return nil, false
	}
	return &RawGetScratch{
		payload: make([]uint64, bitWords(len(topology.catalog.payloads)-1)),
		source:  make([]uint64, bitWords(len(topology.catalog.sources))),
	}, true
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
) Selected[V] {
	ordinal, ok := index.ordinal(uint64(want))
	if !ok {
		return Selected[V]{valid: true}
	}
	tag, cells, selected := engine.SelectorSelectionAt(context, selection, ordinal)
	if !selected || tag != want || cells.Count() != 1 {
		return Selected[V]{}
	}
	value, present, available := cells.At(0)
	if !available {
		return Selected[V]{}
	}
	return Selected[V]{value: value, present: present, found: true, valid: true}
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
