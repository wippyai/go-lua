package arithmetic

import (
	"github.com/wippyai/go-lua/analysis/engine"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// HotRule is Value's receipt-native primitive arithmetic transfer. It keeps
// only owner-issued Value reads and the sealed BinaryArithmetic operand; no
// Program, Flow, Link, or raw scalar enters hot state.
type HotRule struct {
	implementation *valueowner.RuleImplementation[value.BinaryArithmetic]
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
	implementation, bound := valueowner.BindSelectedRuleDirect(owner, fragment.slot, fragment.carry, fragment.write, owner.FactorRef(), engine.HotRuleSpec[value.Value, value.BinaryArithmetic]{
		OperandContent: func(row value.BinaryArithmetic) (value.BinaryArithmetic, [32]byte, bool) {
			return hotContent(owner.Schema(), row)
		},
		Fold: func(frame engine.Frame[value.Value, value.BinaryArithmetic]) engine.RuleResult[value.Value] {
			operand, operandOK := engine.Operand(frame)
			_, _, _, op, endpointsOK := hotEndpoints(owner.Schema(), operand)
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
			// Bottom denotes the arithmetic trap alone, so NoCandidate is
			// staged only where no concrete result is reachable. Every
			// undecided operand pair carries Top and stages a real
			// candidate.
			result, resultOK := owner.Schema().ApplyArithmetic(left, right, op)
			if !resultOK {
				return engine.RuleResult[value.Value]{}
			}
			if owner.Schema().Equal(result, owner.Schema().Bottom()) {
				return engine.NoCandidate(frame)
			}
			return engine.Staged(frame, result)
		},
	}, engine.HotCarrySpec[value.Value, value.BinaryArithmetic]{}, func(row value.BinaryArithmetic) (uint64, bool) {
		result, _, _, _, ok := hotEndpoints(owner.Schema(), row)
		index, indexOK := owner.Schema().CoordinateIndex(result)
		return uint64(index), ok && indexOK
	})
	if !bound {
		return nil, false
	}
	var leftOK, rightOK bool
	left, leftOK = valueowner.AddSelectedRuleDirectExactRead(implementation, fragment.left, owner.FactorRef(), func(row value.BinaryArithmetic) (uint64, bool) {
		_, leftCoord, _, _, ok := hotEndpoints(owner.Schema(), row)
		index, indexOK := owner.Schema().CoordinateIndex(leftCoord)
		return uint64(index), ok && indexOK
	})
	right, rightOK = valueowner.AddSelectedRuleDirectExactRead(implementation, fragment.right, owner.FactorRef(), func(row value.BinaryArithmetic) (uint64, bool) {
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

func (rule *HotRule) resolveOperand(coords engine.OperandCoords) (value.BinaryArithmetic, bool) {
	if rule == nil || rule.owner == nil || rule.owner.Schema() == nil {
		return value.BinaryArithmetic{}, false
	}
	row, ok := rule.owner.Schema().BinaryArithmetic(coords.Mount, coords.Occurrence)
	return row, ok && rule.owner.Schema().OwnsBinaryArithmetic(row)
}

func (rule *HotRule) Implementation() (*valueowner.RuleImplementation[value.BinaryArithmetic], bool) {
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

func hotContent(schema *value.Schema, row value.BinaryArithmetic) (value.BinaryArithmetic, [32]byte, bool) {
	id, ok := row.ID()
	if schema == nil || !schema.OwnsBinaryArithmetic(row) || !ok || [32]byte(id) == ([32]byte{}) {
		return value.BinaryArithmetic{}, [32]byte{}, false
	}
	return row, [32]byte(id), true
}

func hotEndpoints(schema *value.Schema, row value.BinaryArithmetic) (result, left, right value.Coordinate, op flowkind.BinaryOp, ok bool) {
	if schema == nil || !schema.OwnsBinaryArithmetic(row) {
		return value.Coordinate{}, value.Coordinate{}, value.Coordinate{}, 0, false
	}
	result, left, right, op, ok = row.Endpoints()
	if !ok || !flowkind.IsBinaryArithmetic(op) {
		return value.Coordinate{}, value.Coordinate{}, value.Coordinate{}, 0, false
	}
	if _, resultOK := schema.CoordinateIndex(result); !resultOK {
		return value.Coordinate{}, value.Coordinate{}, value.Coordinate{}, 0, false
	}
	if _, leftOK := schema.CoordinateIndex(left); !leftOK {
		return value.Coordinate{}, value.Coordinate{}, value.Coordinate{}, 0, false
	}
	if _, rightOK := schema.CoordinateIndex(right); !rightOK {
		return value.Coordinate{}, value.Coordinate{}, value.Coordinate{}, 0, false
	}
	return result, left, right, op, true
}
