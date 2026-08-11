// Package dispatch declares Call's selected direct-dispatch Rule. Selection
// is driven by one exact Value callee; this package owns no target universe
// and never enumerates Application×Operation candidates.
package dispatch

import (
	calldomain "github.com/wippyai/go-lua/analysis/domain/call"
	callowner "github.com/wippyai/go-lua/analysis/domain/call/owner"
	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	packdomain "github.com/wippyai/go-lua/analysis/domain/pack"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
)

// Rule is the one Value-to-Call selected-dispatch judgment. The raw engine
// rule remains private so callers cannot pair an arbitrary Value coordinate
// with a Call key or assert completeness by supplying selector rows.
type Rule struct {
	rule   *engine.Rule[calldomain.Value, site]
	read   engine.Read[engine.OrderedCells[valuedomain.Value]]
	write  engine.Write[calldomain.Value]
	values *valueowner.Owner
	heaps  heapdomain.Schema
	packs  *packdomain.Schema
	calls  *callowner.Owner
}

// Declare records the exact one-input dispatch rule. The Pack root lives in
// the private site as a cold provenance fence; Pack's recurrent fact is
// deliberately not read here because actual/formal interpretation belongs to
// the selected Target Effect rule.
func Declare(
	composition *engine.Composition,
	ruleSemantic engine.SemanticKey,
	operandFamily engine.SemanticKey,
	evidenceSemantic engine.SemanticKey,
	values *valueowner.Owner,
	heaps heapdomain.Schema,
	packs *packdomain.Schema,
	calls *callowner.Owner,
) (*Rule, bool) {
	valueSchema := (*valuedomain.Schema)(nil)
	if values != nil {
		valueSchema = values.Schema()
	}
	valueLink := (*link.Link)(nil)
	if valueSchema != nil {
		valueLink = valueSchema.Link()
	}
	if composition == nil || values == nil || calls == nil || valueSchema == nil || calls.Algebra() == nil ||
		!heaps.Valid() || packs == nil ||
		valueLink == nil || !valueLink.ContentID().Available() || calls.Algebra().Link() != valueLink ||
		!valueSchema.OwnsHeapSchema(heaps) ||
		heaps.Link() != valueLink || packs.Link() != valueLink ||
		!ruleSemantic.Available() || !operandFamily.Available() || !evidenceSemantic.Available() ||
		ruleSemantic == operandFamily || ruleSemantic == evidenceSemantic || operandFamily == evidenceSemantic {
		return nil, false
	}

	var read engine.Read[engine.OrderedCells[valuedomain.Value]]
	var write engine.Write[calldomain.Value]
	declared, ok := engine.DeclareRule(composition, engine.RuleSpec[calldomain.Value, site]{
		Semantic:       ruleSemantic,
		OperandFamily:  operandFamily,
		OperandContent: content,
		Output:         calls.Output(),
		Inputs:         1,
		Admission:      engine.AdmitRuleByDerivation(evidenceSemantic, checker(ruleSemantic, values, heaps, packs, calls, &read)),
		Transfer: func(access engine.Access[calldomain.Value, site]) bool {
			operand, ok := engine.Operand(access)
			if !ok || !operand.matchesSchemas(heaps, packs) || operand.valueSchema() != values.Schema() {
				return false
			}
			_, keyOK := operand.callKey()
			if !keyOK || operand.algebraOwner() != calls.Algebra() {
				return false
			}
			return engine.Product(access, func(row engine.Row) bool {
				cells, ok := engine.ReadValue(access, row, read)
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
				result, ok := reduce(operand, fact)
				return ok && engine.StageValue(access, row, result)
			})
		},
	}, func(rule *engine.Rule[calldomain.Value, site]) bool {
		input, ok := rule.InputAt(0)
		if !ok {
			return false
		}
		read, ok = engine.ReadFrom(rule, input, values.ExactRead())
		if !ok {
			return false
		}
		write, ok = engine.WriteTo(rule, calls.ExactWrite())
		return ok
	})
	if !ok || declared == nil {
		return nil, false
	}
	return &Rule{rule: declared, read: read, write: write, values: values, heaps: heaps, packs: packs, calls: calls}, true
}

// Instance binds one existing Project Application to the exact Value and
// Call owner coordinates. The private site is constructed here so callers
// cannot provide a foreign callee, Pack root, or Call key independently.
func (rule *Rule) Instance(application linkproject.Application) (*engine.RuleInstance[calldomain.Value, site], bool) {
	if rule == nil || rule.rule == nil || rule.values == nil || rule.calls == nil || rule.packs == nil || !rule.heaps.Valid() {
		return nil, false
	}
	operand, operandOK := newSite(rule.calls.Algebra(), rule.values.Schema(), rule.heaps, rule.packs, application)
	if !operandOK {
		return nil, false
	}
	key, keyOK := operand.callKey()
	coordinate, coordinateOK := operand.valueCoordinate()
	if !keyOK || !coordinateOK {
		return nil, false
	}
	valueRef, valueOK := rule.values.Locate(coordinate)
	callRef, callOK := rule.calls.Locate(key)
	if !valueOK || !callOK {
		return nil, false
	}
	return engine.NewRuleInstance(rule.rule, operand, func(binding *engine.RuleBinding[calldomain.Value, site]) bool {
		return engine.InstanceRead(binding, rule.read, valueRef) && engine.InstanceWrite(binding, rule.write, callRef)
	})
}

func content(operand site) (site, [32]byte, bool) {
	id, ok := operand.contentID()
	return operand, [32]byte(id), ok && [32]byte(id) != [32]byte{}
}

func checker(
	ruleSemantic engine.SemanticKey,
	values *valueowner.Owner,
	heaps heapdomain.Schema,
	packs *packdomain.Schema,
	calls *callowner.Owner,
	read *engine.Read[engine.OrderedCells[valuedomain.Value]],
) engine.RuleDerivationChecker[calldomain.Value, site] {
	return func(derivation engine.RuleDerivation[calldomain.Value, site]) (engine.RuleEvidence, bool) {
		if values == nil || calls == nil || packs == nil || !heaps.Valid() || read == nil || derivation.Rule() != ruleSemantic ||
			derivation.InputCount() != 1 || derivation.ReadCount() != 1 || derivation.DispositionCount() != 1 {
			return engine.RuleEvidence{}, false
		}
		site, siteOK := derivation.Operand()
		id, idOK := site.contentID()
		if !siteOK || !idOK || !derivation.OperandContentMatches([32]byte(id)) || !site.matchesSchemas(heaps, packs) || site.valueSchema() != values.Schema() {
			return engine.RuleEvidence{}, false
		}
		key, keyOK := site.callKey()
		coordinate, coordinateOK := site.valueCoordinate()
		valueRef, valueOK := values.Locate(coordinate)
		callRef, callOK := calls.Locate(key)
		if !keyOK || !coordinateOK || !valueOK || !callOK || site.algebraOwner() != calls.Algebra() ||
			!engine.DerivationReadMatchesRef(derivation, *read, valueRef) {
			return engine.RuleEvidence{}, false
		}
		disposition, dispositionOK := derivation.DispositionAt(0)
		input, inputOK := derivation.InputAt(0)
		if !dispositionOK || !inputOK || disposition.Guard().Empty() || !input.Guard().Same(disposition.Guard()) {
			return engine.RuleEvidence{}, false
		}
		if _, transformed := disposition.CarryTransform(); transformed || disposition.TransformOnly() {
			return engine.RuleEvidence{}, false
		}
		cells, cellsOK := engine.DerivationDispositionReadValue(derivation, disposition, *read)
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
		expected, expectedOK := reduce(site, fact)
		if !expectedOK || disposition.Kind() != engine.RuleDispositionStaged || disposition.TargetCount() != 1 {
			return engine.RuleEvidence{}, false
		}
		target, targetOK := disposition.TargetAt(0)
		actual, actualOK := disposition.Value()
		if !targetOK || !actualOK || !engine.TargetMatchesRef(target, callRef) || !calls.Algebra().Equal(actual, expected) {
			return engine.RuleEvidence{}, false
		}
		return derivation.Accept()
	}
}
