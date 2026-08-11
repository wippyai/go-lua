package index

import (
	"sync"

	calldomain "github.com/wippyai/go-lua/analysis/domain/call"
	callowner "github.com/wippyai/go-lua/analysis/domain/call/owner"
	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/domain/heap/keymatch"
	heapowner "github.com/wippyai/go-lua/analysis/domain/heap/owner"
	"github.com/wippyai/go-lua/analysis/domain/pack"
	packowner "github.com/wippyai/go-lua/analysis/domain/pack/owner"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	"github.com/wippyai/go-lua/analysis/engine"
)

// RawGetRule is the sole Value-producing judgment for an executable typed Read
// candidate. It owns selection only; Heap and Pack remain the semantic owners
// of storage and Lua adjustment respectively.
type RawGetRule struct {
	topology *Topology
	values   *valueowner.Owner
	calls    *callowner.Owner
	heap     *heapowner.Owner
	packs    *packowner.Owner
	// selectors is keymatch's sealed Value-relation projection.  RawGet uses
	// it but never rebuilds selector equality or deduplication locally.
	selectors *keymatch.SelectorProjection
	payloads  []rawPayload
	sources   []rawSource

	rule       *engine.Rule[valuedomain.Value, Access]
	receiver   engine.Read[engine.OrderedCells[valuedomain.Value]]
	key        engine.Read[engine.Selection[uint64, engine.OrderedCells[valuedomain.Value]]]
	call       engine.Read[engine.Selection[uint64, engine.OrderedCells[calldomain.Value]]]
	heapRead   engine.Read[engine.Selection[heapdomain.RawRouteTag, engine.OrderedCells[heapdomain.Value]]]
	packRead   engine.Read[engine.Selection[heapdomain.RawPayloadTag, engine.OrderedCells[pack.Value]]]
	sourceRead engine.Read[engine.Selection[rawSourceTag, engine.OrderedCells[valuedomain.Value]]]
	write      engine.Write[valuedomain.Value]

	scratch sync.Pool
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

func DeclareRawGet(composition *engine.Composition, semantic, family, evidence engine.SemanticKey, topology *Topology, values *valueowner.Owner, calls *callowner.Owner, heap *heapowner.Owner, packs *packowner.Owner) (*RawGetRule, bool) {
	if composition == nil || topology == nil || values == nil || calls == nil || heap == nil || packs == nil ||
		!semantic.Available() || !family.Available() || !evidence.Available() || semantic == family || semantic == evidence || family == evidence ||
		!topology.valid() || values.Schema() != topology.values || calls.Algebra() != topology.calls || heap.Schema() != topology.heap ||
		packs.Schema() == nil || packs.Schema().Link() != values.Schema().Link() {
		return nil, false
	}
	payloads, sources, ok := buildRawPayloads(topology, packs.Schema())
	if !ok {
		return nil, false
	}
	selectors, ok := keymatch.NewSelectorProjection(heap.Schema(), values.Schema())
	if !ok {
		return nil, false
	}
	result := &RawGetRule{topology: topology, values: values, calls: calls, heap: heap, packs: packs, selectors: selectors, payloads: payloads, sources: sources}
	result.scratch.New = func() any {
		return &rawGetScratch{
			payload: make([]uint64, bitWords(len(payloads)-1)),
			source:  make([]uint64, bitWords(len(sources))),
		}
	}
	result.scratch.Put(result.scratch.New())
	declared, ok := engine.DeclareRule(composition, engine.RuleSpec[valuedomain.Value, Access]{
		Semantic: semantic, OperandFamily: family, OperandContent: rawGetContent,
		Output: values.Output(), Inputs: 4,
		Admission: engine.AdmitRuleByDerivation(evidence, result.check(semantic)), Transfer: result.transfer,
	}, result.declare)
	if !ok || declared == nil || result.rule != declared {
		return nil, false
	}
	return result, true
}

func rawGetContent(access Access) (Access, [32]byte, bool) {
	id, ok := access.ID()
	return access, [32]byte(id), ok && access.Read()
}

func (rule *RawGetRule) owns(access Access) bool {
	if !rule.valid() || !rule.topology.OwnsAccess(access) || !access.Read() {
		return false
	}
	return true
}

// valid is the runtime declaration fence for RawGet.  The Heap owner and
// Topology must retain the exact same immutable Heap Schema handle; matching
// ContentID values are insufficient because independently sealed same-Link
// schemas issue distinct owner-bound coordinates.
func (rule *RawGetRule) valid() bool {
	return rule != nil && rule.topology != nil && rule.topology.valid() && rule.values != nil && rule.calls != nil && rule.heap != nil && rule.packs != nil &&
		rule.values.Schema() == rule.topology.values && rule.calls.Algebra() == rule.topology.calls && rule.heap.Schema() == rule.topology.heap &&
		rule.packs.Schema() != nil && rule.packs.Schema().Link() == rule.values.Schema().Link()
}

func (rule *RawGetRule) declare(raw *engine.Rule[valuedomain.Value, Access]) bool {
	valueIn, a := raw.InputAt(0)
	callIn, b := raw.InputAt(1)
	heapIn, c := raw.InputAt(2)
	packIn, d := raw.InputAt(3)
	if !a || !b || !c || !d {
		return false
	}
	var ok bool
	rule.receiver, ok = engine.ReadFrom(raw, valueIn, rule.values.ExactRead())
	if !ok {
		return false
	}
	rule.key, ok = engine.SelectRead[valuedomain.Value, Access, valuedomain.Value, engine.OrderedCells[valuedomain.Value], uint64](raw, valueIn, rule.values.ExactRead(), []engine.Dependency{engine.ReadDependency(rule.receiver)}, rule.locateKey)
	if !ok {
		return false
	}
	rule.call, ok = engine.SelectRead[valuedomain.Value, Access, calldomain.Value, engine.OrderedCells[calldomain.Value], uint64](raw, callIn, rule.calls.ExactRead(), []engine.Dependency{engine.ReadDependency(rule.receiver), engine.ReadDependency(rule.key)}, rule.locateCall)
	if !ok {
		return false
	}
	rule.heapRead, ok = engine.SelectRead[valuedomain.Value, Access, heapdomain.Value, engine.OrderedCells[heapdomain.Value], heapdomain.RawRouteTag](raw, heapIn, rule.heap.ExactRead(), []engine.Dependency{engine.ReadDependency(rule.receiver), engine.ReadDependency(rule.key), engine.ReadDependency(rule.call)}, rule.locateHeap)
	if !ok {
		return false
	}
	rule.packRead, ok = engine.SelectRead[valuedomain.Value, Access, pack.Value, engine.OrderedCells[pack.Value], heapdomain.RawPayloadTag](raw, packIn, rule.packs.ExactRead(), []engine.Dependency{engine.ReadDependency(rule.key), engine.ReadDependency(rule.heapRead)}, rule.locatePack)
	if !ok {
		return false
	}
	rule.sourceRead, ok = engine.SelectRead[valuedomain.Value, Access, valuedomain.Value, engine.OrderedCells[valuedomain.Value], rawSourceTag](raw, valueIn, rule.values.ExactRead(), []engine.Dependency{engine.ReadDependency(rule.key), engine.ReadDependency(rule.heapRead), engine.ReadDependency(rule.packRead)}, rule.locateSource)
	if !ok {
		return false
	}
	if !engine.CarryFrom(raw, valueIn, rule.values.Carry()) {
		return false
	}
	rule.write, ok = engine.WriteTo(raw, rule.values.ExactWrite())
	if ok {
		rule.rule = raw
	}
	return ok
}

func (rule *RawGetRule) Instance(access Access) (*engine.RuleInstance[valuedomain.Value, Access], bool) {
	if rule == nil || rule.rule == nil || !rule.owns(access) {
		return nil, false
	}
	receiver, a := access.Receiver()
	result, b := access.Result()
	receiverRef, c := rule.values.Locate(receiver)
	resultRef, d := rule.values.Locate(result)
	if !a || !b || !c || !d {
		return nil, false
	}
	return engine.NewRuleInstance(rule.rule, access, func(binding *engine.RuleBinding[valuedomain.Value, Access]) bool {
		return engine.InstanceRead(binding, rule.receiver, receiverRef) &&
			engine.InstanceSelectorRead(binding, rule.key, rule.values.ExactRead()) &&
			engine.InstanceSelectorRead(binding, rule.call, rule.calls.ExactRead()) &&
			engine.InstanceSelectorRead(binding, rule.heapRead, rule.heap.ExactRead()) &&
			engine.InstanceSelectorRead(binding, rule.packRead, rule.packs.ExactRead()) &&
			engine.InstanceSelectorRead(binding, rule.sourceRead, rule.values.ExactRead()) &&
			engine.InstanceWrite(binding, rule.write, resultRef)
	})
}

func (rule *RawGetRule) locateKey(context engine.SelectorContext, access Access) bool {
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
	ref, ok := rule.values.Locate(coordinate)
	return ok && engine.SelectRoute(context, ref, uint64(1))
}

func (rule *RawGetRule) locateCall(context engine.SelectorContext, access Access) bool {
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
	return rule.topology.VisitReceiverCallDemand(receiver, func(key calldomain.Key, tag uint64) bool {
		ref, found := rule.calls.Locate(key)
		return found && engine.SelectRoute(context, ref, tag)
	})
}

func (rule *RawGetRule) locateHeap(context engine.SelectorContext, access Access) bool {
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
	visited := rule.topology.VisitReceiver(receiver, state, func(route Route) bool {
		key, role, rooted := route.Root()
		if !rooted {
			return true
		}
		ref, found := rule.heap.Locate(key)
		tag, tagged := rule.heap.Schema().RouteTag(key, role)
		return found && tagged && engine.SelectRoute(context, ref, tag)
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

func (rule *RawGetRule) keySelector(access Access) (heapdomain.KeySelector, bool) {
	slot, ok := access.Slot()
	if !ok {
		return heapdomain.KeySelector{}, false
	}
	return rule.heap.Schema().SelectorForSlot(slot)
}
