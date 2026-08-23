package order

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// HotRule is Value's receipt-native ordered relational computation. It
// retains only owner-issued Value read capabilities and the sealed
// BinaryOrder operand; no Program or Flow carrier is reachable from hot state.
type HotRule struct {
	implementation *valueowner.RuleImplementation[value.BinaryOrder]
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
	var implementation *valueowner.RuleImplementation[value.BinaryOrder]
	rule := &HotRule{owner: owner}
	implementation, bound := valueowner.BindSelectedRuleDirect(owner, fragment.slot, fragment.carry, fragment.write, owner.FactorRef(), engine.HotRuleSpec[value.Value, value.BinaryOrder]{
		OperandContent: func(row value.BinaryOrder) (value.BinaryOrder, [32]byte, bool) {
			return hotContent(owner.Schema(), row)
		},
		OperandResolver: rule.resolveOperand,
		Fold: func(frame engine.Frame[value.Value, value.BinaryOrder]) engine.RuleResult[value.Value] {
			operand, operandOK := engine.Operand(frame)
			op, opOK := operand.Op()
			if !operandOK || !opOK {
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
			result, resultOK := owner.Schema().CompareOrder(left, right, op)
			if !resultOK {
				return engine.RuleResult[value.Value]{}
			}
			if owner.Schema().Equal(result, owner.Schema().Bottom()) {
				return engine.NoCandidate(frame)
			}
			return engine.Staged(frame, result)
		},
	}, engine.HotCarrySpec[value.Value, value.BinaryOrder]{}, func(row value.BinaryOrder) (uint64, bool) {
		return row.Endpoint(value.EndpointWrite)
	})
	if !bound {
		return nil, false
	}
	var leftOK, rightOK bool
	left, leftOK = valueowner.AddSelectedRuleDirectExactRead(implementation, fragment.left, owner.FactorRef(), func(row value.BinaryOrder) (uint64, bool) {
		return row.Endpoint(value.EndpointLeft)
	})
	right, rightOK = valueowner.AddSelectedRuleDirectExactRead(implementation, fragment.right, owner.FactorRef(), func(row value.BinaryOrder) (uint64, bool) {
		return row.Endpoint(value.EndpointRight)
	})
	if !leftOK || !rightOK {
		return nil, false
	}
	leftRead, rightRead = left, right
	rule.implementation, rule.left, rule.right = implementation, left, right
	return rule, true
}

func (rule *HotRule) resolveOperand(coords engine.OperandCoords) (value.BinaryOrder, bool) {
	if rule == nil || rule.owner == nil || rule.owner.Schema() == nil {
		return value.BinaryOrder{}, false
	}
	row, ok := rule.owner.Schema().BinaryOrder(coords.Mount, coords.Occurrence)
	return row, ok && rule.owner.Schema().OwnsBinaryOrder(row)
}

func (rule *HotRule) Implementation() (*valueowner.RuleImplementation[value.BinaryOrder], bool) {
	if rule == nil || rule.implementation == nil {
		return nil, false
	}
	_, ok := valueowner.ResolveRuleImplementation(rule.implementation)
	return rule.implementation, ok
}

func hotContent(schema *value.Schema, row value.BinaryOrder) (value.BinaryOrder, [32]byte, bool) {
	id, ok := row.ID()
	if schema == nil || !schema.OwnsBinaryOrder(row) || !ok || [32]byte(id) == ([32]byte{}) {
		return value.BinaryOrder{}, [32]byte{}, false
	}
	return row, [32]byte(id), true
}
