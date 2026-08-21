package fresh

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/domain/placement"
	placementowner "github.com/wippyai/go-lua/domain/placement/owner"
)

// HotRule is Placement/fresh's zero-input Link seed issuer. Heap owns the
// fresh-root denominator and the KeyID inverse; Placement retains only the
// operand authentication needed by its exact-write rule.
type HotRule struct {
	implementation *placementowner.RuleImplementation[operand]
	owner          *placementowner.HotOwner
}

// BindHot binds the exact Link-lane fresh-root seed to Placement's factor.
// Each Heap fresh root is unconditionally staged as Stack.
func BindHot(binding *engine.SchemaBinding, fragment *SchemaFragment, owner *placementowner.HotOwner, schema placement.Schema) (*HotRule, bool) {
	if binding == nil || fragment == nil || fragment.slot == nil || owner == nil || !owner.MatchesBinding(binding) || !owner.Schema().Valid() ||
		owner.Schema() != schema || !schema.Valid() || !fragment.semantic.Available() {
		return nil, false
	}
	rule := &HotRule{owner: owner}
	implementation, bound := placementowner.BindExactWriteRule(owner, fragment.slot, fragment.write, engine.HotRuleSpec[placement.Placement, operand]{
		OperandContent: func(candidate operand) (operand, [32]byte, bool) {
			return operandContentForSchema(schema, candidate)
		},
		OperandResolver: rule.resolveOperand,
		Fold: func(frame engine.Frame[placement.Placement, operand]) engine.RuleResult[placement.Placement] {
			return engine.Staged(frame, placement.Stack)
		},
	}, func(candidate operand) (uint64, bool) {
		return operandCoordinateForSchema(schema, candidate)
	})
	if !bound || implementation == nil {
		return nil, false
	}
	rule.implementation = implementation
	return rule, true
}

func (rule *HotRule) resolveOperand(coords engine.OperandCoords) (operand, bool) {
	if rule == nil || rule.owner == nil || !rule.owner.Schema().Valid() || !coords.Occurrence.Available() {
		return operand{}, false
	}
	key, keyOK := rule.owner.Schema().Heap().KeyForID(coords.Occurrence)
	if !keyOK {
		return operand{}, false
	}
	return operandForSchema(rule.owner.Schema(), key)
}

// Count implements the Link occurrence-inventory interface from Heap's
// canonical fresh-root denominator. Placement does not retain a second
// inventory.
func (rule *HotRule) Count() int {
	if rule == nil || rule.owner == nil || !rule.owner.Schema().Valid() {
		return 0
	}
	return rule.owner.Schema().Heap().FreshCount()
}

// IDAt projects one owner-issued fresh-root KeyID from Heap's canonical
// fresh-root order and reauthenticates it against Placement before publishing
// the Link occurrence identity.
func (rule *HotRule) IDAt(index int) (identity.ContentID, bool) {
	if rule == nil || rule.owner == nil || !rule.owner.Schema().Valid() || index < 0 {
		return identity.ContentID{}, false
	}
	id, key, keyOK := rule.owner.Schema().Heap().FreshAt(index)
	if !keyOK || !id.Available() {
		return identity.ContentID{}, false
	}
	candidate, candidateOK := operandForSchema(rule.owner.Schema(), key)
	return id, candidateOK && candidate.id == id
}

// Implementation returns the pending owner-typed Rule issuer.
func (rule *HotRule) Implementation() (*placementowner.RuleImplementation[operand], bool) {
	if rule == nil || rule.implementation == nil {
		return nil, false
	}
	return rule.implementation, true
}

// SealProgramRule publishes the exact sealed Link-lane Rule row.
func SealProgramRule(rule *HotRule) (engine.ProgramRule, bool) {
	if rule == nil || rule.owner == nil || rule.implementation == nil {
		return engine.ProgramRule{}, false
	}
	implementation, ok := placementowner.ResolveRuleImplementationFor(rule.owner, rule.implementation)
	if !ok {
		return engine.ProgramRule{}, false
	}
	return engine.SealProgramRule(implementation)
}
