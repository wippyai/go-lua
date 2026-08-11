// Package seed declares Call's sealed dispatch seed.  The Call algebra owns
// open/complete status; this child merely transports that Link-derived fact
// to its exact Call source coordinate.
package seed

import (
	calldomain "github.com/wippyai/go-lua/analysis/domain/call"
	callowner "github.com/wippyai/go-lua/analysis/domain/call/owner"
	"github.com/wippyai/go-lua/analysis/engine"
)

// Rule is the zero-read source rule for the Call source sum. Only callback
// and resume arms are admitted here and seed an open relation. Application
// coordinates retain the algebra default Bottom and have no seed member, so
// selected dispatch is their sole producer.
type Rule struct {
	rule  *engine.Rule[calldomain.Value, calldomain.Key]
	write engine.Write[calldomain.Value]
	owner *callowner.Owner
}

func Declare(composition *engine.Composition, ruleSemantic, operandFamily, evidenceSemantic engine.SemanticKey, owner *callowner.Owner) (*Rule, bool) {
	if composition == nil || owner == nil || owner.Algebra() == nil || !owner.Algebra().Valid() || !ruleSemantic.Available() || !operandFamily.Available() || !evidenceSemantic.Available() ||
		ruleSemantic == operandFamily || ruleSemantic == evidenceSemantic || operandFamily == evidenceSemantic {
		return nil, false
	}
	var write engine.Write[calldomain.Value]
	declared, ok := engine.DeclareRule(composition, engine.RuleSpec[calldomain.Value, calldomain.Key]{
		Semantic: ruleSemantic, OperandFamily: operandFamily, OperandContent: content, Output: owner.Output(), Inputs: 0,
		Admission: engine.AdmitRuleByDerivation(evidenceSemantic, checker(owner, ruleSemantic)),
		Transfer: func(access engine.Access[calldomain.Value, calldomain.Key]) bool {
			key, ok := engine.Operand(access)
			if !ok {
				return false
			}
			_, value, ok := result(owner, key)
			if !ok {
				return false
			}
			rows := 0
			return engine.Product(access, func(row engine.Row) bool {
				rows++
				return rows == 1 && engine.StageValue(access, row, value)
			}) && rows == 1
		},
	}, func(rule *engine.Rule[calldomain.Value, calldomain.Key]) bool {
		var ok bool
		write, ok = engine.WriteTo(rule, owner.ExactWrite())
		return ok
	})
	if !ok || declared == nil {
		return nil, false
	}
	return &Rule{rule: declared, write: write, owner: owner}, true
}

func (rule *Rule) Instance(key calldomain.Key) (*engine.RuleInstance[calldomain.Value, calldomain.Key], bool) {
	if rule == nil || rule.rule == nil || rule.owner == nil || key.IsApplication() {
		return nil, false
	}
	_, _, ok := result(rule.owner, key)
	if !ok {
		return nil, false
	}
	ref, ok := rule.owner.Locate(key)
	if !ok {
		return nil, false
	}
	return engine.NewRuleInstance(rule.rule, key, func(binding *engine.RuleBinding[calldomain.Value, calldomain.Key]) bool {
		return engine.InstanceWrite(binding, rule.write, ref)
	})
}

func content(key calldomain.Key) (calldomain.Key, [32]byte, bool) {
	id, ok := key.ContentID()
	return key, [32]byte(id), ok && [32]byte(id) != [32]byte{}
}

func result(owner *callowner.Owner, key calldomain.Key) (calldomain.Key, calldomain.Value, bool) {
	if owner == nil || owner.Algebra() == nil || !key.Valid() || key.IsApplication() {
		return calldomain.Key{}, calldomain.Value{}, false
	}
	value, ok := owner.Algebra().Initial(key)
	return key, value, ok && owner.Algebra().Admits(key, value)
}

func checker(owner *callowner.Owner, semantic engine.SemanticKey) engine.RuleDerivationChecker[calldomain.Value, calldomain.Key] {
	return func(derivation engine.RuleDerivation[calldomain.Value, calldomain.Key]) (engine.RuleEvidence, bool) {
		if owner == nil || derivation.Rule() != semantic || derivation.InputCount() != 0 || derivation.ReadCount() != 0 || derivation.DispositionCount() != 1 {
			return engine.RuleEvidence{}, false
		}
		key, ok := derivation.Operand()
		id, idOK := key.ContentID()
		if !ok || !idOK || !derivation.OperandContentMatches([32]byte(id)) {
			return engine.RuleEvidence{}, false
		}
		_, expected, resultOK := result(owner, key)
		ref, refOK := owner.Locate(key)
		disposition, dispositionOK := derivation.DispositionAt(0)
		if !resultOK || !refOK || !dispositionOK || disposition.Kind() != engine.RuleDispositionStaged || disposition.TargetCount() != 1 || disposition.Guard().Empty() {
			return engine.RuleEvidence{}, false
		}
		target, targetOK := disposition.TargetAt(0)
		actual, actualOK := disposition.Value()
		if !targetOK || !actualOK || !engine.TargetMatchesRef(target, ref) || !owner.Algebra().Equal(actual, expected) {
			return engine.RuleEvidence{}, false
		}
		return derivation.Accept()
	}
}
