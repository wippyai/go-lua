package index

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
	"github.com/wippyai/go-lua/domain/pack"
	packowner "github.com/wippyai/go-lua/domain/pack/owner"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// RawSetHotRule is the direct RawSet issuer. The reducer remains the one
// shared transfer/evidence plane; this type only retains owner-typed reads
// and the sealed Heap rule implementation.
type RawSetHotRule struct {
	implementation *heapowner.RuleImplementation[Access]
	core           *RawSetRule
	values         *valueowner.HotOwner
	heap           *heapowner.HotOwner
}

func (rule *RawSetRule) operandContent(access Access) (Access, [32]byte, bool) {
	if rule == nil || !rule.owns(access) {
		return Access{}, [32]byte{}, false
	}
	return access, [32]byte(access.id), true
}

// Implementation returns the exact Heap-owned issuer after the enclosing
// SchemaBinding seals.
func (rule *RawSetHotRule) Implementation() (*heapowner.RuleImplementation[Access], bool) {
	if rule == nil || rule.heap == nil || rule.implementation == nil {
		return nil, false
	}
	_, ok := heapowner.ResolveRuleImplementationFor(rule.heap, rule.implementation)
	return rule.implementation, ok
}

// SealProgramRule is this typed rule's schema registration.
func SealRawSetProgramRule(rule *RawSetHotRule) (engine.ProgramRule, bool) {
	if rule == nil {
		return engine.ProgramRule{}, false
	}
	implementation, ok := heapowner.ResolveRuleImplementationFor(rule.heap, rule.implementation)
	if !ok {
		return engine.ProgramRule{}, false
	}
	return engine.SealProgramRule(implementation)
}

func (rule *RawSetHotRule) ReceiptForOccurrence(module, occurrenceID identity.ContentID) (Access, bool) {
	if rule == nil || rule.core == nil || rule.core.topology == nil {
		return Access{}, false
	}
	topology := rule.core.topology
	mount, mountOK := topology.heap.OccurrenceMountForModule(module)
	if !mountOK {
		return Access{}, false
	}
	indexAccess, accessOK := mount.IndexAccessForOccurrence(occurrenceID, false)
	if !accessOK {
		return Access{}, false
	}
	access, accessOK := topology.Access(indexAccess)
	if !accessOK {
		return Access{}, false
	}
	return access, access.Write()
}

// BindRawSetHot binds RawSet's r0..r4 read chain, Heap carry, and route write
// directly at their declared schema ordinals. No construction transaction,
// cold Owner, Composition Rule, or copied Factor geometry is retained.
func BindRawSetHot(binding *engine.SchemaBinding, fragment *RawSetSchemaFragment, topology *Topology, values *valueowner.HotOwner, heap *heapowner.HotOwner, packs *packowner.HotOwner) (*RawSetHotRule, bool) {
	if binding == nil || fragment == nil || fragment.slot == nil || topology == nil || !topology.valid() || values == nil || !values.MatchesBinding(binding) || heap == nil || !heap.MatchesBinding(binding) || packs == nil || !packs.MatchesBinding(binding) || !packs.OwnsSchema(topology.packs) || fragment.semantic == (identity.SemanticKey{}) || fragment.evidence == (identity.SemanticKey{}) || values.Schema() != topology.values || heap.Schema() != topology.heap || !topology.packs.LinkOwner().Matches(values.Schema().LinkOwner()) {
		return nil, false
	}
	core := &RawSetRule{topology: topology}
	core.runtime = hotRawSetRuntime(values, heap, packs)
	core.scratch.New = func() any { return &rawSetScratch{} }
	core.scratch.Put(&rawSetScratch{})

	implementation, bound := heapowner.BindSelectedRouteRuleDirect(heap, fragment.slot, fragment.carry, fragment.write, fragment.heapRef, engine.HotRuleSpec[heapdomain.Value, Access]{
		OperandContent: core.operandContent,
		Admission:      engine.AdmitRuleByDerivation(fragment.evidence, core.check(fragment.semantic)),
		Transfer:       core.transfer,
	}, engine.HotCarrySpec[heapdomain.Value, Access]{}, nil)
	if !bound {
		return nil, false
	}
	var ok bool
	if core.receiver, ok = heapowner.AddSelectedRouteRuleDirectExactRead(implementation, fragment.receiver, fragment.valueRef, func(access Access) (uint64, bool) {
		receiver, receiverOK := access.Receiver()
		index, indexOK := values.Schema().CoordinateIndex(receiver)
		return uint64(index), receiverOK && indexOK
	}); !ok {
		return nil, false
	}
	if core.key, ok = heapowner.AddSelectedRouteRuleDirectOperandRead[Access, valuedomain.Value, uint64](implementation, fragment.key, fragment.valueRef, core.locateKey); !ok {
		return nil, false
	}
	if core.heapRead, ok = heapowner.AddSelectedRouteRuleDirectOperandRead[Access, heapdomain.Value, heapdomain.RawRouteTag](implementation, fragment.heapRead, fragment.heapRef, core.locateHeap); !ok {
		return nil, false
	}
	if core.packRead, ok = heapowner.AddSelectedRouteRuleDirectOperandRead[Access, pack.Value, heapdomain.RawPayloadTag](implementation, fragment.packRead, fragment.packRef, core.locatePack); !ok {
		return nil, false
	}
	if core.source, ok = heapowner.AddSelectedRouteRuleDirectOperandRead[Access, valuedomain.Value, rawSourceTag](implementation, fragment.sourceRead, fragment.valueRef, core.locateSource); !ok {
		return nil, false
	}
	rule := &RawSetHotRule{implementation: implementation, core: core, values: values, heap: heap}
	if !implementation.InstallOperandResolver(rule.resolveOperand) {
		return nil, false
	}
	return rule, true
}

func (rule *RawSetHotRule) resolveOperand(coords engine.OperandCoords) (Access, bool) {
	return rule.ReceiptForOccurrence(coords.Mount, coords.Occurrence)
}

func hotRawSetRuntime(values *valueowner.HotOwner, heap *heapowner.HotOwner, packs *packowner.HotOwner) *rawSetRuntime {
	runtime := &rawSetRuntime{values: values.Schema(), heap: heap.Schema()}
	runtime.valueRoute = func(context engine.SelectorContext, coordinate valuedomain.Coordinate, tag uint64) bool {
		return values.SelectRoute(context, coordinate, tag)
	}
	runtime.sourceRoute = func(context engine.SelectorContext, coordinate valuedomain.Coordinate, tag rawSourceTag) bool {
		return valueowner.SelectRouteTyped(values, context, coordinate, tag)
	}
	runtime.heapRoute = func(context engine.SelectorContext, key heapdomain.Key, tag heapdomain.RawRouteTag) bool {
		return heapowner.SelectRouteTyped(heap, context, key, tag)
	}
	runtime.packRoute = func(context engine.SelectorContext, root pack.Root, tag heapdomain.RawPayloadTag) bool {
		return packowner.SelectRouteTyped(packs, context, root, tag)
	}
	runtime.valueTarget = func(target engine.RuleTarget, coordinate valuedomain.Coordinate) bool {
		return values.TargetMatches(target, coordinate)
	}
	runtime.heapTarget = func(target engine.RuleTarget, key heapdomain.Key) bool { return heap.TargetMatches(target, key) }
	runtime.valueReadRef = func(derivation engine.RuleDerivation[heapdomain.Value, Access], read engine.Read[engine.OrderedCells[valuedomain.Value]], coordinate valuedomain.Coordinate) bool {
		return valueowner.ReadMatches(values, derivation, read, coordinate)
	}
	runtime.valueSelectionRef = func(derivation engine.RuleDerivation[heapdomain.Value, Access], disposition engine.RuleDisposition[heapdomain.Value], read engine.Read[engine.Selection[uint64, engine.OrderedCells[valuedomain.Value]]], ordinal int, coordinate valuedomain.Coordinate) bool {
		return valueowner.SelectionMatches(values, derivation, disposition, read, ordinal, coordinate)
	}
	runtime.sourceSelectionRef = func(derivation engine.RuleDerivation[heapdomain.Value, Access], disposition engine.RuleDisposition[heapdomain.Value], read engine.Read[engine.Selection[rawSourceTag, engine.OrderedCells[valuedomain.Value]]], ordinal int, coordinate valuedomain.Coordinate) bool {
		return valueowner.SelectionMatches(values, derivation, disposition, read, ordinal, coordinate)
	}
	runtime.heapSelectionRef = func(derivation engine.RuleDerivation[heapdomain.Value, Access], disposition engine.RuleDisposition[heapdomain.Value], read engine.Read[engine.Selection[heapdomain.RawRouteTag, engine.OrderedCells[heapdomain.Value]]], ordinal int, key heapdomain.Key) bool {
		return heapowner.SelectionMatches(heap, derivation, disposition, read, ordinal, key)
	}
	runtime.packSelectionRef = func(derivation engine.RuleDerivation[heapdomain.Value, Access], disposition engine.RuleDisposition[heapdomain.Value], read engine.Read[engine.Selection[heapdomain.RawPayloadTag, engine.OrderedCells[pack.Value]]], ordinal int, root pack.Root) bool {
		return packowner.SelectionMatches(packs, derivation, disposition, read, ordinal, root)
	}
	return runtime
}
