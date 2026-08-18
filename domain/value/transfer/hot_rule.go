package transfer

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
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
	}, engine.HotCarrySpec[value.Value, value.StorageTransfer]{}, func(transfer value.StorageTransfer) (uint64, bool) {
		from, _, ok := hotStorageTransferEndpoints(owner.Schema(), transfer)
		index, indexOK := owner.Schema().CoordinateIndex(from)
		return uint64(index), ok && indexOK
	}, func(transfer value.StorageTransfer) (uint64, bool) {
		_, to, ok := hotStorageTransferEndpoints(owner.Schema(), transfer)
		index, indexOK := owner.Schema().CoordinateIndex(to)
		return uint64(index), ok && indexOK
	})
	if !ok || implementation == nil {
		return nil, false
	}
	runtimeRead = read
	rule := &HotRule{implementation: implementation, read: read, owner: owner}
	if !implementation.InstallOperandResolver(rule.resolveOperand) {
		return nil, false
	}
	return rule, true
}

func (rule *HotRule) resolveOperand(coords engine.OperandCoords) (value.StorageTransfer, bool) {
	return rule.ReceiptForOccurrence(coords.Mount, coords.Occurrence)
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

func (rule *HotRule) ProgramAttach() (engine.RuleProgramAttach, bool) {
	return valueowner.ResolveRuleImplementationFor(rule.owner, rule.implementation)
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

func hotTransferChecker(owner *valueowner.HotOwner, semantic identity.SemanticKey, read *engine.Read[engine.OrderedCells[value.Value]]) engine.RuleDerivationChecker[value.Value, value.StorageTransfer] {
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
