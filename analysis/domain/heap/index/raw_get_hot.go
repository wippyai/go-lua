package index

import (
	calldomain "github.com/wippyai/go-lua/analysis/domain/call"
	callowner "github.com/wippyai/go-lua/analysis/domain/call/owner"
	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	heapowner "github.com/wippyai/go-lua/analysis/domain/heap/owner"
	"github.com/wippyai/go-lua/analysis/domain/pack"
	packowner "github.com/wippyai/go-lua/analysis/domain/pack/owner"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
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

func (rule *RawGetHotRule) BeginReceiptCompilation(graph *engine.ReceiptGraph) (*engine.ReceiptCompilation, bool) {
	if rule == nil || rule.values == nil || rule.implementation == nil {
		return nil, false
	}
	implementation, ok := valueowner.ResolveRuleImplementationFor(rule.values, rule.implementation)
	if !ok {
		return nil, false
	}
	return engine.BeginReceiptCompilation(implementation, graph)
}

func (rule *RawGetHotRule) AttachReceiptMember(compilation *engine.ReceiptCompilation, member engine.ReceiptRuleMember, operand Access) (*engine.ReceiptMember, bool) {
	if rule == nil || rule.values == nil || rule.implementation == nil || rule.core == nil || !rule.core.owns(operand) || !operand.Read() {
		return nil, false
	}
	implementation, ok := valueowner.ResolveRuleImplementationFor(rule.values, rule.implementation)
	if !ok {
		return nil, false
	}
	return engine.AttachReceiptRuleMember(compilation, implementation, member, operand)
}

// AttachMountedOccurrence lowers one RawGet artifact row entirely from the
// exact mounted Access receipt. Selected surfaces are occurrence/operand
// anchored; no arbitrary candidate Ref or factor ordinal is chosen here.
func (rule *RawGetHotRule) AttachMountedOccurrence(assembly *engine.ReceiptAssembly, mountID, reusablePointID, occurrenceID identity.ContentID) (engine.BindingRuleRowRef, bool) {
	if rule == nil || rule.values == nil || assembly == nil {
		return engine.BindingRuleRowRef{}, false
	}
	operand, operandOK := rule.ReceiptForOccurrence(mountID, occurrenceID)
	implementation, implementationOK := valueowner.ResolveRuleImplementationFor(rule.values, rule.implementation)
	if !operandOK || !implementationOK {
		return engine.BindingRuleRowRef{}, false
	}
	occurrence, occurrenceOK := assembly.AdmitMountedRuleOccurrence(mountedCapability(rule.implementation), mountID, reusablePointID, occurrenceID)
	if !occurrenceOK {
		return engine.BindingRuleRowRef{}, false
	}
	transaction, transactionOK := engine.BeginMountedRuleAdmission(assembly, implementation, occurrence, operand)
	receiverRef, receiverOK := rule.values.Ref(operand.receiver)
	resultRef, resultOK := rule.values.Ref(operand.result)
	if !transactionOK || !receiverOK || !resultOK {
		return engine.BindingRuleRowRef{}, false
	}
	reads := make([]engine.RuleReadSurface, 0, 6)
	receiver, readOK := engine.ExactReadSurface(receiverRef)
	if !readOK || !transaction.AddRead(receiver) {
		return engine.BindingRuleRowRef{}, false
	}
	reads = append(reads, receiver)
	dependencies := [][]int{{0}, {0, 1}, {0, 1, 2}, {1, 3}, {1, 3, 4}}
	for selectedIndex, dependencyIndexes := range dependencies {
		receipt, receiptOK := implementation.SelectedReadReceipt(uint64(selectedIndex + 1))
		selectedDependencies := make([]engine.RuleReadSurface, len(dependencyIndexes))
		for index, dependencyIndex := range dependencyIndexes {
			selectedDependencies[index] = reads[dependencyIndex]
		}
		selected, selectedOK := transaction.AnchoredSelectedReadSurface(receipt, selectedDependencies)
		if !receiptOK || !selectedOK || !transaction.AddRead(selected) {
			return engine.BindingRuleRowRef{}, false
		}
		reads = append(reads, selected)
	}
	if !transaction.AddCarry() || !engine.AddExactWrite(transaction, resultRef) {
		return engine.BindingRuleRowRef{}, false
	}
	queued := assembly.QueueMountedRuleFinalizer(mountedCapability(rule.implementation), func() bool {
		source, sourceOK := transaction.Seal()
		draft, draftOK := implementation.BeginReceiptRuleRow(source)
		if !sourceOK || !draftOK {
			return false
		}
		for index := uint64(0); index < 6; index++ {
			part, ok := implementation.ReceiptReadPart(source, index)
			if !ok || !draft.AddRead(part) {
				return false
			}
		}
		carry, carryOK := implementation.ReceiptCarryPart(source, 0)
		write, writeOK := implementation.ReceiptWritePart(source, 0)
		if !carryOK || !writeOK || !draft.AddCarry(carry) || !draft.AddWrite(write) {
			return false
		}
		_, added := assembly.AddRuleFromDraft(occurrence, draft)
		return added
	})
	return engine.BindingRuleRowRef{}, queued
}

// AttachMountedReceiptMember resolves the committed RawGet member and its
// private Access receipt internally before runtime attachment.
func (rule *RawGetHotRule) AttachMountedReceiptMember(compilation *engine.ReceiptCompilation, graph *engine.ReceiptGraph, mountID, reusablePointID, occurrenceID identity.ContentID) (*engine.ReceiptMember, bool) {
	if rule == nil || graph == nil {
		return nil, false
	}
	operand, operandOK := rule.ReceiptForOccurrence(mountID, occurrenceID)
	member, memberOK := graph.MountedRuleMember(mountedCapability(rule.implementation), mountID, reusablePointID, occurrenceID)
	if !operandOK || !memberOK {
		return nil, false
	}
	return rule.AttachReceiptMember(compilation, member, operand)
}

// ReceiptForOccurrence resolves one exact mounted RawGet candidate directly
// through Heap's mount-scoped inverse. No Topology occurrence directory or
// operation-local wrapper is retained.
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
	tx, ok := valueowner.BeginSelectedRuleBinding(values, fragment.slot, fragment.carry, fragment.write, values.FactorRef(), engine.HotRuleSpec[valuedomain.Value, Access]{OperandContent: core.operandContent, Admission: engine.AdmitRuleByDerivation(fragment.evidence, core.check(fragment.semantic)), Transfer: core.transfer}, engine.HotCarrySpec[valuedomain.Value, Access]{})
	if !ok {
		return nil, false
	}
	if core.receiver, ok = valueowner.AddSelectedRuleExactRead(tx, fragment.receiver, values.FactorRef()); !ok {
		_ = valueowner.AbortSelectedRuleBinding(tx)
		return nil, false
	}
	if core.key, ok = valueowner.AddSelectedRuleOperandRead[Access, valuedomain.Value, uint64](tx, fragment.key, values.FactorRef(), core.locateKey); !ok {
		_ = valueowner.AbortSelectedRuleBinding(tx)
		return nil, false
	}
	if core.call, ok = valueowner.AddSelectedRuleOperandRead[Access, calldomain.Value, uint64](tx, fragment.call, calls.FactorRef(), core.locateCall); !ok {
		_ = valueowner.AbortSelectedRuleBinding(tx)
		return nil, false
	}
	if core.heapRead, ok = valueowner.AddSelectedRuleOperandRead[Access, heapdomain.Value, heapdomain.RawRouteTag](tx, fragment.heapRead, heap.FactorRef(), core.locateHeap); !ok {
		_ = valueowner.AbortSelectedRuleBinding(tx)
		return nil, false
	}
	if core.packRead, ok = valueowner.AddSelectedRuleOperandRead[Access, pack.Value, heapdomain.RawPayloadTag](tx, fragment.packRead, packs.FactorRef(), core.locatePack); !ok {
		_ = valueowner.AbortSelectedRuleBinding(tx)
		return nil, false
	}
	if core.sourceRead, ok = valueowner.AddSelectedRuleOperandRead[Access, valuedomain.Value, rawSourceTag](tx, fragment.sourceRead, values.FactorRef(), core.locateSource); !ok {
		_ = valueowner.AbortSelectedRuleBinding(tx)
		return nil, false
	}
	if !valueowner.CommitSelectedRuleBinding(tx) {
		return nil, false
	}
	implementation, ok := tx.Implementation()
	if !ok {
		return nil, false
	}
	return &RawGetHotRule{implementation: implementation, core: core, values: values}, true
}
