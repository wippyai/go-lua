package equality

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// HotRule is Value's receipt-native ordered equality computation. It retains
// only owner-issued Value read capabilities and the sealed BinaryEquality
// operand; no Program or Flow carrier is reachable from the hot path.
type HotRule struct {
	implementation *valueowner.RuleImplementation[value.BinaryEquality]
	left, right    engine.Read[engine.OrderedCells[value.Value]]
	owner          *valueowner.HotOwner
}

func BindHot(fragment *SchemaFragment, owner *valueowner.HotOwner) (*HotRule, bool) {
	if fragment == nil || fragment.slot == nil || owner == nil || owner.Schema() == nil ||
		!fragment.semantic.Available() {
		return nil, false
	}
	var leftRead, rightRead engine.Read[engine.OrderedCells[value.Value]]
	var left, right engine.Read[engine.OrderedCells[value.Value]]
	var implementation *valueowner.RuleImplementation[value.BinaryEquality]
	rule := &HotRule{owner: owner}
	implementation, bound := valueowner.BindSelectedRuleDirect(owner, fragment.slot, fragment.carry, fragment.write, owner.FactorRef(), engine.HotRuleSpec[value.Value, value.BinaryEquality]{
		OperandContent: func(row value.BinaryEquality) (value.BinaryEquality, [32]byte, bool) {
			return hotContent(owner.Schema(), row)
		},
		OperandResolver: rule.resolveOperand,
		Fold: func(frame engine.Frame[value.Value, value.BinaryEquality]) engine.RuleResult[value.Value] {
			operand, operandOK := engine.Operand(frame)
			notEqual, notEqualOK := operand.NotEqual()
			if !operandOK || !notEqualOK {
				return engine.RuleResult[value.Value]{}
			}
			leftCells, leftOK := engine.ReadValue(frame, leftRead)
			rightCells, rightOK := engine.ReadValue(frame, rightRead)
			if !leftOK || !rightOK || leftCells.Count() != 1 || rightCells.Count() != 1 {
				return engine.RuleResult[value.Value]{}
			}
			left, leftPresent, leftAvailable := leftCells.At(0)
			right, rightPresent, rightAvailable := rightCells.At(0)
			if !leftAvailable || !rightAvailable {
				return engine.RuleResult[value.Value]{}
			}
			if !leftPresent || !rightPresent {
				return engine.NoCandidate(frame)
			}
			result, resultOK := owner.Schema().CompareEquality(left, right, notEqual)
			if !resultOK {
				return engine.RuleResult[value.Value]{}
			}
			return engine.Staged(frame, result)
		},
	}, engine.HotCarrySpec[value.Value, value.BinaryEquality]{}, func(row value.BinaryEquality) (uint64, bool) {
		return row.Endpoint(value.EndpointWrite)
	})
	if !bound {
		return nil, false
	}
	var leftOK, rightOK bool
	left, leftOK = valueowner.AddSelectedRuleDirectExactRead(implementation, fragment.left, owner.FactorRef(), func(row value.BinaryEquality) (uint64, bool) {
		return row.Endpoint(value.EndpointLeft)
	})
	right, rightOK = valueowner.AddSelectedRuleDirectExactRead(implementation, fragment.right, owner.FactorRef(), func(row value.BinaryEquality) (uint64, bool) {
		return row.Endpoint(value.EndpointRight)
	})
	if !leftOK || !rightOK {
		return nil, false
	}
	leftRead, rightRead = left, right
	rule.implementation, rule.left, rule.right = implementation, left, right
	return rule, true
}

func (rule *HotRule) resolveOperand(coords engine.OperandCoords) (value.BinaryEquality, bool) {
	if rule == nil || rule.owner == nil || rule.owner.Schema() == nil {
		return value.BinaryEquality{}, false
	}
	row, ok := rule.owner.Schema().BinaryEquality(coords.Mount, coords.Occurrence)
	return row, ok && rule.owner.Schema().OwnsBinaryEquality(row)
}

func (rule *HotRule) Implementation() (*valueowner.RuleImplementation[value.BinaryEquality], bool) {
	if rule == nil || rule.implementation == nil {
		return nil, false
	}
	_, ok := valueowner.ResolveRuleImplementation(rule.implementation)
	return rule.implementation, ok
}

func hotContent(schema *value.Schema, row value.BinaryEquality) (value.BinaryEquality, [32]byte, bool) {
	id, ok := row.ID()
	if schema == nil || !schema.OwnsBinaryEquality(row) || !ok || [32]byte(id) == ([32]byte{}) {
		return value.BinaryEquality{}, [32]byte{}, false
	}
	return row, [32]byte(id), true
}
