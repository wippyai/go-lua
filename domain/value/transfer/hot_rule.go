package transfer

import (
	"github.com/wippyai/go-lua/analysis/engine"
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
		!fragment.semantic.Available() {
		return nil, false
	}
	var runtimeRead engine.Read[engine.OrderedCells[value.Value]]
	rule := &HotRule{owner: owner}
	implementation, read, ok := valueowner.BindExactReadAndCarryRule(owner, fragment.slot, fragment.read, fragment.carry, fragment.write, engine.HotRuleSpec[value.Value, value.StorageTransfer]{
		OperandContent: func(transfer value.StorageTransfer) (value.StorageTransfer, [32]byte, bool) {
			return hotStorageTransferContent(owner.Schema(), transfer)
		},
		OperandResolver: rule.resolveOperand,
		Fold: func(frame engine.Frame[value.Value, value.StorageTransfer]) engine.RuleResult[value.Value] {
			transfer, operandOK := engine.Operand(frame)
			if !operandOK {
				return engine.RuleResult[value.Value]{}
			}
			_, _, endpointsOK := hotStorageTransferEndpoints(owner.Schema(), transfer)
			if !endpointsOK {
				return engine.RuleResult[value.Value]{}
			}
			cells, readOK := engine.ReadValue(frame, runtimeRead)
			if !readOK || cells.Count() != 1 {
				return engine.RuleResult[value.Value]{}
			}
			fact, present, available := cells.At(0)
			if !available {
				return engine.RuleResult[value.Value]{}
			}
			if !present {
				return engine.NoCandidate(frame)
			}
			return engine.Staged(frame, fact)
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
	rule.implementation, rule.read = implementation, read
	return rule, true
}

func (rule *HotRule) resolveOperand(coords engine.OperandCoords) (value.StorageTransfer, bool) {
	if rule == nil || rule.owner == nil || rule.owner.Schema() == nil || !coords.Mount.Available() || !coords.Occurrence.Available() {
		return value.StorageTransfer{}, false
	}
	transfer, ok := rule.owner.Schema().StorageTransferForArtifactOccurrence(coords.Mount, coords.Occurrence)
	return transfer, ok && rule.owner.Schema().OwnsStorageTransfer(transfer)
}

// Implementation resolves only after the shared SchemaBinding seals.
func (rule *HotRule) Implementation() (*valueowner.RuleImplementation[value.StorageTransfer], bool) {
	if rule == nil || rule.implementation == nil {
		return nil, false
	}
	_, ok := valueowner.ResolveRuleImplementation(rule.implementation)
	return rule.implementation, ok
}

// SealProgramRule is this typed rule's schema registration.
func SealProgramRule(rule *HotRule) (engine.ProgramRule, bool) {
	if rule == nil {
		return engine.ProgramRule{}, false
	}
	implementation, ok := valueowner.ResolveRuleImplementationFor(rule.owner, rule.implementation)
	if !ok {
		return engine.ProgramRule{}, false
	}
	return engine.SealProgramRule(implementation)
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
