package order

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
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
		!fragment.semantic.Available() || !fragment.evidence.Available() || fragment.semantic == fragment.evidence {
		return nil, false
	}
	var leftRead, rightRead engine.Read[engine.OrderedCells[value.Value]]
	var left, right engine.Read[engine.OrderedCells[value.Value]]
	var implementation *valueowner.RuleImplementation[value.BinaryOrder]
	bound := valueowner.BindSelectedRule(owner, fragment.slot, fragment.carry, fragment.write, owner.FactorRef(), engine.HotRuleSpec[value.Value, value.BinaryOrder]{
		OperandContent: func(row value.BinaryOrder) (value.BinaryOrder, [32]byte, bool) {
			return hotContent(owner.Schema(), row)
		},
		Admission: engine.AdmitRuleByDerivation(fragment.evidence, hotChecker(owner, fragment.semantic, &leftRead, &rightRead)),
		Transfer: func(access engine.Access[value.Value, value.BinaryOrder]) bool {
			operand, operandOK := engine.Operand(access)
			_, _, _, op, endpointsOK := hotEndpoints(owner.Schema(), operand)
			if !operandOK || !endpointsOK {
				return false
			}
			return engine.Product(access, func(row engine.Row) bool {
				leftCells, leftOK := engine.ReadValue(access, row, leftRead)
				rightCells, rightOK := engine.ReadValue(access, row, rightRead)
				if !leftOK || !rightOK || leftCells.Count() != 1 || rightCells.Count() != 1 {
					return false
				}
				left, leftPresent, leftAvailable := leftCells.At(0)
				right, rightPresent, rightAvailable := rightCells.At(0)
				if !leftAvailable || !rightAvailable {
					return false
				}
				if !leftPresent || !rightPresent {
					return engine.NoCandidate(access, row)
				}
				result, resultOK := owner.Schema().CompareOrder(left, right, op)
				if !resultOK || owner.Schema().Equal(result, owner.Schema().Bottom()) {
					return resultOK && engine.NoCandidate(access, row)
				}
				return engine.StageValue(access, row, result)
			})
		},
	}, engine.HotCarrySpec[value.Value, value.BinaryOrder]{}, func(row value.BinaryOrder) (uint64, bool) {
		result, _, _, _, ok := hotEndpoints(owner.Schema(), row)
		index, indexOK := owner.Schema().CoordinateIndex(result)
		return uint64(index), ok && indexOK
	}, func(tx *valueowner.SelectedRuleBinding[value.BinaryOrder]) bool {
		var leftOK, rightOK, implementationOK bool
		left, leftOK = valueowner.AddSelectedRuleExactRead(tx, fragment.left, owner.FactorRef(), func(row value.BinaryOrder) (uint64, bool) {
			_, leftCoord, _, _, ok := hotEndpoints(owner.Schema(), row)
			index, indexOK := owner.Schema().CoordinateIndex(leftCoord)
			return uint64(index), ok && indexOK
		})
		right, rightOK = valueowner.AddSelectedRuleExactRead(tx, fragment.right, owner.FactorRef(), func(row value.BinaryOrder) (uint64, bool) {
			_, _, rightCoord, _, ok := hotEndpoints(owner.Schema(), row)
			index, indexOK := owner.Schema().CoordinateIndex(rightCoord)
			return uint64(index), ok && indexOK
		})
		implementation, implementationOK = tx.Implementation()
		return leftOK && rightOK && implementationOK
	})
	if !bound {
		return nil, false
	}
	leftRead, rightRead = left, right
	rule := &HotRule{implementation: implementation, left: left, right: right, owner: owner}
	if !implementation.InstallOperandResolver(rule.resolveOperand) {
		return nil, false
	}
	return rule, true
}

func (rule *HotRule) resolveOperand(coords engine.OperandCoords) (value.BinaryOrder, bool) {
	return rule.ReceiptForOccurrence(coords.Mount, coords.Occurrence)
}

func (rule *HotRule) ReceiptForOccurrence(mount, id identity.ContentID) (value.BinaryOrder, bool) {
	if rule == nil || rule.owner == nil || rule.owner.Schema() == nil {
		return value.BinaryOrder{}, false
	}
	row, ok := rule.owner.Schema().BinaryOrder(mount, id)
	return row, ok && rule.owner.Schema().OwnsBinaryOrder(row)
}

func (rule *HotRule) Implementation() (*valueowner.RuleImplementation[value.BinaryOrder], bool) {
	if rule == nil || rule.implementation == nil {
		return nil, false
	}
	_, ok := valueowner.ResolveRuleImplementation(rule.implementation)
	return rule.implementation, ok
}

func (rule *HotRule) ProgramAttach() (engine.RuleProgramAttach, bool) {
	return valueowner.ResolveRuleImplementationFor(rule.owner, rule.implementation)
}

func mountedCapability(issuer interface {
	MountedCapability() (engine.RuleSlotCapability, bool)
}) engine.RuleSlotCapability {
	capability, _ := issuer.MountedCapability()
	return capability
}

func hotContent(schema *value.Schema, row value.BinaryOrder) (value.BinaryOrder, [32]byte, bool) {
	id, ok := row.ID()
	if schema == nil || !schema.OwnsBinaryOrder(row) || !ok || [32]byte(id) == ([32]byte{}) {
		return value.BinaryOrder{}, [32]byte{}, false
	}
	return row, [32]byte(id), true
}

func hotEndpoints(schema *value.Schema, row value.BinaryOrder) (result, left, right value.Coordinate, op flowkind.BinaryOp, ok bool) {
	if schema == nil || !schema.OwnsBinaryOrder(row) {
		return value.Coordinate{}, value.Coordinate{}, value.Coordinate{}, 0, false
	}
	result, left, right, op, ok = row.Endpoints()
	if !ok {
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

func hotChecker(owner *valueowner.HotOwner, semantic identity.SemanticKey, leftRead, rightRead *engine.Read[engine.OrderedCells[value.Value]]) engine.RuleDerivationChecker[value.Value, value.BinaryOrder] {
	return func(derivation engine.RuleDerivation[value.Value, value.BinaryOrder]) (engine.RuleEvidence, bool) {
		if owner == nil || owner.Schema() == nil || leftRead == nil || rightRead == nil || derivation.Rule() != semantic || derivation.InputCount() != 1 || derivation.ReadCount() != 2 || derivation.DispositionCount() == 0 {
			return engine.RuleEvidence{}, false
		}
		operand, operandOK := derivation.Operand()
		canonical, digest, contentOK := hotContent(owner.Schema(), operand)
		result, left, right, op, endpointsOK := hotEndpoints(owner.Schema(), canonical)
		input, inputOK := derivation.InputAt(0)
		if !operandOK || !contentOK || !endpointsOK || !derivation.OperandContentMatches(digest) || !inputOK || input.Guard().Empty() ||
			!valueowner.ReadMatches(owner, derivation, *leftRead, left) || !valueowner.ReadMatches(owner, derivation, *rightRead, right) {
			return engine.RuleEvidence{}, false
		}
		for index := 0; index < derivation.DispositionCount(); index++ {
			disposition, dispositionOK := derivation.DispositionAt(index)
			if !dispositionOK || disposition.Guard().Empty() {
				return engine.RuleEvidence{}, false
			}
			leftCells, leftOK := engine.DerivationDispositionReadValue(derivation, disposition, *leftRead)
			rightCells, rightOK := engine.DerivationDispositionReadValue(derivation, disposition, *rightRead)
			if !leftOK || !rightOK || leftCells.Count() != 1 || rightCells.Count() != 1 {
				return engine.RuleEvidence{}, false
			}
			leftValue, leftPresent, leftAvailable := leftCells.At(0)
			rightValue, rightPresent, rightAvailable := rightCells.At(0)
			if !leftAvailable || !rightAvailable {
				return engine.RuleEvidence{}, false
			}
			if !leftPresent || !rightPresent {
				if disposition.Kind() != engine.RuleDispositionNoCandidate || disposition.TargetCount() != 0 {
					return engine.RuleEvidence{}, false
				}
				continue
			}
			expected, expectedOK := owner.Schema().CompareOrder(leftValue, rightValue, op)
			if !expectedOK {
				return engine.RuleEvidence{}, false
			}
			if owner.Schema().Equal(expected, owner.Schema().Bottom()) {
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
			if !targetOK || !valueOK || !owner.TargetMatches(target, result) || !owner.Schema().Equal(actual, expected) {
				return engine.RuleEvidence{}, false
			}
		}
		return derivation.Accept()
	}
}
