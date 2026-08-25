package index

import (
	"github.com/wippyai/go-lua/analysis/engine"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
	"github.com/wippyai/go-lua/domain/pack"
	packowner "github.com/wippyai/go-lua/domain/pack/owner"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// RawSetHotRule is the direct RawSet issuer. The reducer remains the one
// shared Fold plane; this type only retains owner-typed reads
// and the sealed Heap rule implementation.
type RawSetHotRule struct {
	implementation *heapowner.RuleImplementation[Index]
	core           *RawSetRule
	values         *valueowner.HotOwner
	heap           *heapowner.HotOwner
}

func (rule *RawSetRule) operandContent(access Index) (Index, [32]byte, bool) {
	if rule == nil || !rule.owns(access) {
		return Index{}, [32]byte{}, false
	}
	return access, [32]byte(access.id), true
}

// Implementation returns the exact Heap-owned issuer after the enclosing
// SchemaBinding seals.
func (rule *RawSetHotRule) Implementation() (*heapowner.RuleImplementation[Index], bool) {
	if rule == nil || rule.heap == nil || rule.implementation == nil {
		return nil, false
	}
	_, ok := heapowner.ResolveRuleImplementationFor(rule.heap, rule.implementation)
	return rule.implementation, ok
}

// BindRawSetHot binds RawSet's r0..r4 read chain, Heap carry, and route write
// directly at their declared schema ordinals. No construction transaction,
// cold Owner, Composition Rule, or copied Factor geometry is retained.
func BindRawSetHot(binding *engine.SchemaBinding, fragment *RawSetSchemaFragment, topology *Topology, values *valueowner.HotOwner, heap *heapowner.HotOwner, packs *packowner.HotOwner) (*RawSetHotRule, bool) {
	if binding == nil || fragment == nil || fragment.slot == nil || topology == nil || !topology.valid() || values == nil || !values.MatchesBinding(binding) || heap == nil || !heap.MatchesBinding(binding) || packs == nil || !packs.MatchesBinding(binding) || !packs.OwnsSchema(topology.packs) || values.Schema() != topology.values || heap.Schema() != topology.heap || !topology.packs.LinkOwner().Matches(values.Schema().LinkOwner()) {
		return nil, false
	}
	core := &RawSetRule{topology: topology}
	core.runtime = hotRawSetRuntime(values, heap, packs)
	core.scratch.New = func() any { return &rawSetScratch{} }
	core.scratch.Put(&rawSetScratch{})
	rule := &RawSetHotRule{core: core, values: values, heap: heap}
	implementation, bound := heapowner.BindSelectedRouteRuleDirect(heap, fragment.slot, fragment.carry, fragment.write, fragment.heapRef, engine.HotRuleSpec[heapdomain.Value, Index]{
		OperandContent:  core.operandContent,
		OperandResolver: rule.resolveOperand,
		Fold:            core.fold,
	}, engine.HotCarrySpec[heapdomain.Value, Index]{}, nil)
	if !bound {
		return nil, false
	}
	var ok bool
	if core.receiver, ok = heapowner.AddSelectedRouteRuleDirectExactRead(implementation, fragment.receiver, fragment.valueRef, func(access Index) (uint64, bool) {
		receiver, receiverOK := access.Receiver()
		index, indexOK := values.Schema().CoordinateIndex(receiver)
		return uint64(index), receiverOK && indexOK
	}); !ok {
		return nil, false
	}
	if core.key, ok = heapowner.AddSelectedRouteRuleDirectOperandRead[Index, valuedomain.Value, uint64](implementation, fragment.key, fragment.valueRef, core.locateKey); !ok {
		return nil, false
	}
	if core.heapRead, ok = heapowner.AddSelectedRouteRuleDirectOperandRead[Index, heapdomain.Value, heapdomain.RawRouteTag](implementation, fragment.heapRead, fragment.heapRef, core.locateHeap); !ok {
		return nil, false
	}
	if core.packRead, ok = heapowner.AddSelectedRouteRuleDirectOperandRead[Index, pack.Value, heapdomain.RawPayloadTag](implementation, fragment.packRead, fragment.packRef, core.locatePack); !ok {
		return nil, false
	}
	if core.source, ok = heapowner.AddSelectedRouteRuleDirectOperandRead[Index, valuedomain.Value, RawSourceTag](implementation, fragment.sourceRead, fragment.valueRef, core.locateSource); !ok {
		return nil, false
	}
	rule.implementation = implementation
	return rule, true
}

func (rule *RawSetHotRule) resolveOperand(coords engine.OperandCoords) (Index, bool) {
	if rule == nil || rule.core == nil || rule.core.topology == nil {
		return Index{}, false
	}
	topology := rule.core.topology
	mount, mountOK := topology.heap.OccurrenceMountForModule(coords.Mount)
	if !mountOK {
		return Index{}, false
	}
	indexAccess, accessOK := mount.IndexAccessForOccurrence(coords.Occurrence, false)
	if !accessOK {
		return Index{}, false
	}
	access, accessOK := topology.Access(indexAccess)
	if !accessOK {
		return Index{}, false
	}
	return access, access.Write()
}

func hotRawSetRuntime(values *valueowner.HotOwner, heap *heapowner.HotOwner, packs *packowner.HotOwner) *rawSetRuntime {
	runtime := &rawSetRuntime{values: values.Schema(), heap: heap.Schema()}
	runtime.valueRoute = func(context engine.SelectorContext, coordinate valuedomain.Coordinate, tag uint64) bool {
		return values.SelectRoute(context, coordinate, tag)
	}
	runtime.sourceRoute = func(context engine.SelectorContext, coordinate valuedomain.Coordinate, tag RawSourceTag) bool {
		return valueowner.SelectRouteTyped(values, context, coordinate, tag)
	}
	runtime.heapRoute = func(context engine.SelectorContext, key heapdomain.Key, tag heapdomain.RawRouteTag) bool {
		return heapowner.SelectRouteTyped(heap, context, key, tag)
	}
	runtime.packRoute = func(context engine.SelectorContext, root pack.Root, tag heapdomain.RawPayloadTag) bool {
		return packowner.SelectRouteTyped(packs, context, root, tag)
	}
	return runtime
}
