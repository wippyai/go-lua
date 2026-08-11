// Package rules declares Static's sole state-dependent typeof judgment.
package rules

import (
	staticdomain "github.com/wippyai/go-lua/analysis/domain/static"
	staticowner "github.com/wippyai/go-lua/analysis/domain/static/owner"
	"github.com/wippyai/go-lua/analysis/domain/value"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	linkstatic "github.com/wippyai/go-lua/program/link/static"
)

// Rule owns the only Static rule that reads runtime Value state. Its raw
// engine Rule remains private: an instance can be formed only from Link's
// existing TypeOf StaticInput and derives both exact owner coordinates from it.
type Rule struct {
	rule      *engine.Rule[staticdomain.Value, linkstatic.InputRef]
	read      engine.Read[engine.OrderedCells[value.Value]]
	write     engine.Write[staticdomain.Value]
	static    *staticowner.Owner
	values    *valueowner.Owner
	authority *staticdomain.Authority
}

// Declare records exactly one one-read typeof judgment. Static retains the
// type result algebra; Value retains the runtime-kind projection and its
// exact input coordinate. Neither Factor obtains the other's authority.
func Declare(
	composition *engine.Composition,
	ruleSemantic engine.SemanticKey,
	operandFamily engine.SemanticKey,
	evidenceSemantic engine.SemanticKey,
	static *staticowner.Owner,
	values *valueowner.Owner,
) (*Rule, bool) {
	if composition == nil || static == nil || values == nil || static.Authority() == nil || values.Schema() == nil ||
		static.Authority().Link() == nil || static.Authority().Link() != values.Schema().Link() ||
		!ruleSemantic.Available() || !operandFamily.Available() || !evidenceSemantic.Available() ||
		ruleSemantic == operandFamily || ruleSemantic == evidenceSemantic || operandFamily == evidenceSemantic {
		return nil, false
	}
	var read engine.Read[engine.OrderedCells[value.Value]]
	var write engine.Write[staticdomain.Value]
	declared, ok := engine.DeclareRule(composition, engine.RuleSpec[staticdomain.Value, linkstatic.InputRef]{
		Semantic:      ruleSemantic,
		OperandFamily: operandFamily,
		OperandContent: func(input linkstatic.InputRef) (linkstatic.InputRef, [32]byte, bool) {
			return staticInputContent(static.Authority(), values, input)
		},
		Output:    static.Output(),
		Inputs:    1,
		Admission: engine.AdmitRuleByDerivation(evidenceSemantic, checker(static, values, ruleSemantic, &read)),
		Transfer: func(access engine.Access[staticdomain.Value, linkstatic.InputRef]) bool {
			input, ok := engine.Operand(access)
			if !ok {
				return false
			}
			if _, contained, ok := static.Authority().TypeOf(input); !ok {
				return false
			} else if _, runtime := contained.RuntimeSubject(); !runtime {
				return false
			}
			return engine.Product(access, func(row engine.Row) bool {
				cells, ok := engine.ReadValue(access, row, read)
				if !ok || cells.Count() != 1 {
					return false
				}
				_, present, available := cells.At(0)
				if !available {
					return false
				}
				if !present {
					return engine.NoCandidate(access, row)
				}
				result, ok := runtimeResult(static.Authority(), values.Schema(), cells)
				return ok && engine.StageValue(access, row, result)
			})
		},
	}, func(rule *engine.Rule[staticdomain.Value, linkstatic.InputRef]) bool {
		input, ok := rule.InputAt(0)
		if !ok {
			return false
		}
		read, ok = engine.ReadFrom(rule, input, values.ExactRead())
		if !ok {
			return false
		}
		write, ok = engine.WriteTo(rule, static.ExactWrite())
		return ok
	})
	if !ok || declared == nil {
		return nil, false
	}
	return &Rule{rule: declared, read: read, write: write, static: static, values: values, authority: static.Authority()}, true
}

// Instance binds the one existing Link TypeOf StaticInput. The RuntimeSubject,
// Value coordinate, Static output coordinate, and semantic identity are all
// rederived from that operand before the atomic binding callback is opened.
func (rule *Rule) Instance(input linkstatic.InputRef) (*engine.RuleInstance[staticdomain.Value, linkstatic.InputRef], bool) {
	if rule == nil || rule.rule == nil || rule.static == nil || rule.values == nil || rule.authority == nil {
		return nil, false
	}
	output, contained, ok := rule.authority.TypeOf(input)
	if !ok {
		return nil, false
	}
	subject, ok := contained.RuntimeSubject()
	if !ok || subject.LinkID() != rule.authority.LinkID() {
		return nil, false
	}
	raw, ok := subject.Value()
	if !ok {
		return nil, false
	}
	coordinate, ok := rule.values.Schema().CoordinateFor(raw)
	if !ok {
		return nil, false
	}
	valueRef, ok := rule.values.Locate(coordinate)
	if !ok {
		return nil, false
	}
	outputRef, ok := rule.static.Locate(output)
	if !ok {
		return nil, false
	}
	boundary := rule.values.Schema().Link().Boundary()
	if boundary == nil {
		return nil, false
	}
	if _, ok := boundary.Values().ID(raw); !ok {
		return nil, false
	}
	return engine.NewRuleInstance(rule.rule, input, func(binding *engine.RuleBinding[staticdomain.Value, linkstatic.InputRef]) bool {
		return engine.InstanceRead(binding, rule.read, valueRef) && engine.InstanceWrite(binding, rule.write, outputRef)
	})
}

func staticInputContent(authority *staticdomain.Authority, values *valueowner.Owner, input linkstatic.InputRef) (linkstatic.InputRef, [32]byte, bool) {
	if authority == nil || values == nil || values.Schema() == nil {
		return linkstatic.InputRef{}, [32]byte{}, false
	}
	_, contained, ok := authority.TypeOf(input)
	if !ok {
		return linkstatic.InputRef{}, [32]byte{}, false
	}
	subject, ok := contained.RuntimeSubject()
	if !ok || subject.LinkID() != authority.LinkID() {
		return linkstatic.InputRef{}, [32]byte{}, false
	}
	if _, ok := subject.Value(); !ok {
		return linkstatic.InputRef{}, [32]byte{}, false
	}
	id, ok := authority.Link().Static().Inputs().ID(input)
	return input, [32]byte(id), ok && [32]byte(id) != [32]byte{}
}

func runtimeResult(authority *staticdomain.Authority, schema *value.Schema, cells engine.OrderedCells[value.Value]) (staticdomain.Value, bool) {
	if authority == nil || schema == nil || cells.Count() != 1 {
		return staticdomain.Value{}, false
	}
	fact, present, available := cells.At(0)
	if !available || !present {
		return staticdomain.Value{}, false
	}
	return authority.RuntimeTypeOf(schema.RuntimeKinds(fact))
}

func checker(static *staticowner.Owner, values *valueowner.Owner, ruleSemantic engine.SemanticKey, read *engine.Read[engine.OrderedCells[value.Value]]) engine.RuleDerivationChecker[staticdomain.Value, linkstatic.InputRef] {
	return func(derivation engine.RuleDerivation[staticdomain.Value, linkstatic.InputRef]) (engine.RuleEvidence, bool) {
		if static == nil || values == nil || read == nil || static.Authority() == nil || values.Schema() == nil || derivation.Rule() != ruleSemantic ||
			derivation.InputCount() != 1 || derivation.ReadCount() != 1 || derivation.DispositionCount() != 1 {
			return engine.RuleEvidence{}, false
		}
		operand, ok := derivation.Operand()
		if !ok {
			return engine.RuleEvidence{}, false
		}
		output, contained, ok := static.Authority().TypeOf(operand)
		if !ok {
			return engine.RuleEvidence{}, false
		}
		subject, ok := contained.RuntimeSubject()
		if !ok || subject.LinkID() != static.Authority().LinkID() {
			return engine.RuleEvidence{}, false
		}
		raw, ok := subject.Value()
		if !ok {
			return engine.RuleEvidence{}, false
		}
		id, ok := static.Authority().Link().Static().Inputs().ID(operand)
		if !ok || !derivation.OperandContentMatches([32]byte(id)) {
			return engine.RuleEvidence{}, false
		}
		coordinate, ok := values.Schema().CoordinateFor(raw)
		if !ok {
			return engine.RuleEvidence{}, false
		}
		valueRef, ok := values.Locate(coordinate)
		if !ok || !engine.DerivationReadMatchesRef(derivation, *read, valueRef) {
			return engine.RuleEvidence{}, false
		}
		outputRef, outputOK := static.Locate(output)
		disposition, dispositionOK := derivation.DispositionAt(0)
		input, inputOK := derivation.InputAt(0)
		if !outputOK || !dispositionOK || !inputOK || disposition.Guard().Empty() || !input.Guard().Same(disposition.Guard()) {
			return engine.RuleEvidence{}, false
		}
		cells, cellsOK := engine.DerivationDispositionReadValue(derivation, disposition, *read)
		if !cellsOK || cells.Count() != 1 {
			return engine.RuleEvidence{}, false
		}
		_, present, available := cells.At(0)
		if !available {
			return engine.RuleEvidence{}, false
		}
		if !present {
			if disposition.Kind() != engine.RuleDispositionNoCandidate || disposition.TargetCount() != 0 || disposition.Guard().Empty() {
				return engine.RuleEvidence{}, false
			}
			return derivation.Accept()
		}
		if disposition.Kind() != engine.RuleDispositionStaged || disposition.TargetCount() != 1 {
			return engine.RuleEvidence{}, false
		}
		target, ok := disposition.TargetAt(0)
		if !ok || !engine.TargetMatchesRef(target, outputRef) {
			return engine.RuleEvidence{}, false
		}
		expected, expectedOK := runtimeResult(static.Authority(), values.Schema(), cells)
		actual, actualOK := disposition.Value()
		if !expectedOK || !actualOK || !static.Authority().Equal(actual, expected) {
			return engine.RuleEvidence{}, false
		}
		return derivation.Accept()
	}
}
