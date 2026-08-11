// Package selectrule declares Value's guarded Lua and/or result Rules.
package selectrule

import (
	"github.com/wippyai/go-lua/analysis/domain/value"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	"github.com/wippyai/go-lua/analysis/engine"
)

// LeftRule handles the selected-left arm. Its result is filtered by the exact
// left truth edge, so `x or y` cannot return a falsy x along its truthy arm.
type LeftRule struct {
	rule     *engine.Rule[value.Value, value.SelectBranch]
	semantic engine.SemanticKey
	owner    *valueowner.Owner
	read     engine.Read[engine.OrderedCells[value.Value]]
	write    engine.Write[value.Value]
}

func DeclareLeft(composition *engine.Composition, semantic, operandFamily, evidence engine.SemanticKey, owner *valueowner.Owner) (*LeftRule, bool) {
	if !validDeclaration(composition, semantic, operandFamily, evidence, owner) {
		return nil, false
	}
	declaration := &LeftRule{semantic: semantic, owner: owner}
	rule, ok := engine.DeclareRule(composition, engine.RuleSpec[value.Value, value.SelectBranch]{
		Semantic: semantic, OperandFamily: operandFamily, OperandContent: selectContent,
		Output: owner.Output(), Inputs: 1, Admission: engine.AdmitRuleByDerivation(evidence, declaration.check), Transfer: declaration.transfer,
	}, func(rule *engine.Rule[value.Value, value.SelectBranch]) bool {
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

func (rule *LeftRule) Instance(operand value.SelectBranch) (*engine.RuleInstance[value.Value, value.SelectBranch], bool) {
	if rule == nil || rule.rule == nil || !validLeft(rule.owner, operand) {
		return nil, false
	}
	result, left, _, _, _, ok := operand.Endpoints()
	if !ok {
		return nil, false
	}
	leftRef, leftOK := rule.owner.Locate(left)
	resultRef, resultOK := rule.owner.Locate(result)
	if !leftOK || !resultOK {
		return nil, false
	}
	return engine.NewRuleInstance(rule.rule, operand, func(binding *engine.RuleBinding[value.Value, value.SelectBranch]) bool {
		return engine.InstanceRead(binding, rule.read, leftRef) && engine.InstanceWrite(binding, rule.write, resultRef)
	})
}

func (rule *LeftRule) result(operand value.SelectBranch, left value.Value) (value.Value, bool) {
	if rule == nil || !validLeft(rule.owner, operand) {
		return value.Value{}, false
	}
	_, _, _, truthy, _, ok := operand.Endpoints()
	if !ok {
		return value.Value{}, false
	}
	return rule.owner.Schema().FilterTruth(left, truthy)
}

func (rule *LeftRule) transfer(access engine.Access[value.Value, value.SelectBranch]) bool {
	operand, ok := engine.Operand(access)
	if !ok || !validLeft(rule.owner, operand) {
		return false
	}
	return engine.Product(access, func(row engine.Row) bool {
		cells, ok := engine.ReadValue(access, row, rule.read)
		if !ok || cells.Count() != 1 {
			return false
		}
		left, present, available := cells.At(0)
		if !available {
			return false
		}
		if !present {
			return engine.NoCandidate(access, row)
		}
		result, ok := rule.result(operand, left)
		if !ok || rule.owner.Schema().Equal(result, rule.owner.Schema().Bottom()) {
			return engine.NoCandidate(access, row)
		}
		return engine.StageValue(access, row, result)
	})
}

func (rule *LeftRule) check(derivation engine.RuleDerivation[value.Value, value.SelectBranch]) (engine.RuleEvidence, bool) {
	if rule == nil || derivation.Rule() != rule.semantic || derivation.InputCount() != 1 || derivation.ReadCount() != 1 || derivation.DispositionCount() != 1 {
		return engine.RuleEvidence{}, false
	}
	operand, operandOK := derivation.Operand()
	input, inputOK := derivation.InputAt(0)
	if !operandOK || !inputOK || input.Guard().Empty() || !validLeft(rule.owner, operand) || !matchesContent(derivation, operand) {
		return engine.RuleEvidence{}, false
	}
	result, left, _, _, _, endpointsOK := operand.Endpoints()
	leftRef, leftOK := rule.owner.Locate(left)
	resultRef, resultOK := rule.owner.Locate(result)
	if !endpointsOK || !leftOK || !resultOK || !engine.DerivationReadMatchesRef(derivation, rule.read, leftRef) {
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
	leftFact, present, available := cells.At(0)
	if !available {
		return engine.RuleEvidence{}, false
	}
	expected, expectedOK := rule.result(operand, leftFact)
	if !present || !expectedOK || rule.owner.Schema().Equal(expected, rule.owner.Schema().Bottom()) {
		if disposition.Kind() != engine.RuleDispositionNoCandidate || disposition.TargetCount() != 0 {
			return engine.RuleEvidence{}, false
		}
		return derivation.Accept()
	}
	target, targetOK := disposition.TargetAt(0)
	actual, actualOK := disposition.Value()
	if disposition.Kind() != engine.RuleDispositionStaged || disposition.TargetCount() != 1 || !targetOK || !actualOK ||
		!engine.TargetMatchesRef(target, resultRef) || !rule.owner.Schema().Equal(actual, expected) {
		return engine.RuleEvidence{}, false
	}
	return derivation.Accept()
}

// RightRule handles the selected-right arm. It reads the left solely as the
// branch feasibility proof and transports the right fact unchanged.
type RightRule struct {
	rule       *engine.Rule[value.Value, value.SelectBranch]
	semantic   engine.SemanticKey
	owner      *valueowner.Owner
	leftRead   engine.Read[engine.OrderedCells[value.Value]]
	chosenRead engine.Read[engine.OrderedCells[value.Value]]
	write      engine.Write[value.Value]
}

func DeclareRight(composition *engine.Composition, semantic, operandFamily, evidence engine.SemanticKey, owner *valueowner.Owner) (*RightRule, bool) {
	if !validDeclaration(composition, semantic, operandFamily, evidence, owner) {
		return nil, false
	}
	declaration := &RightRule{semantic: semantic, owner: owner}
	rule, ok := engine.DeclareRule(composition, engine.RuleSpec[value.Value, value.SelectBranch]{
		Semantic: semantic, OperandFamily: operandFamily, OperandContent: selectContent,
		Output: owner.Output(), Inputs: 1, Admission: engine.AdmitRuleByDerivation(evidence, declaration.check), Transfer: declaration.transfer,
	}, func(rule *engine.Rule[value.Value, value.SelectBranch]) bool {
		input, inputOK := rule.InputAt(0)
		left, leftOK := engine.ReadFrom(rule, input, owner.ExactRead())
		chosen, chosenOK := engine.ReadFrom(rule, input, owner.ExactRead())
		write, writeOK := engine.WriteTo(rule, owner.ExactWrite())
		if !inputOK || !leftOK || !chosenOK || !writeOK || !engine.CarryFrom(rule, input, owner.Carry()) {
			return false
		}
		declaration.rule, declaration.leftRead, declaration.chosenRead, declaration.write = rule, left, chosen, write
		return true
	})
	if !ok || rule == nil || declaration.rule != rule {
		return nil, false
	}
	return declaration, true
}

func (rule *RightRule) Instance(operand value.SelectBranch) (*engine.RuleInstance[value.Value, value.SelectBranch], bool) {
	if rule == nil || rule.rule == nil || !validRight(rule.owner, operand) {
		return nil, false
	}
	result, left, chosen, _, _, ok := operand.Endpoints()
	if !ok {
		return nil, false
	}
	leftRef, leftOK := rule.owner.Locate(left)
	chosenRef, chosenOK := rule.owner.Locate(chosen)
	resultRef, resultOK := rule.owner.Locate(result)
	if !leftOK || !chosenOK || !resultOK {
		return nil, false
	}
	return engine.NewRuleInstance(rule.rule, operand, func(binding *engine.RuleBinding[value.Value, value.SelectBranch]) bool {
		return engine.InstanceRead(binding, rule.leftRead, leftRef) && engine.InstanceRead(binding, rule.chosenRead, chosenRef) && engine.InstanceWrite(binding, rule.write, resultRef)
	})
}

func (rule *RightRule) enabled(operand value.SelectBranch, left value.Value) bool {
	if rule == nil || !validRight(rule.owner, operand) {
		return false
	}
	_, _, _, truthy, _, ok := operand.Endpoints()
	if !ok {
		return false
	}
	filtered, ok := rule.owner.Schema().FilterTruth(left, truthy)
	return ok && !rule.owner.Schema().Equal(filtered, rule.owner.Schema().Bottom())
}

func (rule *RightRule) transfer(access engine.Access[value.Value, value.SelectBranch]) bool {
	operand, ok := engine.Operand(access)
	if !ok || !validRight(rule.owner, operand) {
		return false
	}
	return engine.Product(access, func(row engine.Row) bool {
		leftCells, leftOK := engine.ReadValue(access, row, rule.leftRead)
		chosenCells, chosenOK := engine.ReadValue(access, row, rule.chosenRead)
		if !leftOK || !chosenOK || leftCells.Count() != 1 || chosenCells.Count() != 1 {
			return false
		}
		left, leftPresent, leftAvailable := leftCells.At(0)
		chosen, chosenPresent, chosenAvailable := chosenCells.At(0)
		if !leftAvailable || !chosenAvailable {
			return false
		}
		if !leftPresent || !chosenPresent || !rule.enabled(operand, left) {
			return engine.NoCandidate(access, row)
		}
		return engine.StageValue(access, row, chosen)
	})
}

func (rule *RightRule) check(derivation engine.RuleDerivation[value.Value, value.SelectBranch]) (engine.RuleEvidence, bool) {
	if rule == nil || derivation.Rule() != rule.semantic || derivation.InputCount() != 1 || derivation.ReadCount() != 2 || derivation.DispositionCount() != 1 {
		return engine.RuleEvidence{}, false
	}
	operand, operandOK := derivation.Operand()
	input, inputOK := derivation.InputAt(0)
	if !operandOK || !inputOK || input.Guard().Empty() || !validRight(rule.owner, operand) || !matchesContent(derivation, operand) {
		return engine.RuleEvidence{}, false
	}
	result, left, chosen, _, _, endpointsOK := operand.Endpoints()
	leftRef, leftOK := rule.owner.Locate(left)
	chosenRef, chosenOK := rule.owner.Locate(chosen)
	resultRef, resultOK := rule.owner.Locate(result)
	if !endpointsOK || !leftOK || !chosenOK || !resultOK || !engine.DerivationReadMatchesRef(derivation, rule.leftRead, leftRef) || !engine.DerivationReadMatchesRef(derivation, rule.chosenRead, chosenRef) {
		return engine.RuleEvidence{}, false
	}
	disposition, dispositionOK := derivation.DispositionAt(0)
	if !dispositionOK || disposition.Guard().Empty() || !disposition.Guard().Same(input.Guard()) {
		return engine.RuleEvidence{}, false
	}
	leftCells, leftCellsOK := engine.DerivationDispositionReadValue(derivation, disposition, rule.leftRead)
	chosenCells, chosenCellsOK := engine.DerivationDispositionReadValue(derivation, disposition, rule.chosenRead)
	if !leftCellsOK || !chosenCellsOK || leftCells.Count() != 1 || chosenCells.Count() != 1 {
		return engine.RuleEvidence{}, false
	}
	leftFact, leftPresent, leftAvailable := leftCells.At(0)
	chosenFact, chosenPresent, chosenAvailable := chosenCells.At(0)
	if !leftAvailable || !chosenAvailable {
		return engine.RuleEvidence{}, false
	}
	if !leftPresent || !chosenPresent || !rule.enabled(operand, leftFact) {
		if disposition.Kind() != engine.RuleDispositionNoCandidate || disposition.TargetCount() != 0 {
			return engine.RuleEvidence{}, false
		}
		return derivation.Accept()
	}
	target, targetOK := disposition.TargetAt(0)
	actual, actualOK := disposition.Value()
	if disposition.Kind() != engine.RuleDispositionStaged || disposition.TargetCount() != 1 || !targetOK || !actualOK ||
		!engine.TargetMatchesRef(target, resultRef) || !rule.owner.Schema().Equal(actual, chosenFact) {
		return engine.RuleEvidence{}, false
	}
	return derivation.Accept()
}

func validDeclaration(composition *engine.Composition, semantic, operandFamily, evidence engine.SemanticKey, owner *valueowner.Owner) bool {
	return composition != nil && owner != nil && owner.Schema() != nil && semantic.Available() && operandFamily.Available() && evidence.Available() &&
		semantic != operandFamily && semantic != evidence && operandFamily != evidence
}

func selectContent(operand value.SelectBranch) (value.SelectBranch, [32]byte, bool) {
	id, ok := operand.ID()
	return operand, [32]byte(id), ok && [32]byte(id) != [32]byte{}
}

func matchesContent(derivation engine.RuleDerivation[value.Value, value.SelectBranch], operand value.SelectBranch) bool {
	id, ok := operand.ID()
	return ok && derivation.OperandContentMatches([32]byte(id))
}

func validLeft(owner *valueowner.Owner, operand value.SelectBranch) bool {
	if owner == nil || owner.Schema() == nil {
		return false
	}
	_, _, _, _, left, endpoints := operand.Endpoints()
	id, idOK := operand.ID()
	return endpoints && left && idOK && id.Available() && owner.Schema().OwnsSelectBranch(operand)
}

func validRight(owner *valueowner.Owner, operand value.SelectBranch) bool {
	if owner == nil || owner.Schema() == nil {
		return false
	}
	_, _, _, _, left, endpoints := operand.Endpoints()
	id, idOK := operand.ID()
	return endpoints && !left && idOK && id.Available() && owner.Schema().OwnsSelectBranch(operand)
}
