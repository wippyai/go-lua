package branch

import (
	"github.com/wippyai/go-lua/analysis/domain/numeric"
	numericowner "github.com/wippyai/go-lua/analysis/domain/numeric/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
)

// EqualityOperand retains one Flow BinaryPrimitive ==/~= truth branch. The
// sealed Comparison inversion is applied exactly once when selecting raw
// equality versus disequality.
type EqualityOperand struct{ branchOperand }

func NewEqualityOperand(source *link.Link, algebra *numeric.Algebra, shard linkproject.Shard, binary keyspace.Term, branch int) (EqualityOperand, bool) {
	operand, ok := newBranchOperand(source, algebra, shard, binary, branch)
	if !ok || !equalityOperator(operand.op) {
		return EqualityOperand{}, false
	}
	result := EqualityOperand{branchOperand: operand}
	if !pairSupported(algebra, operand.outputKey, operand.left, operand.right) {
		return EqualityOperand{}, false
	}
	return result, true
}

func (operand EqualityOperand) BranchRoot() (numeric.Root, bool) {
	return operand.branchOperand.BranchRoot()
}

func equalityResult(algebra *numeric.Algebra, operand EqualityOperand) (numeric.Value, bool) {
	if algebra == nil || operand.algebra != algebra || !operand.branchOperand.valid() || !equalityOperator(operand.op) {
		return numeric.Value{}, false
	}
	var value numeric.Value
	var ok bool
	if operand.truthy != operand.invert {
		value, ok = algebra.MustRawEqual(operand.outputKey, operand.left, operand.right)
	} else {
		value, ok = algebra.MustRawUnequal(operand.outputKey, operand.left, operand.right)
	}
	if ok {
		return value, true
	}
	// The pair is sealed at this exact branch key, so normalization rejection
	// means the branch contradicts intrinsic literal/NaN facts.
	return algebra.Bottom(), true
}

type EqualityRule struct {
	rule     *engine.Rule[numeric.Value, EqualityOperand]
	owner    *numericowner.Owner
	semantic engine.SemanticKey
	write    engine.Write[numeric.Value]
}

func DeclareEquality(composition *engine.Composition, semantic, operandFamily, evidence engine.SemanticKey, owner *numericowner.Owner) (*EqualityRule, bool) {
	if !validDeclaration(composition, semantic, operandFamily, evidence, owner) {
		return nil, false
	}
	declaration := &EqualityRule{owner: owner, semantic: semantic}
	declared, ok := engine.DeclareRule(composition, engine.RuleSpec[numeric.Value, EqualityOperand]{
		Semantic: semantic, OperandFamily: operandFamily, OperandContent: equalityOperandContent,
		Output: owner.Output(), Inputs: 0, Admission: engine.AdmitRuleByDerivation(evidence, declaration.check), Transfer: declaration.transfer,
	}, func(rule *engine.Rule[numeric.Value, EqualityOperand]) bool {
		write, written := engine.WriteTo(rule, owner.ExactWrite())
		if written {
			declaration.rule, declaration.write = rule, write
		}
		return written
	})
	if !ok || declared == nil || declaration.rule != declared {
		return nil, false
	}
	return declaration, true
}

func (rule *EqualityRule) Instance(operand EqualityOperand) (*engine.RuleInstance[numeric.Value, EqualityOperand], bool) {
	if rule == nil || rule.rule == nil || !validEqualityOperand(rule.owner, operand) {
		return nil, false
	}
	ref, ok := rule.owner.Locate(operand.outputKey)
	if !ok {
		return nil, false
	}
	return engine.NewRuleInstance(rule.rule, operand, func(binding *engine.RuleBinding[numeric.Value, EqualityOperand]) bool {
		return engine.InstanceWrite(binding, rule.write, ref)
	})
}

func equalityOperandContent(operand EqualityOperand) (EqualityOperand, [32]byte, bool) {
	return operand, operand.content, operand.branchOperand.valid() && equalityOperator(operand.op)
}

func validEqualityOperand(owner *numericowner.Owner, operand EqualityOperand) bool {
	if owner == nil || owner.Algebra() == nil || operand.algebra != owner.Algebra() {
		return false
	}
	expected, ok := NewEqualityOperand(operand.source, owner.Algebra(), operand.shard, operand.binary, int(operand.branch))
	return ok && expected.content == operand.content && expected.outputKey == operand.outputKey
}

func (rule *EqualityRule) transfer(access engine.Access[numeric.Value, EqualityOperand]) bool {
	operand, ok := engine.Operand(access)
	if !ok || !validEqualityOperand(rule.owner, operand) {
		return false
	}
	value, ok := equalityResult(rule.owner.Algebra(), operand)
	if !ok {
		return false
	}
	return engine.Product(access, func(row engine.Row) bool { return engine.StageValue(access, row, value) })
}

func (rule *EqualityRule) check(derivation engine.RuleDerivation[numeric.Value, EqualityOperand]) (engine.RuleEvidence, bool) {
	if rule == nil || derivation.Rule() != rule.semantic || derivation.InputCount() != 0 || derivation.ReadCount() != 0 || derivation.DispositionCount() != 1 {
		return engine.RuleEvidence{}, false
	}
	operand, ok := derivation.Operand()
	if !ok || !validEqualityOperand(rule.owner, operand) || !derivation.OperandContentMatches(operand.content) {
		return engine.RuleEvidence{}, false
	}
	disposition, ok := derivation.DispositionAt(0)
	target, targetOK := disposition.TargetAt(0)
	actual, actualOK := disposition.Value()
	expected, expectedOK := equalityResult(rule.owner.Algebra(), operand)
	ref, refOK := rule.owner.Locate(operand.outputKey)
	if !ok || disposition.Kind() != engine.RuleDispositionStaged || disposition.TargetCount() != 1 || disposition.Guard().Empty() ||
		!targetOK || !actualOK || !expectedOK || !refOK || !engine.TargetMatchesRef(target, ref) || !rule.owner.Algebra().Equal(actual, expected) {
		return engine.RuleEvidence{}, false
	}
	return derivation.Accept()
}
