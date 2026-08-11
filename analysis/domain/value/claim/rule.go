// Package claim declares Value's identity-preserving source-claim Rule.
package claim

import (
	"github.com/wippyai/go-lua/analysis/domain/value"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
)

// Rule transfers the exact runtime relation through `as`, `::`, and postfix
// non-nil claims. The static target remains a Link/Static authority on the
// operand; this Rule deliberately neither decodes it nor invents a type
// refinement. Program declares claims as identity-preserving expressions.
type Rule struct {
	rule     *engine.Rule[value.Value, value.ValueClaim]
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
	rule, ok := engine.DeclareRule(composition, engine.RuleSpec[value.Value, value.ValueClaim]{
		Semantic: semantic, OperandFamily: operandFamily, OperandContent: claimContent,
		Output: owner.Output(), Inputs: 1, Admission: engine.AdmitRuleByDerivation(evidence, declaration.check), Transfer: declaration.transfer,
	}, func(rule *engine.Rule[value.Value, value.ValueClaim]) bool {
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

func (rule *Rule) Instance(claim value.ValueClaim) (*engine.RuleInstance[value.Value, value.ValueClaim], bool) {
	if rule == nil || rule.rule == nil || !validClaim(rule.owner, claim) {
		return nil, false
	}
	result, operand, ok := claim.Endpoints()
	if !ok {
		return nil, false
	}
	operandRef, operandOK := rule.owner.Locate(operand)
	resultRef, resultOK := rule.owner.Locate(result)
	if !operandOK || !resultOK {
		return nil, false
	}
	return engine.NewRuleInstance(rule.rule, claim, func(binding *engine.RuleBinding[value.Value, value.ValueClaim]) bool {
		return engine.InstanceRead(binding, rule.read, operandRef) && engine.InstanceWrite(binding, rule.write, resultRef)
	})
}

func claimContent(claim value.ValueClaim) (value.ValueClaim, [32]byte, bool) {
	id, ok := claim.ID()
	return claim, [32]byte(id), ok && [32]byte(id) != [32]byte{}
}

func validClaim(owner *valueowner.Owner, claim value.ValueClaim) bool {
	if owner == nil || owner.Schema() == nil {
		return false
	}
	id, idOK := claim.ID()
	if !idOK || !id.Available() || !owner.Schema().OwnsValueClaim(claim) {
		return false
	}
	kind, kindOK := claim.Kind()
	_, targetOK := claim.StaticTarget()
	if !kindOK {
		return false
	}
	switch kind {
	case flowkind.ValueClaimTypeAs, flowkind.ValueClaimTypeColonColon:
		return targetOK
	case flowkind.ValueClaimNonNil:
		return !targetOK
	default:
		return false
	}
}

func (rule *Rule) transfer(access engine.Access[value.Value, value.ValueClaim]) bool {
	claim, ok := engine.Operand(access)
	if !ok || !validClaim(rule.owner, claim) {
		return false
	}
	return engine.Product(access, func(row engine.Row) bool {
		cells, ok := engine.ReadValue(access, row, rule.read)
		if !ok || cells.Count() != 1 {
			return false
		}
		fact, present, available := cells.At(0)
		if !available {
			return false
		}
		if !present {
			return engine.NoCandidate(access, row)
		}
		return engine.StageValue(access, row, fact)
	})
}

func (rule *Rule) check(derivation engine.RuleDerivation[value.Value, value.ValueClaim]) (engine.RuleEvidence, bool) {
	if rule == nil || derivation.Rule() != rule.semantic || derivation.InputCount() != 1 || derivation.ReadCount() != 1 || derivation.DispositionCount() != 1 {
		return engine.RuleEvidence{}, false
	}
	claim, claimOK := derivation.Operand()
	input, inputOK := derivation.InputAt(0)
	if !claimOK || !inputOK || input.Guard().Empty() || !validClaim(rule.owner, claim) {
		return engine.RuleEvidence{}, false
	}
	id, idOK := claim.ID()
	result, operand, endpointsOK := claim.Endpoints()
	operandRef, operandOK := rule.owner.Locate(operand)
	resultRef, resultOK := rule.owner.Locate(result)
	if !idOK || !endpointsOK || !operandOK || !resultOK || !derivation.OperandContentMatches([32]byte(id)) ||
		!engine.DerivationReadMatchesRef(derivation, rule.read, operandRef) {
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
	target, targetOK := disposition.TargetAt(0)
	actual, actualOK := disposition.Value()
	if disposition.Kind() != engine.RuleDispositionStaged || disposition.TargetCount() != 1 || !targetOK || !actualOK ||
		!engine.TargetMatchesRef(target, resultRef) || !rule.owner.Schema().Equal(actual, fact) {
		return engine.RuleEvidence{}, false
	}
	return derivation.Accept()
}
