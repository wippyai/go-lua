package allocation

import (
	"github.com/wippyai/go-lua/analysis/engine"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/placement"
	placementowner "github.com/wippyai/go-lua/domain/placement/owner"
)

// HotRule is Placement/allocation's zero-input seed issuer. The owner is the
// exact immutable authority; no map or second coordinate index is retained by
// the transfer path.
type HotRule struct {
	implementation *placementowner.RuleImplementation[operand]
	owner          *placementowner.HotOwner
}

// BindHot binds the exact allocation seed fragment to Placement's projected
// Heap axis. Every mounted Program allocation occurrence resolves through
// Heap's owner-issued occurrence row and writes Stack at its exact coordinate.
func BindHot(binding *engine.SchemaBinding, fragment *SchemaFragment, owner *placementowner.HotOwner, heap heapdomain.Schema) (*HotRule, bool) {
	if binding == nil || fragment == nil || fragment.slot == nil || owner == nil || !owner.MatchesBinding(binding) || !owner.Schema().Valid() ||
		owner.Schema().Heap() != heap || !heap.Valid() ||
		!fragment.semantic.Available() {
		return nil, false
	}
	schema := owner.Schema()
	rule := &HotRule{owner: owner}
	implementation, ok := placementowner.BindExactWriteRule(owner, fragment.slot, fragment.write, engine.HotRuleSpec[placement.Placement, operand]{
		OperandContent: func(candidate operand) (operand, [32]byte, bool) {
			return allocationOperandContentForSchema(schema, candidate)
		},
		OperandResolver: rule.resolveOperand,
		Fold: func(frame engine.Frame[placement.Placement, operand]) engine.RuleResult[placement.Placement] {
			candidate, operandOK := engine.Operand(frame)
			if !operandOK {
				return engine.RuleResult[placement.Placement]{}
			}
			if _, _, contentOK := allocationOperandContentForSchema(schema, candidate); !contentOK {
				return engine.RuleResult[placement.Placement]{}
			}
			return engine.Staged(frame, placement.Stack)
		},
	}, func(candidate operand) (uint64, bool) {
		return allocationCoordinateForSchema(schema, candidate)
	})
	if !ok || implementation == nil {
		return nil, false
	}
	rule.implementation = implementation
	return rule, true
}

func (rule *HotRule) resolveOperand(coords engine.OperandCoords) (operand, bool) {
	if rule == nil || rule.owner == nil || !rule.owner.Schema().Valid() || !coords.Mount.Available() || !coords.Occurrence.Available() {
		return operand{}, false
	}
	mount, mountOK := rule.owner.Schema().Heap().OccurrenceMountForModule(coords.Mount)
	key, keyOK := mount.AllocationRootForOccurrence(coords.Occurrence)
	if !mountOK || !keyOK {
		return operand{}, false
	}
	return allocationOperandForSchema(rule.owner.Schema(), key)
}

// Implementation returns the Placement owner's opaque pending Rule issuer.
func (rule *HotRule) Implementation() (*placementowner.RuleImplementation[operand], bool) {
	if rule == nil || rule.implementation == nil {
		return nil, false
	}
	return rule.implementation, true
}

// SealProgramRule issues the engine's sealed Program rule after the shared
// SchemaBinding publishes its exact Placement implementation.
func SealProgramRule(rule *HotRule) (engine.ProgramRule, bool) {
	if rule == nil {
		return engine.ProgramRule{}, false
	}
	implementation, ok := placementowner.ResolveRuleImplementationFor(rule.owner, rule.implementation)
	if !ok {
		return engine.ProgramRule{}, false
	}
	return engine.SealProgramRule(implementation)
}
