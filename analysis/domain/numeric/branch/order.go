package branch

import (
	"github.com/wippyai/go-lua/analysis/domain/numeric"
	numericowner "github.com/wippyai/go-lua/analysis/domain/numeric/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
)

// OrderOperand retains one primitive integer order branch. The Rule emits no
// constraint until its exact Numeric predecessor proves both operands integer.
type OrderOperand struct{ branchOperand }

func NewOrderOperand(source *link.Link, algebra *numeric.Algebra, shard linkproject.Shard, binary keyspace.Term, branch int) (OrderOperand, bool) {
	operand, ok := newBranchOperand(source, algebra, shard, binary, branch)
	if !ok || !orderOperator(operand.op) {
		return OrderOperand{}, false
	}
	result := OrderOperand{branchOperand: operand}
	if !pairSupported(algebra, operand.outputKey, operand.left, operand.right) ||
		!pairSupported(algebra, operand.outputKey, operand.right, operand.left) {
		return OrderOperand{}, false
	}
	return result, true
}

func (operand OrderOperand) BranchRoot() (numeric.Root, bool) {
	return operand.branchOperand.BranchRoot()
}

func orderConstraint(algebra *numeric.Algebra, operand OrderOperand) (numeric.Value, bool) {
	if algebra == nil || operand.algebra != algebra || !operand.branchOperand.valid() || !orderOperator(operand.op) {
		return numeric.Value{}, false
	}
	trueComparison := operand.truthy != operand.invert
	strict := operand.op == kind.BinaryLess || operand.op == kind.BinaryGreater
	left, right, bound := operand.left, operand.right, int64(0)
	if strict == trueComparison {
		bound = -1
	}
	if !trueComparison {
		left, right = right, left
	}
	return algebra.IntegerDifference(operand.outputKey, left, right, bound)
}

type OrderRule struct {
	rule     *engine.Rule[numeric.Value, OrderOperand]
	owner    *numericowner.Owner
	semantic engine.SemanticKey
	read     engine.Read[engine.OrderedCells[numeric.Value]]
	write    engine.Write[numeric.Value]
}

func DeclareOrder(composition *engine.Composition, semantic, operandFamily, evidence engine.SemanticKey, owner *numericowner.Owner) (*OrderRule, bool) {
	if !validDeclaration(composition, semantic, operandFamily, evidence, owner) {
		return nil, false
	}
	declaration := &OrderRule{owner: owner, semantic: semantic}
	declared, ok := engine.DeclareRule(composition, engine.RuleSpec[numeric.Value, OrderOperand]{
		Semantic: semantic, OperandFamily: operandFamily, OperandContent: orderOperandContent,
		Output: owner.Output(), Inputs: 1, Admission: engine.AdmitRuleByDerivation(evidence, declaration.check), Transfer: declaration.transfer,
	}, func(rule *engine.Rule[numeric.Value, OrderOperand]) bool {
		input, inputOK := rule.InputAt(0)
		read, readOK := engine.ReadFrom(rule, input, owner.ExactRead())
		write, writeOK := engine.WriteTo(rule, owner.ExactWrite())
		if inputOK && readOK && writeOK {
			declaration.rule, declaration.read, declaration.write = rule, read, write
		}
		return inputOK && readOK && writeOK
	})
	if !ok || declared == nil || declaration.rule != declared {
		return nil, false
	}
	return declaration, true
}

func (rule *OrderRule) Instance(operand OrderOperand) (*engine.RuleInstance[numeric.Value, OrderOperand], bool) {
	if rule == nil || rule.rule == nil || !validOrderOperand(rule.owner, operand) {
		return nil, false
	}
	input, inputOK := rule.owner.Locate(operand.inputKey)
	output, outputOK := rule.owner.Locate(operand.outputKey)
	if !inputOK || !outputOK {
		return nil, false
	}
	return engine.NewRuleInstance(rule.rule, operand, func(binding *engine.RuleBinding[numeric.Value, OrderOperand]) bool {
		return engine.InstanceRead(binding, rule.read, input) && engine.InstanceWrite(binding, rule.write, output)
	})
}

func orderOperandContent(operand OrderOperand) (OrderOperand, [32]byte, bool) {
	return operand, operand.content, operand.branchOperand.valid() && orderOperator(operand.op)
}

func validOrderOperand(owner *numericowner.Owner, operand OrderOperand) bool {
	if owner == nil || owner.Algebra() == nil || operand.algebra != owner.Algebra() {
		return false
	}
	expected, ok := NewOrderOperand(operand.source, owner.Algebra(), operand.shard, operand.binary, int(operand.branch))
	return ok && expected.content == operand.content && expected.inputKey == operand.inputKey && expected.outputKey == operand.outputKey
}

func orderResult(algebra *numeric.Algebra, operand OrderOperand, current numeric.Value) (numeric.Value, bool) {
	if algebra == nil || !algebra.Admits(operand.inputKey, current) {
		return numeric.Value{}, false
	}
	left, leftOK := current.Eligibility(operand.left)
	right, rightOK := current.Eligibility(operand.right)
	if !leftOK || !rightOK || !left.MustInteger() || !right.MustInteger() {
		return numeric.Value{}, false
	}
	value, ok := orderConstraint(algebra, operand)
	if ok {
		return value, true
	}
	// With an exact MustInteger premise and sealed pair/threshold topology, a
	// rejected integer constraint is an intrinsic contradiction.
	return algebra.Bottom(), true
}

func (rule *OrderRule) transfer(access engine.Access[numeric.Value, OrderOperand]) bool {
	operand, ok := engine.Operand(access)
	if !ok || !validOrderOperand(rule.owner, operand) {
		return false
	}
	return engine.Product(access, func(row engine.Row) bool {
		cells, readOK := engine.ReadValue(access, row, rule.read)
		if !readOK || cells.Count() != 1 {
			return false
		}
		current, present, cellOK := cells.At(0)
		if !cellOK {
			return false
		}
		if !present {
			return engine.NoCandidate(access, row)
		}
		value, reduced := orderResult(rule.owner.Algebra(), operand, current)
		if !reduced {
			return engine.NoCandidate(access, row)
		}
		return engine.StageValue(access, row, value)
	})
}

func (rule *OrderRule) check(derivation engine.RuleDerivation[numeric.Value, OrderOperand]) (engine.RuleEvidence, bool) {
	if rule == nil || derivation.Rule() != rule.semantic || derivation.InputCount() != 1 || derivation.ReadCount() != 1 || derivation.DispositionCount() != 1 {
		return engine.RuleEvidence{}, false
	}
	operand, ok := derivation.Operand()
	input, exactInput := derivation.InputAt(0)
	inputRef, inputOK := rule.owner.Locate(operand.inputKey)
	outputRef, outputOK := rule.owner.Locate(operand.outputKey)
	if !ok || !exactInput || input.Guard().Empty() || !validOrderOperand(rule.owner, operand) || !derivation.OperandContentMatches(operand.content) || !inputOK || !outputOK ||
		!engine.DerivationReadMatchesRef(derivation, rule.read, inputRef) {
		return engine.RuleEvidence{}, false
	}
	disposition, ok := derivation.DispositionAt(0)
	if !ok || disposition.Guard().Empty() || !disposition.Guard().Same(input.Guard()) {
		return engine.RuleEvidence{}, false
	}
	cells, cellsOK := engine.DerivationDispositionReadValue(derivation, disposition, rule.read)
	if !cellsOK || cells.Count() != 1 {
		return engine.RuleEvidence{}, false
	}
	current, present, cellOK := cells.At(0)
	if !cellOK {
		return engine.RuleEvidence{}, false
	}
	expected, reduced := orderResult(rule.owner.Algebra(), operand, current)
	if !present || !reduced {
		if disposition.Kind() != engine.RuleDispositionNoCandidate || disposition.TargetCount() != 0 {
			return engine.RuleEvidence{}, false
		}
		return derivation.Accept()
	}
	target, targetOK := disposition.TargetAt(0)
	actual, actualOK := disposition.Value()
	if disposition.Kind() != engine.RuleDispositionStaged || disposition.TargetCount() != 1 || !targetOK || !actualOK ||
		!engine.TargetMatchesRef(target, outputRef) || !rule.owner.Algebra().Equal(actual, expected) {
		return engine.RuleEvidence{}, false
	}
	return derivation.Accept()
}

func validDeclaration(composition *engine.Composition, semantic, operandFamily, evidence engine.SemanticKey, owner *numericowner.Owner) bool {
	return composition != nil && owner != nil && owner.Algebra() != nil && semantic.Available() && operandFamily.Available() && evidence.Available() &&
		semantic != operandFamily && semantic != evidence && operandFamily != evidence
}
