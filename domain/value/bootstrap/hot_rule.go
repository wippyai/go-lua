package bootstrap

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// HotRule is Value/bootstrap's receipt-native GlobalBinding Rule issuer. It
// retains no host mapping or Value coordinate; those remain owner/schema
// authorities consulted by the typed callbacks.
type HotRule struct {
	implementation *valueowner.RuleImplementation[identity.ContentID]
	owner          *valueowner.HotOwner
}

// BindHot binds one exact bootstrap fragment to Value's exact Factor owner.
// All malformed binding, host mapping, and target initial-value cases fail at
// the typed callbacks; no legacy declaration path is consulted.
func BindHot(fragment *SchemaFragment, owner *valueowner.HotOwner) (*HotRule, bool) {
	if fragment == nil || fragment.slot == nil || owner == nil || owner.Schema() == nil ||
		!fragment.semantic.Available() {
		return nil, false
	}
	schema := owner.Schema()
	rule := &HotRule{owner: owner}
	implementation, ok := valueowner.BindExactWriteRule(owner, fragment.slot, fragment.write, engine.HotRuleSpec[value.Value, identity.ContentID]{
		OperandContent:  globalContentForSchema(schema),
		OperandResolver: rule.resolveOperand,
		Fold: func(frame engine.Frame[value.Value, identity.ContentID]) engine.RuleResult[value.Value] {
			binding, operandOK := engine.Operand(frame)
			if !operandOK {
				return engine.RuleResult[value.Value]{}
			}
			result, resultOK := globalResultForSchema(schema, binding)
			if !resultOK {
				return engine.RuleResult[value.Value]{}
			}
			if result.absent {
				return engine.NoCandidate(frame)
			}
			return engine.Staged(frame, result.fact)
		},
	}, func(binding identity.ContentID) (uint64, bool) {
		result, resultOK := globalResultForSchema(schema, binding)
		index, indexOK := schema.CoordinateIndex(result.coordinate)
		return uint64(index), resultOK && indexOK
	})
	if !ok || implementation == nil {
		return nil, false
	}
	rule.implementation = implementation
	return rule, true
}

func (rule *HotRule) resolveOperand(coords engine.OperandCoords) (identity.ContentID, bool) {
	if rule == nil || rule.owner == nil || rule.owner.Schema() == nil {
		return identity.ContentID{}, false
	}
	receipt, ok := rule.owner.Schema().GlobalBootstrapResultForID(coords.Occurrence)
	if !ok {
		return identity.ContentID{}, false
	}
	return receipt.ID()
}

// Count implements the Link occurrence denominator from Value's canonical
// sealed schema. It retains no copied IDs or lookup state.
func (rule *HotRule) Count() int {
	if rule == nil || rule.owner == nil || rule.owner.Schema() == nil {
		return 0
	}
	return rule.owner.Schema().GlobalBootstrapResultCount()
}

// IDAt projects the canonical Value global-binding order for Link admission.
func (rule *HotRule) IDAt(index int) (identity.ContentID, bool) {
	if rule == nil || rule.owner == nil || rule.owner.Schema() == nil {
		return identity.ContentID{}, false
	}
	return rule.owner.Schema().GlobalBootstrapResultIDAt(index)
}

func (rule *HotRule) Implementation() (*valueowner.RuleImplementation[identity.ContentID], bool) {
	if rule == nil || rule.implementation == nil {
		return nil, false
	}
	return rule.implementation, true
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
