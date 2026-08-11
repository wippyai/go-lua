package rule

import (
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	typestateowner "github.com/wippyai/go-lua/analysis/domain/typestate/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/keyspace"
)

// TransitionOperand binds one opaque Typestate transition declaration. Its
// Contract row and selected outcome were derived from the exact formal
// ResourceOrigin and Application × operation coordinate before Rule admission.
type TransitionOperand struct{ transition typestate.Transition }

func NewTransitionOperand(schema typestate.Schema, transition typestate.Transition) (TransitionOperand, bool) {
	if !schema.ValidTransition(transition) {
		return TransitionOperand{}, false
	}
	return TransitionOperand{transition: transition}, true
}

func (operand TransitionOperand) OutcomeID() keyspace.ContentID {
	return operand.transition.ContentID()
}

type TransitionDeclaration struct {
	semantic engine.SemanticKey
	rule     *engine.Rule[typestate.Relation, TransitionOperand]
	owner    *typestateowner.Owner
	read     engine.Read[engine.OrderedCells[typestate.Relation]]
	write    engine.Write[typestate.Relation]
}

func DeclareTransition(composition *engine.Composition, semantic, operandFamily, evidence engine.SemanticKey, owner *typestateowner.Owner) (*TransitionDeclaration, bool) {
	if composition == nil || owner == nil || !semantic.Available() || !operandFamily.Available() || !evidence.Available() ||
		semantic == operandFamily || semantic == evidence || operandFamily == evidence {
		return nil, false
	}
	declaration := &TransitionDeclaration{semantic: semantic, owner: owner}
	rule, ok := engine.DeclareRule(composition, engine.RuleSpec[typestate.Relation, TransitionOperand]{
		Semantic: semantic, OperandFamily: operandFamily, OperandContent: transitionContent, Output: owner.Output(), Inputs: 1,
		Admission: engine.AdmitRuleByDerivation(evidence, declaration.check), Transfer: declaration.transfer,
	}, func(rule *engine.Rule[typestate.Relation, TransitionOperand]) bool {
		input, inputOK := rule.InputAt(0)
		read, readOK := engine.ReadFrom(rule, input, owner.Read())
		write, writeOK := engine.WriteTo(rule, owner.Write())
		if !inputOK || !readOK || !writeOK {
			return false
		}
		declaration.rule, declaration.read, declaration.write = rule, read, write
		return true
	})
	if !ok || rule == nil || declaration.rule != rule {
		return nil, false
	}
	return declaration, true
}

func (d *TransitionDeclaration) NewInstance(operand TransitionOperand) (*engine.RuleInstance[typestate.Relation, TransitionOperand], bool) {
	if d == nil || d.rule == nil || d.owner == nil || !validTransitionOperand(operand, d.owner.Schema()) {
		return nil, false
	}
	return engine.NewRuleInstance(d.rule, operand, func(binding *engine.RuleBinding[typestate.Relation, TransitionOperand]) bool {
		ref, ok := d.owner.Locate(operand.transition.Key())
		return ok && engine.InstanceRead(binding, d.read, ref) && engine.InstanceWrite(binding, d.write, ref)
	})
}

func transitionContent(operand TransitionOperand) (TransitionOperand, [32]byte, bool) {
	content := operand.transition.ContentID()
	if !content.Available() {
		return TransitionOperand{}, [32]byte{}, false
	}
	return operand, [32]byte(content), true
}

func validTransitionOperand(operand TransitionOperand, schema typestate.Schema) bool {
	return schema.ValidTransition(operand.transition)
}

func (d *TransitionDeclaration) transfer(access engine.Access[typestate.Relation, TransitionOperand]) bool {
	operand, ok := engine.Operand(access)
	if !ok || d == nil || d.owner == nil || !validTransitionOperand(operand, d.owner.Schema()) {
		return false
	}
	return engine.Product(access, func(row engine.Row) bool {
		cells, readOK := engine.ReadValue(access, row, d.read)
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
		next, reduced := d.owner.Algebra().Transition(typestate.Fact{Key: operand.transition.Key(), Value: current}, operand.transition)
		if !reduced || next.Key != operand.transition.Key() {
			return engine.NoCandidate(access, row)
		}
		return engine.StageValue(access, row, next.Value)
	})
}

func (d *TransitionDeclaration) check(derivation engine.RuleDerivation[typestate.Relation, TransitionOperand]) (engine.RuleEvidence, bool) {
	if d == nil || d.owner == nil || derivation.Rule() != d.semantic || derivation.InputCount() != 1 || derivation.ReadCount() != 1 || derivation.DispositionCount() != 1 {
		return engine.RuleEvidence{}, false
	}
	input, inputOK := derivation.InputAt(0)
	if !inputOK || input.Guard().Empty() {
		return engine.RuleEvidence{}, false
	}
	operand, operandOK := derivation.Operand()
	if !operandOK || !validTransitionOperand(operand, d.owner.Schema()) || !derivation.OperandContentMatches([32]byte(operand.transition.ContentID())) {
		return engine.RuleEvidence{}, false
	}
	ref, refOK := d.owner.Locate(operand.transition.Key())
	if !refOK || !engine.DerivationReadMatchesRef(derivation, d.read, ref) {
		return engine.RuleEvidence{}, false
	}
	disposition, dispositionOK := derivation.DispositionAt(0)
	cells, readOK := engine.DerivationDispositionReadValue(derivation, disposition, d.read)
	if !dispositionOK || !readOK || cells.Count() != 1 || !disposition.Guard().Same(input.Guard()) {
		return engine.RuleEvidence{}, false
	}
	current, present, cellOK := cells.At(0)
	if !cellOK {
		return engine.RuleEvidence{}, false
	}
	expected, reduced := d.owner.Algebra().Transition(typestate.Fact{Key: operand.transition.Key(), Value: current}, operand.transition)
	if !present || !reduced || expected.Key != operand.transition.Key() {
		if disposition.Kind() != engine.RuleDispositionNoCandidate || disposition.TargetCount() != 0 {
			return engine.RuleEvidence{}, false
		}
		return derivation.Accept()
	}
	actual, staged := disposition.Value()
	target, targetOK := disposition.TargetAt(0)
	if disposition.Kind() != engine.RuleDispositionStaged || !staged || disposition.TargetCount() != 1 || !targetOK || !d.owner.Algebra().Equal(actual, expected.Value) || !engine.TargetMatchesRef(target, ref) {
		return engine.RuleEvidence{}, false
	}
	return derivation.Accept()
}
