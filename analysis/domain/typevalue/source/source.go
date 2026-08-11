// Package source declares TypeValue's sole binder-authorized source Rule.
// It consumes an Authority-issued Seed and writes the matching existing
// TypeValue root; no spelling, static reference, or ordinary Value may become
// a seed through this package.
package source

import (
	"github.com/wippyai/go-lua/analysis/domain/typevalue"
	typevalueowner "github.com/wippyai/go-lua/analysis/domain/typevalue/owner"
	"github.com/wippyai/go-lua/analysis/engine"
)

// Rule retains only the typed write capability for TypeValue's sealed source
// relation.  The raw engine Rule is private so callers cannot detach a Seed
// from its Authority-owned root and singleton descriptor value.
type Rule struct {
	rule  *engine.Rule[typevalue.Value, typevalue.Seed]
	write engine.Write[typevalue.Value]
	owner *typevalueowner.Owner
}

// Declare records the zero-input runtime-TypeValue seed judgment.  All
// semantic identities are supplied by the future composition owner; this
// child declares neither a registry nor a second seed relation.
func Declare(
	composition *engine.Composition,
	ruleSemantic engine.SemanticKey,
	operandFamily engine.SemanticKey,
	evidenceSemantic engine.SemanticKey,
	owner *typevalueowner.Owner,
) (*Rule, bool) {
	if composition == nil || owner == nil || owner.Authority() == nil ||
		!ruleSemantic.Available() || !operandFamily.Available() || !evidenceSemantic.Available() ||
		ruleSemantic == operandFamily || ruleSemantic == evidenceSemantic || operandFamily == evidenceSemantic {
		return nil, false
	}
	var write engine.Write[typevalue.Value]
	declared, ok := engine.DeclareRule(composition, engine.RuleSpec[typevalue.Value, typevalue.Seed]{
		Semantic:       ruleSemantic,
		OperandFamily:  operandFamily,
		OperandContent: seedContent(owner),
		Output:         owner.Output(),
		Inputs:         0,
		Admission:      engine.AdmitRuleByDerivation(evidenceSemantic, checker(owner, ruleSemantic)),
		Transfer: func(access engine.Access[typevalue.Value, typevalue.Seed]) bool {
			seed, ok := engine.Operand(access)
			if !ok {
				return false
			}
			_, fact, ok := seedResult(owner, seed)
			if !ok {
				return false
			}
			rows := 0
			complete := engine.Product(access, func(row engine.Row) bool {
				rows++
				return rows == 1 && engine.StageValue(access, row, fact)
			})
			return complete && rows == 1
		},
	}, func(rule *engine.Rule[typevalue.Value, typevalue.Seed]) bool {
		var written bool
		write, written = engine.WriteTo(rule, owner.ExactWrite())
		return written
	})
	if !ok || declared == nil {
		return nil, false
	}
	return &Rule{rule: declared, write: write, owner: owner}, true
}

// Instance derives the atomic binding from the exact Authority-issued Seed.
func (rule *Rule) Instance(seed typevalue.Seed) (*engine.RuleInstance[typevalue.Value, typevalue.Seed], bool) {
	if rule == nil || rule.rule == nil || rule.owner == nil {
		return nil, false
	}
	root, _, ok := seedResult(rule.owner, seed)
	if !ok {
		return nil, false
	}
	ref, ok := rule.owner.Locate(root)
	if !ok {
		return nil, false
	}
	return engine.NewRuleInstance(rule.rule, seed, func(binding *engine.RuleBinding[typevalue.Value, typevalue.Seed]) bool {
		return engine.InstanceWrite(binding, rule.write, ref)
	})
}

func seedContent(owner *typevalueowner.Owner) func(typevalue.Seed) (typevalue.Seed, [32]byte, bool) {
	return func(seed typevalue.Seed) (typevalue.Seed, [32]byte, bool) {
		if owner == nil || owner.Authority() == nil {
			return typevalue.Seed{}, [32]byte{}, false
		}
		id, ok := owner.Authority().SeedID(seed)
		return seed, [32]byte(id), ok && [32]byte(id) != [32]byte{}
	}
}

func seedResult(owner *typevalueowner.Owner, seed typevalue.Seed) (typevalue.Root, typevalue.Value, bool) {
	if owner == nil || owner.Authority() == nil {
		return typevalue.Root{}, typevalue.Value{}, false
	}
	return sourceResult(owner.Authority(), seed)
}

// sourceResult is the domain reduction behind the Rule.  Its only input is a
// sealed Authority and that Authority's Seed, so it cannot consult a
// composition, source spelling, or ordinary Value flow to manufacture a
// runtime TypeValue.
func sourceResult(authority *typevalue.Authority, seed typevalue.Seed) (typevalue.Root, typevalue.Value, bool) {
	if authority == nil {
		return typevalue.Root{}, typevalue.Value{}, false
	}
	root, fact, ok := authority.SeedValue(seed)
	if !ok || !authority.Owns(fact) || authority.Equal(fact, authority.Bottom()) {
		return typevalue.Root{}, typevalue.Value{}, false
	}
	return root, fact, true
}

func checker(owner *typevalueowner.Owner, ruleSemantic engine.SemanticKey) engine.RuleDerivationChecker[typevalue.Value, typevalue.Seed] {
	return func(derivation engine.RuleDerivation[typevalue.Value, typevalue.Seed]) (engine.RuleEvidence, bool) {
		if owner == nil || owner.Authority() == nil || derivation.Rule() != ruleSemantic ||
			derivation.InputCount() != 0 || derivation.ReadCount() != 0 || derivation.DispositionCount() != 1 {
			return engine.RuleEvidence{}, false
		}
		seed, ok := derivation.Operand()
		if !ok {
			return engine.RuleEvidence{}, false
		}
		id, ok := owner.Authority().SeedID(seed)
		if !ok || !derivation.OperandContentMatches([32]byte(id)) {
			return engine.RuleEvidence{}, false
		}
		root, expected, ok := seedResult(owner, seed)
		if !ok {
			return engine.RuleEvidence{}, false
		}
		ref, ok := owner.Locate(root)
		if !ok {
			return engine.RuleEvidence{}, false
		}
		disposition, ok := derivation.DispositionAt(0)
		if !ok || disposition.Kind() != engine.RuleDispositionStaged || disposition.TargetCount() != 1 || disposition.Guard().Empty() {
			return engine.RuleEvidence{}, false
		}
		target, ok := disposition.TargetAt(0)
		if !ok || !engine.TargetMatchesRef(target, ref) {
			return engine.RuleEvidence{}, false
		}
		actual, ok := disposition.Value()
		if !ok || !owner.Authority().Equal(actual, expected) {
			return engine.RuleEvidence{}, false
		}
		return derivation.Accept()
	}
}
