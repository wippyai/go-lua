// Package source declares Value's unconditional source-seed Rule. It owns no
// Program/Link enumeration or runtime binding: callers bind already-issued
// value.SourceSeed operands and exact Value coordinates during Wave E.
package source

import (
	"github.com/wippyai/go-lua/analysis/domain/value"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	"github.com/wippyai/go-lua/analysis/engine"
)

// Rule owns the sole Value source declaration and its exact output token.
// The raw engine Rule is private so callers cannot pair a SourceSeed with a
// different operand identity or target surface.
type Rule struct {
	rule  *engine.Rule[value.Value, value.SourceSeed]
	write engine.Write[value.Value]
	owner *valueowner.Owner
}

// Declare records exactly one zero-input Value source Rule. All semantic
// identities are supplied by the composition root; this child owns no key
// registry or implicit default identity.
func Declare(
	composition *engine.Composition,
	ruleSemantic engine.SemanticKey,
	operandFamily engine.SemanticKey,
	evidenceSemantic engine.SemanticKey,
	owner *valueowner.Owner,
) (*Rule, bool) {
	if composition == nil || owner == nil || owner.Schema() == nil ||
		!ruleSemantic.Available() || !operandFamily.Available() || !evidenceSemantic.Available() ||
		ruleSemantic == operandFamily || ruleSemantic == evidenceSemantic || operandFamily == evidenceSemantic {
		return nil, false
	}

	checker := sourceChecker(owner, ruleSemantic)
	var write engine.Write[value.Value]
	declared, ok := engine.DeclareRule(composition, engine.RuleSpec[value.Value, value.SourceSeed]{
		Semantic:       ruleSemantic,
		OperandFamily:  operandFamily,
		OperandContent: sourceSeedContent,
		Output:         owner.Output(),
		Inputs:         0,
		Admission:      engine.AdmitRuleByDerivation(evidenceSemantic, checker),
		Transfer: func(access engine.Access[value.Value, value.SourceSeed]) bool {
			seed, ok := engine.Operand(access)
			if !ok {
				return false
			}
			_, fact, ok := sourceResult(owner, seed)
			if !ok {
				return false
			}
			rows := 0
			completed := engine.Product(access, func(row engine.Row) bool {
				rows++
				return rows == 1 && engine.StageValue(access, row, fact)
			})
			return completed && rows == 1
		},
	}, func(rule *engine.Rule[value.Value, value.SourceSeed]) bool {
		var written bool
		write, written = engine.WriteTo(rule, owner.ExactWrite())
		return written
	})
	if !ok || declared == nil {
		return nil, false
	}
	return &Rule{rule: declared, write: write, owner: owner}, true
}

// Instance derives the complete typed binding from one canonical SourceSeed.
// The seed identity, operand payload, and exact Value target cannot be supplied
// independently by the caller.
func (rule *Rule) Instance(seed value.SourceSeed) (*engine.RuleInstance[value.Value, value.SourceSeed], bool) {
	if rule == nil || rule.rule == nil || rule.owner == nil {
		return nil, false
	}
	coordinate, _, resultOK := sourceResult(rule.owner, seed)
	if !resultOK {
		return nil, false
	}
	ref, ok := rule.owner.Locate(coordinate)
	if !ok {
		return nil, false
	}
	return engine.NewRuleInstance(rule.rule, seed, func(binding *engine.RuleBinding[value.Value, value.SourceSeed]) bool {
		return engine.InstanceWrite(binding, rule.write, ref)
	})
}

func sourceSeedContent(seed value.SourceSeed) (value.SourceSeed, [32]byte, bool) {
	id, ok := seed.ID()
	return seed, [32]byte(id), ok && [32]byte(id) != [32]byte{}
}

// sourceResult is the common transfer/checker reconstruction. AdmitsCoordinate
// is the owner fence: a valid seed from another Schema cannot supply either a
// coordinate or a fact to this Rule.
func sourceResult(owner *valueowner.Owner, seed value.SourceSeed) (value.Coordinate, value.Value, bool) {
	if owner == nil || owner.Schema() == nil {
		return value.Coordinate{}, value.Value{}, false
	}
	coordinate, fact, ok := seed.Result()
	schema := owner.Schema()
	if !ok || !schema.AdmitsCoordinate(coordinate, fact) || schema.Equal(fact, schema.Default()) {
		return value.Coordinate{}, value.Value{}, false
	}
	return coordinate, fact, true
}

func sourceFactMatches(owner *valueowner.Owner, seed value.SourceSeed, actual value.Value) bool {
	_, expected, ok := sourceResult(owner, seed)
	return ok && owner.Schema().Equal(actual, expected)
}

func sourceChecker(owner *valueowner.Owner, ruleSemantic engine.SemanticKey) engine.RuleDerivationChecker[value.Value, value.SourceSeed] {
	return func(derivation engine.RuleDerivation[value.Value, value.SourceSeed]) (engine.RuleEvidence, bool) {
		if derivation.Rule() != ruleSemantic || derivation.InputCount() != 0 || derivation.ReadCount() != 0 || derivation.DispositionCount() != 1 {
			return engine.RuleEvidence{}, false
		}
		seed, ok := derivation.Operand()
		if !ok {
			return engine.RuleEvidence{}, false
		}
		id, idOK := seed.ID()
		if !idOK || !derivation.OperandContentMatches([32]byte(id)) {
			return engine.RuleEvidence{}, false
		}
		disposition, ok := derivation.DispositionAt(0)
		if !ok || disposition.Kind() != engine.RuleDispositionStaged || disposition.TargetCount() != 1 || disposition.Guard().Empty() {
			return engine.RuleEvidence{}, false
		}
		if _, ok := disposition.TargetAt(0); !ok {
			return engine.RuleEvidence{}, false
		}
		coordinate, _, coordinateOK := sourceResult(owner, seed)
		ref, refOK := owner.Locate(coordinate)
		target, targetOK := disposition.TargetAt(0)
		if !coordinateOK || !refOK || !targetOK || !engine.TargetMatchesRef(target, ref) {
			return engine.RuleEvidence{}, false
		}
		actual, ok := disposition.Value()
		if !ok || !sourceFactMatches(owner, seed, actual) {
			return engine.RuleEvidence{}, false
		}
		return derivation.Accept()
	}
}
