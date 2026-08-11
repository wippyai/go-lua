// Package unarynot declares Value's exact logical-not judgment.
package unarynot

import (
	"github.com/wippyai/go-lua/analysis/domain/value"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	"github.com/wippyai/go-lua/analysis/engine"
)

type Rule struct {
	rule     *engine.Rule[value.Value, value.UnaryNot]
	semantic engine.SemanticKey
	owner    *valueowner.Owner
	read     engine.Read[engine.OrderedCells[value.Value]]
	write    engine.Write[value.Value]
}

func Declare(composition *engine.Composition, semantic, operandFamily, evidence engine.SemanticKey, owner *valueowner.Owner) (*Rule, bool) {
	if composition == nil || owner == nil || owner.Schema() == nil || !semantic.Available() || !operandFamily.Available() || !evidence.Available() ||
		semantic == operandFamily || semantic == evidence || operandFamily == evidence {
		return nil, false
	}
	declaration := &Rule{semantic: semantic, owner: owner}
	rule, ok := engine.DeclareRule(composition, engine.RuleSpec[value.Value, value.UnaryNot]{
		Semantic: semantic, OperandFamily: operandFamily, OperandContent: operandContent,
		Output: owner.Output(), Inputs: 1, Admission: engine.AdmitRuleByDerivation(evidence, declaration.check), Transfer: declaration.transfer,
	}, func(rule *engine.Rule[value.Value, value.UnaryNot]) bool {
		input, inputOK := rule.InputAt(0)
		read, readOK := engine.ReadFrom(rule, input, owner.ExactRead())
		write, writeOK := engine.WriteTo(rule, owner.ExactWrite())
		if !inputOK || !readOK || !writeOK || !engine.CarryFrom(rule, input, owner.Carry()) {
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

func (rule *Rule) Instance(operand value.UnaryNot) (*engine.RuleInstance[value.Value, value.UnaryNot], bool) {
	if rule == nil || rule.rule == nil || rule.owner == nil || !validOperand(rule.owner, operand) {
		return nil, false
	}
	result, input, ok := operand.Endpoints()
	if !ok {
		return nil, false
	}
	inputRef, inputOK := rule.owner.Locate(input)
	resultRef, resultOK := rule.owner.Locate(result)
	if !inputOK || !resultOK {
		return nil, false
	}
	return engine.NewRuleInstance(rule.rule, operand, func(binding *engine.RuleBinding[value.Value, value.UnaryNot]) bool {
		return engine.InstanceRead(binding, rule.read, inputRef) && engine.InstanceWrite(binding, rule.write, resultRef)
	})
}

func operandContent(operand value.UnaryNot) (value.UnaryNot, [32]byte, bool) {
	id, ok := operand.ID()
	return operand, [32]byte(id), ok && [32]byte(id) != [32]byte{}
}

func validOperand(owner *valueowner.Owner, operand value.UnaryNot) bool {
	if owner == nil || owner.Schema() == nil {
		return false
	}
	id, idOK := operand.ID()
	return idOK && id.Available() && owner.Schema().OwnsUnaryNot(operand)
}

func (rule *Rule) result(operand value.UnaryNot, input value.Value) (value.Value, bool) {
	if rule == nil || !validOperand(rule.owner, operand) || rule.owner == nil || rule.owner.Schema() == nil {
		return value.Value{}, false
	}
	return rule.owner.Schema().Not(input)
}

func (rule *Rule) transfer(access engine.Access[value.Value, value.UnaryNot]) bool {
	operand, ok := engine.Operand(access)
	if !ok || !validOperand(rule.owner, operand) {
		return false
	}
	return engine.Product(access, func(row engine.Row) bool {
		cells, ok := engine.ReadValue(access, row, rule.read)
		if !ok || cells.Count() != 1 {
			return false
		}
		input, present, available := cells.At(0)
		if !available {
			return false
		}
		if !present {
			return engine.NoCandidate(access, row)
		}
		result, ok := rule.result(operand, input)
		return ok && engine.StageValue(access, row, result)
	})
}

func (rule *Rule) check(derivation engine.RuleDerivation[value.Value, value.UnaryNot]) (engine.RuleEvidence, bool) {
	if rule == nil || rule.owner == nil || derivation.Rule() != rule.semantic || derivation.InputCount() != 1 || derivation.ReadCount() != 1 || derivation.DispositionCount() != 1 {
		return engine.RuleEvidence{}, false
	}
	operand, operandOK := derivation.Operand()
	input, inputOK := derivation.InputAt(0)
	if !operandOK || !inputOK || input.Guard().Empty() || !validOperand(rule.owner, operand) {
		return engine.RuleEvidence{}, false
	}
	id, idOK := operand.ID()
	_, source, endpointsOK := operand.Endpoints()
	sourceRef, sourceOK := rule.owner.Locate(source)
	if !idOK || !derivation.OperandContentMatches([32]byte(id)) || !endpointsOK || !sourceOK || !engine.DerivationReadMatchesRef(derivation, rule.read, sourceRef) {
		return engine.RuleEvidence{}, false
	}
	disposition, dispositionOK := derivation.DispositionAt(0)
	if !dispositionOK || disposition.Guard().Empty() || !disposition.Guard().Same(input.Guard()) {
		return engine.RuleEvidence{}, false
	}
	cells, cellsOK := engine.DerivationDispositionReadValue(derivation, disposition, rule.read)
	if !cellsOK || cells.Count() != 1 {
		return engine.RuleEvidence{}, false
	}
	fact, present, available := cells.At(0)
	if !available {
		return engine.RuleEvidence{}, false
	}
	if !present {
		if disposition.Kind() != engine.RuleDispositionNoCandidate || disposition.TargetCount() != 0 {
			return engine.RuleEvidence{}, false
		}
		return derivation.Accept()
	}
	result, resultOK := rule.result(operand, fact)
	destination, _, endpointsOK := operand.Endpoints()
	destinationRef, destinationOK := rule.owner.Locate(destination)
	target, targetOK := disposition.TargetAt(0)
	actual, actualOK := disposition.Value()
	if !resultOK || !endpointsOK || !destinationOK || disposition.Kind() != engine.RuleDispositionStaged || disposition.TargetCount() != 1 ||
		!targetOK || !actualOK || !engine.TargetMatchesRef(target, destinationRef) || !rule.owner.Schema().Equal(actual, result) {
		return engine.RuleEvidence{}, false
	}
	return derivation.Accept()
}
