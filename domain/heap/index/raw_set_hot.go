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

// RawSetHotRule is the receipt-native RawSet issuer. The reducer remains the
// one shared transfer/evidence plane; this wrapper only assembles exact
// Schema read receipts and the Heap-owned route transaction.
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

// Implementation returns the exact Heap-owned pending issuer; resolution is
// fenced until the enclosing SchemaBinding seals.
func (rule *RawSetHotRule) Implementation() (*heapowner.RuleImplementation[Access], bool) {
	if rule == nil || rule.heap == nil || rule.implementation == nil {
		return nil, false
	}
	_, ok := heapowner.ResolveRuleImplementationFor(rule.heap, rule.implementation)
	return rule.implementation, ok
}

func (rule *RawSetHotRule) ProgramDeclaration() (engine.RuleProgramDeclaration, bool) {
	return heapowner.ResolveRuleImplementationFor(rule.heap, rule.implementation)
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
// through one owner-native transaction. No cold Owner, Composition Rule, or
// copied Factor geometry is retained.
func BindRawSetHot(binding *engine.SchemaBinding, fragment *RawSetSchemaFragment, topology *Topology, values *valueowner.HotOwner, heap *heapowner.HotOwner, packs *packowner.HotOwner) (*RawSetHotRule, bool) {
	if binding == nil || fragment == nil || fragment.slot == nil || topology == nil || !topology.valid() || values == nil || !values.MatchesBinding(binding) || heap == nil || !heap.MatchesBinding(binding) || packs == nil || !packs.MatchesBinding(binding) || !packs.OwnsSchema(topology.packs) || fragment.semantic == (identity.SemanticKey{}) || fragment.evidence == (identity.SemanticKey{}) || values.Schema() != topology.values || heap.Schema() != topology.heap || !topology.packs.LinkOwner().Matches(values.Schema().LinkOwner()) {
		return nil, false
	}
	core := &RawSetRule{topology: topology}
	core.runtime = hotRawSetRuntime(values, heap, packs)
	core.scratch.New = func() any { return &rawSetScratch{} }
	core.scratch.Put(&rawSetScratch{})

	var implementation *heapowner.RuleImplementation[Access]
	bound := heapowner.BindSelectedRouteRule(heap, fragment.slot, fragment.carry, fragment.write, fragment.heapRef, engine.HotRuleSpec[heapdomain.Value, Access]{
		OperandContent: core.operandContent,
		Admission:      engine.AdmitRuleByDerivation(fragment.evidence, core.check(fragment.semantic)),
		Transfer:       core.transfer,
	}, engine.HotCarrySpec[heapdomain.Value, Access]{}, nil, func(tx *heapowner.SelectedRouteRuleBinding[Access]) bool {
		var ok bool
		if core.receiver, ok = heapowner.AddExactRead(tx, fragment.receiver, fragment.valueRef, func(access Access) (uint64, bool) {
			receiver, receiverOK := access.Receiver()
			index, indexOK := values.Schema().CoordinateIndex(receiver)
			return uint64(index), receiverOK && indexOK
		}); !ok {
			return false
		}
		if core.key, ok = heapowner.AddOperandSelectedRead[Access, valuedomain.Value, uint64](tx, fragment.key, fragment.valueRef, core.locateKey); !ok {
			return false
		}
		if core.heapRead, ok = heapowner.AddOperandSelectedRead[Access, heapdomain.Value, heapdomain.RawRouteTag](tx, fragment.heapRead, fragment.heapRef, core.locateHeap); !ok {
			return false
		}
		if core.packRead, ok = heapowner.AddOperandSelectedRead[Access, pack.Value, heapdomain.RawPayloadTag](tx, fragment.packRead, fragment.packRef, core.locatePack); !ok {
			return false
		}
		if core.source, ok = heapowner.AddOperandSelectedRead[Access, valuedomain.Value, rawSourceTag](tx, fragment.sourceRead, fragment.valueRef, core.locateSource); !ok {
			return false
		}
		implementation, ok = tx.Implementation()
		return ok && implementation != nil
	})
	if !bound {
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
