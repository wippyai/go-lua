package index

import (
	"github.com/wippyai/go-lua/analysis/engine"
	calldomain "github.com/wippyai/go-lua/domain/call"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
	"github.com/wippyai/go-lua/domain/pack"
	packowner "github.com/wippyai/go-lua/domain/pack/owner"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

type RawGetHotRule struct {
	implementation *valueowner.RuleImplementation[Index]
	core           *RawGetRule
	values         *valueowner.HotOwner
}

func (rule *RawGetRule) operandContent(access Index) (Index, [32]byte, bool) {
	if rule == nil || !rule.owns(access) {
		return Index{}, [32]byte{}, false
	}
	return access, [32]byte(access.id), true
}

func (rule *RawGetHotRule) Implementation() (*valueowner.RuleImplementation[Index], bool) {
	if rule == nil || rule.values == nil || rule.implementation == nil {
		return nil, false
	}
	_, ok := valueowner.ResolveRuleImplementationFor(rule.values, rule.implementation)
	return rule.implementation, ok
}

func BindRawGetHot(binding *engine.SchemaBinding, fragment *RawGetSchemaFragment, topology *Topology, values *valueowner.HotOwner, calls *callowner.HotOwner, heap *heapowner.HotOwner, packs *packowner.HotOwner) (*RawGetHotRule, bool) {
	if binding == nil || fragment == nil || topology == nil || !topology.valid() || values == nil || !values.MatchesBinding(binding) || calls == nil || !calls.MatchesBinding(binding) || heap == nil || !heap.MatchesBinding(binding) || packs == nil || !packs.MatchesBinding(binding) || values.Schema() != topology.values || calls.Algebra() != topology.calls || heap.Schema() != topology.heap || !packs.OwnsSchema(topology.packs) {
		return nil, false
	}
	runtime := &rawGetRuntime{topology: topology, values: topology.values, calls: topology.calls, heap: topology.heap}
	// Hot observation uses Topology's owner-local indexed visitors. Topology
	// owns the canonical freshByRoot index and pooled epoch marks, so this path
	// stays allocation-free for exact demand and O(outputs) when widened.
	runtime.visitCallDemand = topology.VisitReceiverCallDemand
	runtime.visitReceiver = topology.VisitReceiver
	runtime.selectorForSlot = topology.heap.SelectorForSlot
	runtime.callRoute = func(context engine.SelectorContext, key calldomain.Key, tag uint64) bool {
		return calls.SelectRoute(context, key, tag)
	}
	runtime.valueRoute = func(context engine.SelectorContext, coordinate valuedomain.Coordinate, tag uint64) bool {
		return values.SelectRoute(context, coordinate, tag)
	}
	// The semantic-source read is declared over RawSourceTag, so its routes
	// carry that tag type. Emitting the same ordinal as a bare uint64 mints a
	// route the staged sink cannot accept.
	runtime.sourceRoute = func(context engine.SelectorContext, coordinate valuedomain.Coordinate, tag RawSourceTag) bool {
		return valueowner.SelectRouteTyped(values, context, coordinate, tag)
	}
	runtime.heapRoute = func(context engine.SelectorContext, key heapdomain.Key, tag heapdomain.RawRouteTag) bool {
		return heap.SelectRoute(context, key, tag)
	}
	runtime.packRoute = func(context engine.SelectorContext, root pack.Root, tag heapdomain.RawPayloadTag) bool {
		return packowner.SelectRouteTyped(packs, context, root, tag)
	}
	core := &RawGetRule{runtime: runtime}
	core.scratch.New = func() any {
		scratch, _ := NewRawGetScratch(topology)
		return scratch
	}
	core.scratch.Put(core.scratch.New())
	rule := &RawGetHotRule{core: core, values: values}
	implementation, bound := valueowner.BindSelectedRuleDirect(values, fragment.slot, fragment.carry, fragment.write, values.FactorRef(), engine.HotRuleSpec[valuedomain.Value, Index]{OperandContent: core.operandContent, OperandResolver: rule.resolveOperand, Fold: core.fold}, engine.HotCarrySpec[valuedomain.Value, Index]{}, func(access Index) (uint64, bool) {
		result, resultOK := access.Result()
		index, indexOK := values.Schema().CoordinateIndex(result)
		return uint64(index), resultOK && indexOK
	})
	if !bound {
		return nil, false
	}
	var ok bool
	if core.receiver, ok = valueowner.AddSelectedRuleDirectExactRead(implementation, fragment.receiver, values.FactorRef(), func(access Index) (uint64, bool) {
		receiver, receiverOK := access.Receiver()
		index, indexOK := values.Schema().CoordinateIndex(receiver)
		return uint64(index), receiverOK && indexOK
	}); !ok {
		return nil, false
	}
	if core.key, ok = valueowner.AddSelectedRuleDirectOperandRead[Index, valuedomain.Value, uint64](implementation, fragment.key, values.FactorRef(), core.locateKey); !ok {
		return nil, false
	}
	if core.call, ok = valueowner.AddSelectedRuleDirectOperandRead[Index, calldomain.Value, uint64](implementation, fragment.call, calls.FactorRef(), core.locateCall); !ok {
		return nil, false
	}
	if core.heapRead, ok = valueowner.AddSelectedRuleDirectOperandRead[Index, heapdomain.Value, heapdomain.RawRouteTag](implementation, fragment.heapRead, heap.FactorRef(), core.locateHeap); !ok {
		return nil, false
	}
	if core.packRead, ok = valueowner.AddSelectedRuleDirectOperandRead[Index, pack.Value, heapdomain.RawPayloadTag](implementation, fragment.packRead, packs.FactorRef(), core.locatePack); !ok {
		return nil, false
	}
	if core.sourceRead, ok = valueowner.AddSelectedRuleDirectOperandRead[Index, valuedomain.Value, RawSourceTag](implementation, fragment.sourceRead, values.FactorRef(), core.locateSource); !ok {
		return nil, false
	}
	rule.implementation = implementation
	return rule, true
}

func (rule *RawGetHotRule) resolveOperand(coords engine.OperandCoords) (Index, bool) {
	if rule == nil || rule.core == nil || rule.core.runtime == nil || rule.core.runtime.topology == nil {
		return Index{}, false
	}
	topology := rule.core.runtime.topology
	mount, mountOK := topology.heap.OccurrenceMountForModule(coords.Mount)
	if !mountOK {
		return Index{}, false
	}
	indexAccess, accessOK := mount.IndexAccessForOccurrence(coords.Occurrence, true)
	if !accessOK {
		return Index{}, false
	}
	access, accessOK := topology.Access(indexAccess)
	if !accessOK {
		return Index{}, false
	}
	return access, access.Read()
}
