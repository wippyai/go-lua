package order

import (
	"github.com/wippyai/go-lua/analysis/domain/value"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
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
	tx, ok := valueowner.BeginSelectedRuleBinding(owner, fragment.slot, fragment.carry, fragment.write, owner.FactorRef(), engine.HotRuleSpec[value.Value, value.BinaryOrder]{
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
	}, engine.HotCarrySpec[value.Value, value.BinaryOrder]{})
	if !ok || tx == nil {
		return nil, false
	}
	left, leftOK := valueowner.AddSelectedRuleExactRead(tx, fragment.left, owner.FactorRef())
	right, rightOK := valueowner.AddSelectedRuleExactRead(tx, fragment.right, owner.FactorRef())
	implementation, implementationOK := tx.Implementation()
	if !leftOK || !rightOK || !implementationOK || !valueowner.CommitSelectedRuleBinding(tx) {
		valueowner.AbortSelectedRuleBinding(tx)
		return nil, false
	}
	leftRead, rightRead = left, right
	return &HotRule{implementation: implementation, left: left, right: right, owner: owner}, true
}

func (rule *HotRule) ReceiptForOccurrence(mount, id keyspace.ContentID) (value.BinaryOrder, bool) {
	if rule == nil || rule.owner == nil || rule.owner.Schema() == nil {
		return value.BinaryOrder{}, false
	}
	row, ok := rule.owner.Schema().BinaryOrder(mount, id)
	return row, ok && rule.owner.Schema().OwnsBinaryOrder(row)
}

func (rule *HotRule) AttachMountedRule(assembly *engine.ReceiptAssembly, mountID, pointID, occurrenceID keyspace.ContentID) (engine.BindingRuleRowRef, bool) {
	if rule == nil || rule.owner == nil || assembly == nil {
		return engine.BindingRuleRowRef{}, false
	}
	operand, operandOK := rule.ReceiptForOccurrence(mountID, occurrenceID)
	implementation, implementationOK := valueowner.ResolveRuleImplementationFor(rule.owner, rule.implementation)
	capability := mountedCapability(rule.implementation)
	occurrence, occurrenceOK := assembly.AdmitMountedRuleOccurrence(capability, mountID, pointID, occurrenceID)
	result, left, right, _, endpointsOK := hotEndpoints(rule.owner.Schema(), operand)
	leftRef, leftOK := rule.owner.Ref(left)
	rightRef, rightOK := rule.owner.Ref(right)
	resultRef, resultOK := rule.owner.Ref(result)
	if !operandOK || !implementationOK || !occurrenceOK || !endpointsOK || !leftOK || !rightOK || !resultOK {
		return engine.BindingRuleRowRef{}, false
	}
	transaction, transactionOK := engine.BeginMountedRuleAdmission(assembly, implementation, occurrence, operand)
	if !transactionOK || !engine.AddExactRead(transaction, leftRef) || !engine.AddExactRead(transaction, rightRef) || !transaction.AddCarry() || !engine.AddExactWrite(transaction, resultRef) {
		return engine.BindingRuleRowRef{}, false
	}
	queued := assembly.QueueMountedRuleFinalizer(capability, func() bool {
		source, sourceOK := transaction.Seal()
		draft, draftOK := implementation.BeginReceiptRuleRow(source)
		leftPart, leftPartOK := implementation.ReceiptReadPart(source, 0)
		rightPart, rightPartOK := implementation.ReceiptReadPart(source, 1)
		carryPart, carryPartOK := implementation.ReceiptCarryPart(source, 0)
		writePart, writePartOK := implementation.ReceiptWritePart(source, 0)
		if !sourceOK || !draftOK || !leftPartOK || !rightPartOK || !carryPartOK || !writePartOK ||
			!draft.AddRead(leftPart) || !draft.AddRead(rightPart) || !draft.AddCarry(carryPart) || !draft.AddWrite(writePart) {
			return false
		}
		_, added := assembly.AddRuleFromDraft(occurrence, draft)
		return added
	})
	return engine.BindingRuleRowRef{}, queued
}

func (rule *HotRule) AttachReceiptMember(compilation *engine.ReceiptCompilation, member engine.ReceiptRuleMember, operand value.BinaryOrder) (*engine.ReceiptMember, bool) {
	if rule == nil || rule.owner == nil || rule.owner.Schema() == nil || !rule.owner.Schema().OwnsBinaryOrder(operand) {
		return nil, false
	}
	implementation, ok := valueowner.ResolveRuleImplementationFor(rule.owner, rule.implementation)
	if !ok {
		return nil, false
	}
	return engine.AttachReceiptRuleMember(compilation, implementation, member, operand)
}

func (rule *HotRule) AttachMountedReceiptMember(compilation *engine.ReceiptCompilation, graph *engine.ReceiptGraph, mountID, pointID, occurrenceID keyspace.ContentID) (*engine.ReceiptMember, bool) {
	if rule == nil || graph == nil {
		return nil, false
	}
	member, memberOK := graph.MountedRuleMember(mountedCapability(rule.implementation), mountID, pointID, occurrenceID)
	operand, operandOK := rule.ReceiptForOccurrence(mountID, occurrenceID)
	if !memberOK || !operandOK {
		return nil, false
	}
	return rule.AttachReceiptMember(compilation, member, operand)
}

func (rule *HotRule) Implementation() (*valueowner.RuleImplementation[value.BinaryOrder], bool) {
	if rule == nil || rule.implementation == nil {
		return nil, false
	}
	_, ok := valueowner.ResolveRuleImplementation(rule.implementation)
	return rule.implementation, ok
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

func hotChecker(owner *valueowner.HotOwner, semantic engine.SemanticKey, leftRead, rightRead *engine.Read[engine.OrderedCells[value.Value]]) engine.RuleDerivationChecker[value.Value, value.BinaryOrder] {
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
