// Package capability declares Value's exact exposure capability seed Rule.
package capability

import (
	"github.com/wippyai/go-lua/analysis/domain/value"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	"github.com/wippyai/go-lua/analysis/engine"
)

// ExposureRule decorates precisely the existing Value fact at an exposure
// seed's output with that seed's sealed provider capability.
type ExposureRule struct {
	rule  *engine.Rule[value.Value, value.CapabilitySeed]
	read  engine.Read[engine.OrderedCells[value.Value]]
	write engine.Write[value.Value]
	owner *valueowner.Owner
}

func DeclareExposure(
	composition *engine.Composition,
	ruleSemantic engine.SemanticKey,
	operandFamily engine.SemanticKey,
	evidenceSemantic engine.SemanticKey,
	owner *valueowner.Owner,
) (*ExposureRule, bool) {
	if composition == nil || owner == nil || owner.Schema() == nil ||
		!ruleSemantic.Available() || !operandFamily.Available() || !evidenceSemantic.Available() ||
		ruleSemantic == operandFamily || ruleSemantic == evidenceSemantic || operandFamily == evidenceSemantic {
		return nil, false
	}
	var read engine.Read[engine.OrderedCells[value.Value]]
	var write engine.Write[value.Value]
	declared, ok := engine.DeclareRule(composition, engine.RuleSpec[value.Value, value.CapabilitySeed]{
		Semantic:      ruleSemantic,
		OperandFamily: operandFamily,
		OperandContent: func(seed value.CapabilitySeed) (value.CapabilitySeed, [32]byte, bool) {
			id, ok := seed.ID()
			return seed, [32]byte(id), ok && [32]byte(id) != [32]byte{}
		},
		Output:    owner.Output(),
		Inputs:    1,
		Admission: engine.AdmitRuleByDerivation(evidenceSemantic, exposureChecker(owner, ruleSemantic, &read)),
		Transfer: func(access engine.Access[value.Value, value.CapabilitySeed]) bool {
			seed, ok := engine.Operand(access)
			if !ok {
				return false
			}
			coordinate, ok := capabilityExposure(owner, seed)
			if !ok {
				return false
			}
			return engine.Product(access, func(row engine.Row) bool {
				cells, ok := engine.ReadValue(access, row, read)
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
				result, ok := seed.ApplyExposure(coordinate, input)
				return ok && engine.StageValue(access, row, result)
			})
		},
	}, func(rule *engine.Rule[value.Value, value.CapabilitySeed]) bool {
		input, ok := rule.InputAt(0)
		if !ok {
			return false
		}
		read, ok = engine.ReadFrom(rule, input, owner.ExactRead())
		if !ok || !engine.CarryFrom(rule, input, owner.Carry()) {
			return false
		}
		write, ok = engine.WriteTo(rule, owner.ExactWrite())
		return ok
	})
	if !ok || declared == nil {
		return nil, false
	}
	return &ExposureRule{rule: declared, read: read, write: write, owner: owner}, true
}

func (rule *ExposureRule) Instance(seed value.CapabilitySeed) (*engine.RuleInstance[value.Value, value.CapabilitySeed], bool) {
	if rule == nil || rule.rule == nil || rule.owner == nil {
		return nil, false
	}
	coordinate, ok := capabilityExposure(rule.owner, seed)
	if !ok {
		return nil, false
	}
	ref, ok := rule.owner.Locate(coordinate)
	if !ok {
		return nil, false
	}
	return engine.NewRuleInstance(rule.rule, seed, func(binding *engine.RuleBinding[value.Value, value.CapabilitySeed]) bool {
		return engine.InstanceRead(binding, rule.read, ref) && engine.InstanceWrite(binding, rule.write, ref)
	})
}

func capabilityExposure(owner *valueowner.Owner, seed value.CapabilitySeed) (value.Coordinate, bool) {
	if owner == nil || owner.Schema() == nil {
		return value.Coordinate{}, false
	}
	coordinate, ok := seed.Exposure()
	if !ok {
		return value.Coordinate{}, false
	}
	_, admitted := owner.Locate(coordinate)
	return coordinate, admitted
}

func exposureChecker(owner *valueowner.Owner, semantic engine.SemanticKey, read *engine.Read[engine.OrderedCells[value.Value]]) engine.RuleDerivationChecker[value.Value, value.CapabilitySeed] {
	return func(derivation engine.RuleDerivation[value.Value, value.CapabilitySeed]) (engine.RuleEvidence, bool) {
		if owner == nil || owner.Schema() == nil || read == nil || derivation.Rule() != semantic || derivation.InputCount() != 1 || derivation.ReadCount() != 1 || derivation.DispositionCount() != 1 {
			return engine.RuleEvidence{}, false
		}
		seed, ok := derivation.Operand()
		if !ok {
			return engine.RuleEvidence{}, false
		}
		id, ok := seed.ID()
		if !ok || !derivation.OperandContentMatches([32]byte(id)) {
			return engine.RuleEvidence{}, false
		}
		coordinate, ok := capabilityExposure(owner, seed)
		if !ok {
			return engine.RuleEvidence{}, false
		}
		ref, ok := owner.Locate(coordinate)
		if !ok || !engine.DerivationReadMatchesRef(derivation, *read, ref) {
			return engine.RuleEvidence{}, false
		}
		disposition, ok := derivation.DispositionAt(0)
		predecessor, predecessorOK := derivation.InputAt(0)
		if !ok || !predecessorOK || disposition.Guard().Empty() || !predecessor.Guard().Same(disposition.Guard()) {
			return engine.RuleEvidence{}, false
		}
		cells, ok := engine.DerivationDispositionReadValue(derivation, disposition, *read)
		if !ok || cells.Count() != 1 {
			return engine.RuleEvidence{}, false
		}
		input, present, available := cells.At(0)
		if !available {
			return engine.RuleEvidence{}, false
		}
		if !present {
			if disposition.Kind() != engine.RuleDispositionNoCandidate || disposition.TargetCount() != 0 {
				return engine.RuleEvidence{}, false
			}
			return derivation.Accept()
		}
		if disposition.Kind() != engine.RuleDispositionStaged || disposition.TargetCount() != 1 {
			return engine.RuleEvidence{}, false
		}
		target, ok := disposition.TargetAt(0)
		if !ok || !engine.TargetMatchesRef(target, ref) {
			return engine.RuleEvidence{}, false
		}
		expected, ok := seed.ApplyExposure(coordinate, input)
		actual, actualOK := disposition.Value()
		if !ok || !actualOK || !owner.Schema().Equal(actual, expected) {
			return engine.RuleEvidence{}, false
		}
		return derivation.Accept()
	}
}
