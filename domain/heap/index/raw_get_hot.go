package index

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
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
	implementation *valueowner.RuleImplementation[Access]
	core           *RawGetRule
	values         *valueowner.HotOwner
}

func (rule *RawGetRule) operandContent(access Access) (Access, [32]byte, bool) {
	if rule == nil || !rule.owns(access) {
		return Access{}, [32]byte{}, false
	}
	return access, [32]byte(access.id), true
}

func (rule *RawGetHotRule) Implementation() (*valueowner.RuleImplementation[Access], bool) {
	if rule == nil || rule.values == nil || rule.implementation == nil {
		return nil, false
	}
	_, ok := valueowner.ResolveRuleImplementationFor(rule.values, rule.implementation)
	return rule.implementation, ok
}

// SealProgramRule is this typed rule's schema registration.
func SealRawGetProgramRule(rule *RawGetHotRule) (engine.ProgramRule, bool) {
	if rule == nil {
		return engine.ProgramRule{}, false
	}
	implementation, ok := valueowner.ResolveRuleImplementationFor(rule.values, rule.implementation)
	if !ok {
		return engine.ProgramRule{}, false
	}
	return engine.SealProgramRule(implementation)
}

func (rule *RawGetHotRule) ReceiptForOccurrence(module, occurrenceID identity.ContentID) (Access, bool) {
	if rule == nil || rule.core == nil || rule.core.runtime == nil || rule.core.runtime.topology == nil {
		return Access{}, false
	}
	topology := rule.core.runtime.topology
	mount, mountOK := topology.heap.OccurrenceMountForModule(module)
	if !mountOK {
		return Access{}, false
	}
	indexAccess, accessOK := mount.IndexAccessForOccurrence(occurrenceID, true)
	if !accessOK {
		return Access{}, false
	}
	access, accessOK := topology.Access(indexAccess)
	if !accessOK {
		return Access{}, false
	}
	return access, access.Read()
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
	runtime.visitRawRoute = topology.heap.VisitRawAccessRoute
	runtime.selectorForSlot = topology.heap.SelectorForSlot
	runtime.callRoute = func(context engine.SelectorContext, key calldomain.Key, tag uint64) bool {
		return calls.SelectRoute(context, key, tag)
	}
	runtime.valueRoute = func(context engine.SelectorContext, coordinate valuedomain.Coordinate, tag uint64) bool {
		return values.SelectRoute(context, coordinate, tag)
	}
	runtime.heapRoute = func(context engine.SelectorContext, key heapdomain.Key, tag heapdomain.RawRouteTag) bool {
		return heap.SelectRoute(context, key, tag)
	}
	runtime.packRoute = func(context engine.SelectorContext, root pack.Root, tag heapdomain.RawPayloadTag) bool {
		return packs.SelectRoute(context, root, uint64(tag))
	}
	runtime.valueTarget = func(target engine.RuleTarget, coordinate valuedomain.Coordinate) bool {
		return values.TargetMatches(target, coordinate)
	}
	runtime.valueReadRef = func(derivation engine.RuleDerivation[valuedomain.Value, Access], read engine.Read[engine.OrderedCells[valuedomain.Value]], coordinate valuedomain.Coordinate) bool {
		return valueowner.ReadMatches(values, derivation, read, coordinate)
	}
	runtime.valueSelectionRef = func(derivation engine.RuleDerivation[valuedomain.Value, Access], disposition engine.RuleDisposition[valuedomain.Value], read engine.Read[engine.Selection[uint64, engine.OrderedCells[valuedomain.Value]]], ordinal int, coordinate valuedomain.Coordinate) bool {
		return valueowner.SelectionMatches(values, derivation, disposition, read, ordinal, coordinate)
	}
	runtime.callSelectionRef = func(derivation engine.RuleDerivation[valuedomain.Value, Access], disposition engine.RuleDisposition[valuedomain.Value], read engine.Read[engine.Selection[uint64, engine.OrderedCells[calldomain.Value]]], ordinal int, tag uint64) bool {
		key, ok := topology.CallKeyForTag(tag)
		if !ok {
			return false
		}
		ref, ok := calls.Ref(key)
		return ok && engine.DerivationDispositionSelectionMatchesRef(derivation, disposition, read, ordinal, ref)
	}
	runtime.heapSelectionRef = func(derivation engine.RuleDerivation[valuedomain.Value, Access], disposition engine.RuleDisposition[valuedomain.Value], read engine.Read[engine.Selection[heapdomain.RawRouteTag, engine.OrderedCells[heapdomain.Value]]], ordinal int, key heapdomain.Key) bool {
		ref, ok := heap.Ref(key)
		return ok && engine.DerivationDispositionSelectionMatchesRef(derivation, disposition, read, ordinal, ref)
	}
	runtime.packSelectionRef = func(derivation engine.RuleDerivation[valuedomain.Value, Access], disposition engine.RuleDisposition[valuedomain.Value], read engine.Read[engine.Selection[heapdomain.RawPayloadTag, engine.OrderedCells[pack.Value]]], ordinal int, root pack.Root) bool {
		ref, ok := packs.Ref(root)
		return ok && engine.DerivationDispositionSelectionMatchesRef(derivation, disposition, read, ordinal, ref)
	}
	runtime.sourceSelectionRef = func(derivation engine.RuleDerivation[valuedomain.Value, Access], disposition engine.RuleDisposition[valuedomain.Value], read engine.Read[engine.Selection[rawSourceTag, engine.OrderedCells[valuedomain.Value]]], ordinal int, coordinate valuedomain.Coordinate) bool {
		return valueowner.SelectionMatches(values, derivation, disposition, read, ordinal, coordinate)
	}
	runtime.callKeyForTag = topology.CallKeyForTag
	core := &RawGetRule{runtime: runtime}
	core.scratch.New = func() any {
		return &rawGetScratch{payload: make([]uint64, bitWords(len(topology.catalog.payloads)-1)), source: make([]uint64, bitWords(len(topology.catalog.sources)))}
	}
	core.scratch.Put(core.scratch.New())
	var implementation *valueowner.RuleImplementation[Access]
	bound := valueowner.BindSelectedRule(values, fragment.slot, fragment.carry, fragment.write, values.FactorRef(), engine.HotRuleSpec[valuedomain.Value, Access]{OperandContent: core.operandContent, Admission: engine.AdmitRuleByDerivation(fragment.evidence, core.check(fragment.semantic)), Transfer: core.transfer}, engine.HotCarrySpec[valuedomain.Value, Access]{}, func(access Access) (uint64, bool) {
		result, resultOK := access.Result()
		index, indexOK := values.Schema().CoordinateIndex(result)
		return uint64(index), resultOK && indexOK
	}, func(tx *valueowner.SelectedRuleBinding[Access]) bool {
		var ok bool
		if core.receiver, ok = valueowner.AddSelectedRuleExactRead(tx, fragment.receiver, values.FactorRef(), func(access Access) (uint64, bool) {
			receiver, receiverOK := access.Receiver()
			index, indexOK := values.Schema().CoordinateIndex(receiver)
			return uint64(index), receiverOK && indexOK
		}); !ok {
			return false
		}
		if core.key, ok = valueowner.AddSelectedRuleOperandRead[Access, valuedomain.Value, uint64](tx, fragment.key, values.FactorRef(), core.locateKey); !ok {
			return false
		}
		if core.call, ok = valueowner.AddSelectedRuleOperandRead[Access, calldomain.Value, uint64](tx, fragment.call, calls.FactorRef(), core.locateCall); !ok {
			return false
		}
		if core.heapRead, ok = valueowner.AddSelectedRuleOperandRead[Access, heapdomain.Value, heapdomain.RawRouteTag](tx, fragment.heapRead, heap.FactorRef(), core.locateHeap); !ok {
			return false
		}
		if core.packRead, ok = valueowner.AddSelectedRuleOperandRead[Access, pack.Value, heapdomain.RawPayloadTag](tx, fragment.packRead, packs.FactorRef(), core.locatePack); !ok {
			return false
		}
		if core.sourceRead, ok = valueowner.AddSelectedRuleOperandRead[Access, valuedomain.Value, rawSourceTag](tx, fragment.sourceRead, values.FactorRef(), core.locateSource); !ok {
			return false
		}
		implementation, ok = tx.Implementation()
		return ok
	})
	if !bound {
		return nil, false
	}
	rule := &RawGetHotRule{implementation: implementation, core: core, values: values}
	if !implementation.InstallOperandResolver(rule.resolveOperand) {
		return nil, false
	}
	return rule, true
}

func (rule *RawGetHotRule) resolveOperand(coords engine.OperandCoords) (Access, bool) {
	return rule.ReceiptForOccurrence(coords.Mount, coords.Occurrence)
}
