package transfer

import (
	"github.com/wippyai/go-lua/analysis/domain/value"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
)

// HotRule is the receipt-native fixed-storage transfer. The Value owner keeps
// the Factor and coordinate authority; this package keeps only the exact Rule
// issuer and its engine-issued read capability.
type HotRule struct {
	implementation *valueowner.RuleImplementation[value.StorageTransfer]
	read           engine.Read[engine.OrderedCells[value.Value]]
	owner          *valueowner.HotOwner
}

// BindHot attaches the canonical one-input exact read, ordinary carry, and
// exact write semantics to the callback-free transfer fragment. Storage
// endpoints are consumed from Value's sealed StorageTransfer proof; the hot
// path never reopens Link or Flow topology.
func BindHot(fragment *SchemaFragment, owner *valueowner.HotOwner) (*HotRule, bool) {
	if fragment == nil || fragment.slot == nil || owner == nil || owner.Schema() == nil ||
		!fragment.semantic.Available() || !fragment.evidence.Available() || fragment.semantic == fragment.evidence {
		return nil, false
	}
	var runtimeRead engine.Read[engine.OrderedCells[value.Value]]
	implementation, read, ok := valueowner.BindExactReadAndCarryRule(owner, fragment.slot, fragment.read, fragment.carry, fragment.write, engine.HotRuleSpec[value.Value, value.StorageTransfer]{
		OperandContent: func(transfer value.StorageTransfer) (value.StorageTransfer, [32]byte, bool) {
			return hotStorageTransferContent(owner.Schema(), transfer)
		},
		Admission: engine.AdmitRuleByDerivation(fragment.evidence, hotTransferChecker(owner, fragment.semantic, &runtimeRead)),
		Transfer: func(access engine.Access[value.Value, value.StorageTransfer]) bool {
			transfer, operandOK := engine.Operand(access)
			if !operandOK {
				return false
			}
			_, _, endpointsOK := hotStorageTransferEndpoints(owner.Schema(), transfer)
			if !endpointsOK {
				return false
			}
			return engine.Product(access, func(row engine.Row) bool {
				cells, readOK := engine.ReadValue(access, row, runtimeRead)
				if !readOK || cells.Count() != 1 {
					return false
				}
				fact, present, available := cells.At(0)
				if !available {
					return false
				}
				if !present {
					return engine.NoCandidate(access, row)
				}
				return engine.StageValue(access, row, fact)
			})
		},
	}, engine.HotCarrySpec[value.Value, value.StorageTransfer]{})
	if !ok || implementation == nil {
		return nil, false
	}
	runtimeRead = read
	return &HotRule{implementation: implementation, read: read, owner: owner}, true
}

// ReceiptForOccurrence returns the exact preissued Value storage-transfer
// operand for one mounted reusable Program occurrence. The mount qualifier is
// required because equal Program artifacts may be installed more than once.
func (rule *HotRule) ReceiptForOccurrence(mount, id identity.ContentID) (value.StorageTransfer, bool) {
	if rule == nil || rule.owner == nil || rule.owner.Schema() == nil || !mount.Available() || !id.Available() {
		return value.StorageTransfer{}, false
	}
	transfer, ok := rule.owner.Schema().StorageTransferForArtifactOccurrence(mount, id)
	return transfer, ok && rule.owner.Schema().OwnsStorageTransfer(transfer)
}

// AttachMountedRule admits one complete ValueStorageTransfer row before
// topology commit. Read/carry/write surfaces use only exact Value-owner Refs.
func (rule *HotRule) AttachMountedRule(assembly *engine.ReceiptAssembly, mountID, pointID, occurrenceID identity.ContentID) (engine.BindingRuleRowRef, bool) {
	if rule == nil || rule.owner == nil || assembly == nil {
		return engine.BindingRuleRowRef{}, false
	}
	transfer, transferOK := rule.ReceiptForOccurrence(mountID, occurrenceID)
	implementation, implementationOK := valueowner.ResolveRuleImplementationFor(rule.owner, rule.implementation)
	capability := mountedCapability(rule.implementation)
	occurrence, occurrenceOK := assembly.AdmitMountedRuleOccurrence(capability, mountID, pointID, occurrenceID)
	from, to, endpointsOK := hotStorageTransferEndpoints(rule.owner.Schema(), transfer)
	fromRef, fromOK := rule.owner.Ref(from)
	toRef, toOK := rule.owner.Ref(to)
	if !transferOK || !implementationOK || !occurrenceOK || !endpointsOK || !fromOK || !toOK {
		return engine.BindingRuleRowRef{}, false
	}
	transaction, transactionOK := engine.BeginMountedRuleAdmission(assembly, implementation, occurrence, transfer)
	if !transactionOK || !engine.AddExactRead(transaction, fromRef) || !transaction.AddCarry() || !engine.AddExactWrite(transaction, toRef) {
		return engine.BindingRuleRowRef{}, false
	}
	queued := assembly.QueueMountedRuleFinalizer(capability, func() bool {
		source, sourceOK := transaction.Seal()
		draft, draftOK := implementation.BeginReceiptRuleRow(source)
		readPart, readPartOK := implementation.ReceiptReadPart(source, 0)
		carryPart, carryPartOK := implementation.ReceiptCarryPart(source, 0)
		writePart, writePartOK := implementation.ReceiptWritePart(source, 0)
		if !sourceOK || !draftOK || !readPartOK || !carryPartOK || !writePartOK || !draft.AddRead(readPart) || !draft.AddCarry(carryPart) || !draft.AddWrite(writePart) {
			return false
		}
		_, added := assembly.AddRuleFromDraft(occurrence, draft)
		return added
	})
	return engine.BindingRuleRowRef{}, queued
}

// BeginReceiptCompilation starts the opaque graph attachment transaction for
// this exact Value/storage-transfer issuer.
func (rule *HotRule) BeginReceiptCompilation(graph *engine.ReceiptGraph) (*engine.ReceiptCompilation, bool) {
	if rule == nil || rule.owner == nil {
		return nil, false
	}
	implementation, ok := valueowner.ResolveRuleImplementationFor(rule.owner, rule.implementation)
	if !ok {
		return nil, false
	}
	return engine.BeginReceiptCompilation(implementation, graph)
}

// AttachReceiptMember attaches one graph-owned transfer member with the
// exact owner-fenced transfer operand.
func (rule *HotRule) AttachReceiptMember(compilation *engine.ReceiptCompilation, member engine.ReceiptRuleMember, transfer value.StorageTransfer) (*engine.ReceiptMember, bool) {
	if rule == nil || rule.owner == nil || rule.owner.Schema() == nil || !rule.owner.Schema().OwnsStorageTransfer(transfer) {
		return nil, false
	}
	implementation, ok := valueowner.ResolveRuleImplementationFor(rule.owner, rule.implementation)
	if !ok {
		return nil, false
	}
	return engine.AttachReceiptRuleMember(compilation, implementation, member, transfer)
}

// AttachMountedReceiptMember resolves the graph-owned mounted member and the
// exact transfer operand internally, then delegates to AttachReceiptMember.
func (rule *HotRule) AttachMountedReceiptMember(compilation *engine.ReceiptCompilation, graph *engine.ReceiptGraph, mountID, pointID, occurrenceID identity.ContentID) (*engine.ReceiptMember, bool) {
	if rule == nil || graph == nil {
		return nil, false
	}
	member, memberOK := graph.MountedRuleMember(mountedCapability(rule.implementation), mountID, pointID, occurrenceID)
	transfer, transferOK := rule.ReceiptForOccurrence(mountID, occurrenceID)
	if !memberOK || !transferOK {
		return nil, false
	}
	return rule.AttachReceiptMember(compilation, member, transfer)
}

func mountedCapability(issuer interface {
	MountedCapability() (engine.RuleSlotCapability, bool)
}) engine.RuleSlotCapability {
	capability, _ := issuer.MountedCapability()
	return capability
}

// Implementation resolves only after the shared SchemaBinding seals.
func (rule *HotRule) Implementation() (*valueowner.RuleImplementation[value.StorageTransfer], bool) {
	if rule == nil || rule.implementation == nil {
		return nil, false
	}
	_, ok := valueowner.ResolveRuleImplementation(rule.implementation)
	return rule.implementation, ok
}

func hotStorageTransferContent(schema *value.Schema, transfer value.StorageTransfer) (value.StorageTransfer, [32]byte, bool) {
	id, ok := transfer.ID()
	if schema == nil || !schema.OwnsStorageTransfer(transfer) || !ok || [32]byte(id) == ([32]byte{}) {
		return value.StorageTransfer{}, [32]byte{}, false
	}
	return transfer, [32]byte(id), true
}

func hotStorageTransferEndpoints(schema *value.Schema, transfer value.StorageTransfer) (value.Coordinate, value.Coordinate, bool) {
	if schema == nil || !schema.OwnsStorageTransfer(transfer) {
		return value.Coordinate{}, value.Coordinate{}, false
	}
	from, to, ok := transfer.Endpoints()
	if !ok {
		return value.Coordinate{}, value.Coordinate{}, false
	}
	if _, ok := schema.CoordinateIndex(from); !ok {
		return value.Coordinate{}, value.Coordinate{}, false
	}
	if _, ok := schema.CoordinateIndex(to); !ok {
		return value.Coordinate{}, value.Coordinate{}, false
	}
	return from, to, true
}

func hotTransferChecker(owner *valueowner.HotOwner, semantic engine.SemanticKey, read *engine.Read[engine.OrderedCells[value.Value]]) engine.RuleDerivationChecker[value.Value, value.StorageTransfer] {
	return func(derivation engine.RuleDerivation[value.Value, value.StorageTransfer]) (engine.RuleEvidence, bool) {
		if owner == nil || owner.Schema() == nil || read == nil || derivation.Rule() != semantic || derivation.InputCount() != 1 || derivation.ReadCount() != 1 || derivation.DispositionCount() == 0 {
			return engine.RuleEvidence{}, false
		}
		transfer, operandOK := derivation.Operand()
		canonical, digest, contentOK := hotStorageTransferContent(owner.Schema(), transfer)
		from, to, endpointsOK := hotStorageTransferEndpoints(owner.Schema(), canonical)
		input, inputOK := derivation.InputAt(0)
		if !operandOK || !contentOK || !endpointsOK || !derivation.OperandContentMatches(digest) || !inputOK || input.Guard().Empty() ||
			!valueowner.ReadMatches(owner, derivation, *read, from) {
			return engine.RuleEvidence{}, false
		}
		for index := 0; index < derivation.DispositionCount(); index++ {
			disposition, dispositionOK := derivation.DispositionAt(index)
			if !dispositionOK || disposition.Guard().Empty() {
				return engine.RuleEvidence{}, false
			}
			cells, cellsOK := engine.DerivationDispositionReadValue(derivation, disposition, *read)
			if !cellsOK || cells.Count() != 1 {
				return engine.RuleEvidence{}, false
			}
			fact, present, available := cells.At(0)
			if !available {
				return engine.RuleEvidence{}, false
			}
			if !present {
				if disposition.Kind() != engine.RuleDispositionNoCandidate || disposition.TargetCount() != 0 {
					return engine.RuleEvidence{}, false
				}
				continue
			}
			if disposition.Kind() != engine.RuleDispositionStaged || disposition.TargetCount() != 1 {
				return engine.RuleEvidence{}, false
			}
			target, targetOK := disposition.TargetAt(0)
			actual, valueOK := disposition.Value()
			if !targetOK || !valueOK || !owner.TargetMatches(target, to) || !owner.Schema().Equal(actual, fact) {
				return engine.RuleEvidence{}, false
			}
		}
		return derivation.Accept()
	}
}
