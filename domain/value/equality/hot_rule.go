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
	implementation, bound := valueowner.BindSelectedRuleDirect(owner, fragment.slot, fragment.carry, fragment.write, owner.FactorRef(), engine.HotRuleSpec[value.Value, value.BinaryEquality]{
		OperandContent: func(row value.BinaryEquality) (value.BinaryEquality, [32]byte, bool) {
			return hotContent(owner.Schema(), row)
		},
		Fold: func(frame engine.Frame[value.Value, value.BinaryEquality]) engine.RuleResult[value.Value] {
			operand, operandOK := engine.Operand(frame)
			_, _, _, notEqual, endpointsOK := hotEndpoints(owner.Schema(), operand)
			if !operandOK || !endpointsOK {
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
		result, _, _, _, ok := hotEndpoints(owner.Schema(), row)
		index, indexOK := owner.Schema().CoordinateIndex(result)
		return uint64(index), ok && indexOK
	})
	if !bound {
		return nil, false
	}
	var leftOK, rightOK bool
	left, leftOK = valueowner.AddSelectedRuleDirectExactRead(implementation, fragment.left, owner.FactorRef(), func(row value.BinaryEquality) (uint64, bool) {
		_, leftCoord, _, _, ok := hotEndpoints(owner.Schema(), row)
		index, indexOK := owner.Schema().CoordinateIndex(leftCoord)
		return uint64(index), ok && indexOK
	})
	right, rightOK = valueowner.AddSelectedRuleDirectExactRead(implementation, fragment.right, owner.FactorRef(), func(row value.BinaryEquality) (uint64, bool) {
		_, _, rightCoord, _, ok := hotEndpoints(owner.Schema(), row)
		index, indexOK := owner.Schema().CoordinateIndex(rightCoord)
		return uint64(index), ok && indexOK
	})
	if !leftOK || !rightOK {
		return nil, false
	}
	leftRead, rightRead = left, right
	rule := &HotRule{implementation: implementation, left: left, right: right, owner: owner}
	if !implementation.InstallOperandResolver(rule.resolveOperand) {
		return nil, false
	}
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

func hotContent(schema *value.Schema, row value.BinaryEquality) (value.BinaryEquality, [32]byte, bool) {
	id, ok := row.ID()
	if schema == nil || !schema.OwnsBinaryEquality(row) || !ok || [32]byte(id) == ([32]byte{}) {
		return value.BinaryEquality{}, [32]byte{}, false
	}
	return row, [32]byte(id), true
}

func hotEndpoints(schema *value.Schema, row value.BinaryEquality) (result, left, right value.Coordinate, notEqual bool, ok bool) {
	if schema == nil || !schema.OwnsBinaryEquality(row) {
		return value.Coordinate{}, value.Coordinate{}, value.Coordinate{}, false, false
	}
	result, left, right, notEqual, ok = row.Endpoints()
	if !ok {
		return value.Coordinate{}, value.Coordinate{}, value.Coordinate{}, false, false
	}
	if _, resultOK := schema.CoordinateIndex(result); !resultOK {
		return value.Coordinate{}, value.Coordinate{}, value.Coordinate{}, false, false
	}
	if _, leftOK := schema.CoordinateIndex(left); !leftOK {
		return value.Coordinate{}, value.Coordinate{}, value.Coordinate{}, false, false
	}
	if _, rightOK := schema.CoordinateIndex(right); !rightOK {
		return value.Coordinate{}, value.Coordinate{}, value.Coordinate{}, false, false
	}
	return result, left, right, notEqual, true
}
