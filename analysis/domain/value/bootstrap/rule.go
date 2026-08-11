// Package bootstrap owns Value's host-global bootstrap judgment. It consumes
// only Host's exact GlobalBinding handle; all Program, Module, Boundary, and
// Target relations are rebuilt by the rule rather than copied into an operand.
package bootstrap

import (
	"github.com/wippyai/go-lua/analysis/domain/value"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	linkhost "github.com/wippyai/go-lua/program/link/host"
	"github.com/wippyai/go-lua/program/target"
)

// Rule is the sole zero-input Value bootstrap declaration for Host globals.
type Rule struct {
	rule  *engine.Rule[value.Value, linkhost.GlobalBinding]
	write engine.Write[value.Value]
	owner *valueowner.Owner
}

// Declare installs the exact Host-global bootstrap judgment.
func Declare(composition *engine.Composition, semantic, operandFamily, evidence engine.SemanticKey, owner *valueowner.Owner) (*Rule, bool) {
	if composition == nil || owner == nil || owner.Schema() == nil ||
		!semantic.Available() || !operandFamily.Available() || !evidence.Available() ||
		semantic == operandFamily || semantic == evidence || operandFamily == evidence {
		return nil, false
	}
	var write engine.Write[value.Value]
	declared, ok := engine.DeclareRule(composition, engine.RuleSpec[value.Value, linkhost.GlobalBinding]{
		Semantic:       semantic,
		OperandFamily:  operandFamily,
		OperandContent: globalContent(owner),
		Output:         owner.Output(),
		Inputs:         0,
		Admission:      engine.AdmitRuleByDerivation(evidence, checker(owner, semantic)),
		Transfer: func(access engine.Access[value.Value, linkhost.GlobalBinding]) bool {
			binding, ok := engine.Operand(access)
			if !ok {
				return false
			}
			result, ok := globalResult(owner, binding)
			if !ok {
				return false
			}
			rows := 0
			return engine.Product(access, func(row engine.Row) bool {
				rows++
				if rows != 1 {
					return false
				}
				if result.absent {
					return engine.NoCandidate(access, row)
				}
				return engine.StageValue(access, row, result.fact)
			}) && rows == 1
		},
	}, func(rule *engine.Rule[value.Value, linkhost.GlobalBinding]) bool {
		var ok bool
		write, ok = engine.WriteTo(rule, owner.ExactWrite())
		return ok
	})
	if !ok || declared == nil {
		return nil, false
	}
	return &Rule{rule: declared, write: write, owner: owner}, true
}

// Instance binds the one exact Value coordinate reconstructed from a Host
// GlobalBinding. No raw coordinate, initial value, boot root, or source row
// can be supplied independently.
func (rule *Rule) Instance(binding linkhost.GlobalBinding) (*engine.RuleInstance[value.Value, linkhost.GlobalBinding], bool) {
	if rule == nil || rule.rule == nil || rule.owner == nil {
		return nil, false
	}
	result, ok := globalResult(rule.owner, binding)
	if !ok {
		return nil, false
	}
	ref, ok := rule.owner.Locate(result.coordinate)
	if !ok {
		return nil, false
	}
	return engine.NewRuleInstance(rule.rule, binding, func(instance *engine.RuleBinding[value.Value, linkhost.GlobalBinding]) bool {
		return engine.InstanceWrite(instance, rule.write, ref)
	})
}

type result struct {
	coordinate value.Coordinate
	fact       value.Value
	absent     bool
}

func globalContent(owner *valueowner.Owner) func(linkhost.GlobalBinding) (linkhost.GlobalBinding, [32]byte, bool) {
	return func(binding linkhost.GlobalBinding) (linkhost.GlobalBinding, [32]byte, bool) {
		if owner == nil || owner.Schema() == nil || owner.Schema().Link() == nil {
			return linkhost.GlobalBinding{}, [32]byte{}, false
		}
		id, idOK := owner.Schema().Link().Host().Globals().ID(binding)
		if _, resultOK := globalResult(owner, binding); !idOK || !resultOK || !id.Available() {
			return linkhost.GlobalBinding{}, [32]byte{}, false
		}
		return binding, [32]byte(id), true
	}
}

// globalResult is the complete replay law shared by instance construction,
// transfer, and evidence. InitialValueAbsent is a valid source relation with
// no candidate; every other unavailable or malformed initial value fails.
func globalResult(owner *valueowner.Owner, binding linkhost.GlobalBinding) (result, bool) {
	if owner == nil || owner.Schema() == nil || owner.Schema().Link() == nil {
		return result{}, false
	}
	schema, linked := owner.Schema(), owner.Schema().Link()
	if linked.Host() == nil || linked.Module() == nil || linked.Boundary() == nil {
		return result{}, false
	}
	analysis, boot, cell, _, class, initial, mappingOK := linked.Host().Globals().Mapping(binding)
	if !mappingOK || class == target.InitialBindingInvalid || initial == 0 {
		return result{}, false
	}
	// Mapping is not sufficient as a hot-owner fence: replay the inverse on
	// this exact Host component so an equivalent binding from another seal can
	// never become local merely because its content happens to agree.
	canonical, canonicalOK := linked.Host().Globals().For(analysis, cell)
	if !canonicalOK || canonical != binding {
		return result{}, false
	}
	shard, _, _, rootOK := linked.Module().Roots().Mapping(analysis)
	if !rootOK {
		return result{}, false
	}
	subject, subjectOK := linked.Boundary().Values().Of(shard, cell)
	coordinate, coordinateOK := schema.CoordinateFor(subject)
	if !subjectOK || !coordinateOK {
		return result{}, false
	}
	// A global bootstrap must not race an unconditional source producer at the
	// same Value coordinate; source ownership remains singular.
	if _, overlap := schema.SourceSeed(subject); overlap {
		return result{}, false
	}
	contract, contractOK := linked.Boundary().Target()
	if !contractOK || contract == nil {
		return result{}, false
	}
	kind, kindOK := contract.InitialValueKind(initial)
	if !kindOK {
		return result{}, false
	}
	if kind == target.InitialValueAbsent {
		return result{coordinate: coordinate, absent: true}, true
	}
	fact, factOK := schema.TargetInitial(boot, initial)
	if !factOK || !schema.AdmitsCoordinate(coordinate, fact) || schema.Equal(fact, schema.Default()) {
		return result{}, false
	}
	return result{coordinate: coordinate, fact: fact}, true
}

func checker(owner *valueowner.Owner, semantic engine.SemanticKey) engine.RuleDerivationChecker[value.Value, linkhost.GlobalBinding] {
	return func(derivation engine.RuleDerivation[value.Value, linkhost.GlobalBinding]) (engine.RuleEvidence, bool) {
		if owner == nil || owner.Schema() == nil || derivation.Rule() != semantic || derivation.InputCount() != 0 || derivation.ReadCount() != 0 || derivation.DispositionCount() != 1 {
			return engine.RuleEvidence{}, false
		}
		binding, operandOK := derivation.Operand()
		canonical, digest, contentOK := globalContent(owner)(binding)
		result, resultOK := globalResult(owner, binding)
		if !operandOK || !contentOK || canonical != binding || !resultOK || !derivation.OperandContentMatches(digest) {
			return engine.RuleEvidence{}, false
		}
		disposition, ok := derivation.DispositionAt(0)
		if !ok || disposition.Guard().Empty() {
			return engine.RuleEvidence{}, false
		}
		if _, transformed := disposition.CarryTransform(); transformed || disposition.TransformOnly() {
			return engine.RuleEvidence{}, false
		}
		if result.absent {
			if disposition.Kind() != engine.RuleDispositionNoCandidate || disposition.TargetCount() != 0 {
				return engine.RuleEvidence{}, false
			}
			return derivation.Accept()
		}
		ref, refOK := owner.Locate(result.coordinate)
		targetRef, targetOK := disposition.TargetAt(0)
		actual, actualOK := disposition.Value()
		if disposition.Kind() != engine.RuleDispositionStaged || disposition.TargetCount() != 1 || !refOK || !targetOK || !actualOK ||
			!engine.TargetMatchesRef(targetRef, ref) || !owner.Schema().Equal(actual, result.fact) {
			return engine.RuleEvidence{}, false
		}
		return derivation.Accept()
	}
}
