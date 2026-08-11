// Package allocation owns Value's allocation transition. Heap retains the
// creation coordinate; this package is the sole executable authority that
// ages carried aliases and writes the new Recent result in one patch.
package allocation

import (
	"github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/domain/materialization"
	"github.com/wippyai/go-lua/analysis/domain/value"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	"github.com/wippyai/go-lua/analysis/engine"
)

// Rule retains only the owner-issued forms needed by the one allocation
// transition. Heap Key is the immutable instance operand; no second creation
// identity or allocation registry is introduced here.
type Rule struct {
	rule  *engine.Rule[value.Value, operand]
	write engine.Write[value.Value]
	owner *valueowner.Owner
}

// operand is the one cold, immutable closure of Heap allocation provenance.
// It keeps root/result reconstruction and Fresh construction out of the
// terminal transform loop without exposing a second public allocation type.
type operand struct {
	key        heap.Key
	coordinate value.Coordinate
	fresh      value.Value
	digest     [32]byte
}

// Declare installs one input carry transformed by Age(key), followed by one
// exact Fresh(key) result write in the same predecessor-bound patch.
func Declare(
	composition *engine.Composition,
	ruleSemantic engine.SemanticKey,
	operandFamily engine.SemanticKey,
	transformSemantic engine.SemanticKey,
	evidenceSemantic engine.SemanticKey,
	owner *valueowner.Owner,
) (*Rule, bool) {
	if composition == nil || owner == nil || owner.Schema() == nil ||
		!distinct(ruleSemantic, operandFamily, transformSemantic, evidenceSemantic) {
		return nil, false
	}

	declaration := &Rule{owner: owner}
	var write engine.Write[value.Value]
	declared, ok := engine.DeclareRule(composition, engine.RuleSpec[value.Value, operand]{
		Semantic:      ruleSemantic,
		OperandFamily: operandFamily,
		OperandContent: func(candidate operand) (operand, [32]byte, bool) {
			return allocationOperandContent(owner, candidate)
		},
		Output:    owner.Output(),
		Inputs:    1,
		Admission: engine.AdmitRuleByDerivation(evidenceSemantic, allocationChecker(owner, ruleSemantic, transformSemantic)),
		Transfer: func(access engine.Access[value.Value, operand]) bool {
			allocation, operandOK := engine.Operand(access)
			if !operandOK {
				return false
			}
			// A no-read Product has exactly one row when the predecessor is
			// reachable and zero rows when its support is empty.  Both are total
			// executions: the reachable row stages Fresh after applying Age,
			// while the unreachable occurrence is the engine-owned structural
			// no-op and must not make the whole solve incomplete.
			return engine.Product(access, func(row engine.Row) bool {
				return engine.StageValue(access, row, allocation.fresh)
			})
		},
	}, func(rule *engine.Rule[value.Value, operand]) bool {
		input, inputOK := rule.InputAt(0)
		var writeOK bool
		write, writeOK = engine.WriteTo(rule, owner.ExactWrite())
		return inputOK && writeOK && engine.TransformCarryFrom(rule, input, owner.Carry(), transformSemantic, func(allocation operand, prior value.Value) (value.Value, bool) {
			return owner.Schema().Age(prior, allocation.key)
		})
	})
	if !ok || declared == nil {
		return nil, false
	}
	declaration.rule, declaration.write = declared, write
	return declaration, true
}

// Instance binds the result target from the same Heap key that selects the
// Age transform. Callers cannot pair one allocation root with another result.
func (rule *Rule) Instance(key heap.Key) (*engine.RuleInstance[value.Value, operand], bool) {
	if rule == nil || rule.rule == nil || rule.owner == nil {
		return nil, false
	}
	allocation, ok := allocationOperandFor(rule.owner, key)
	if !ok {
		return nil, false
	}
	ref, ok := rule.owner.Locate(allocation.coordinate)
	if !ok {
		return nil, false
	}
	return engine.NewRuleInstance(rule.rule, allocation, func(binding *engine.RuleBinding[value.Value, operand]) bool {
		return engine.InstanceWrite(binding, rule.write, ref)
	})
}

func allocationOperandContent(owner *valueowner.Owner, candidate operand) (operand, [32]byte, bool) {
	canonical, ok := allocationOperandFor(owner, candidate.key)
	if !ok || candidate.key != canonical.key || candidate.coordinate != canonical.coordinate ||
		!owner.Schema().Same(candidate.fresh, canonical.fresh) || candidate.digest != canonical.digest {
		return operand{}, [32]byte{}, false
	}
	return canonical, canonical.digest, true
}

func allocationOperandFor(owner *valueowner.Owner, key heap.Key) (operand, bool) {
	coordinate, fresh, ok := allocationResult(owner, key)
	if !ok {
		return operand{}, false
	}
	id, ok := key.ContentID()
	digest := [32]byte(id)
	if !ok || digest == [32]byte{} {
		return operand{}, false
	}
	return operand{key: key, coordinate: coordinate, fresh: fresh, digest: digest}, true
}

// allocationResult is the sole reconstruction shared by transfer, transform,
// binding, and evidence. It admits only Program allocation sources with an
// existing Value result. Target-fresh results remain owned by their guarded
// application outcome Rule; no Value coordinate is fabricated for them.
func allocationResult(owner *valueowner.Owner, key heap.Key) (value.Coordinate, value.Value, bool) {
	if owner == nil || owner.Schema() == nil || owner.Schema().Link() == nil {
		return value.Coordinate{}, value.Value{}, false
	}
	schema, linked := owner.Schema(), owner.Schema().Link()
	shard, term, _, rootOK := key.ProgramAllocation()
	subject, subjectOK := linked.Boundary().Values().Of(shard, term)
	coordinate, coordinateOK := schema.CoordinateFor(subject)
	recent, recentOK := schema.Allocation(key, materialization.Recent)
	fresh, freshOK := schema.Singleton(recent)
	if !rootOK || !subjectOK || !coordinateOK || !recentOK || !freshOK ||
		!schema.AdmitsCoordinate(coordinate, fresh) || schema.Equal(fresh, schema.Default()) {
		return value.Coordinate{}, value.Value{}, false
	}
	return coordinate, fresh, true
}

func allocationChecker(owner *valueowner.Owner, ruleSemantic, transformSemantic engine.SemanticKey) engine.RuleDerivationChecker[value.Value, operand] {
	return func(derivation engine.RuleDerivation[value.Value, operand]) (engine.RuleEvidence, bool) {
		if derivation.Rule() != ruleSemantic || derivation.InputCount() != 1 || derivation.ReadCount() != 0 || derivation.DispositionCount() != 1 {
			return engine.RuleEvidence{}, false
		}
		if _, ok := derivation.InputAt(0); !ok {
			return engine.RuleEvidence{}, false
		}
		allocation, operandOK := derivation.Operand()
		canonical, contentOK := allocationOperandFor(owner, allocation.key)
		if !operandOK || !contentOK || allocation.key != canonical.key || allocation.coordinate != canonical.coordinate ||
			!owner.Schema().Same(allocation.fresh, canonical.fresh) || allocation.digest != canonical.digest ||
			!derivation.OperandContentMatches(canonical.digest) {
			return engine.RuleEvidence{}, false
		}
		disposition, ok := derivation.DispositionAt(0)
		semantic, transformed := disposition.CarryTransform()
		if !ok || disposition.Kind() != engine.RuleDispositionStaged || disposition.TransformOnly() ||
			!transformed || semantic != transformSemantic || disposition.TargetCount() != 1 || disposition.Guard().Empty() {
			return engine.RuleEvidence{}, false
		}
		actual, valueOK := disposition.Value()
		target, targetOK := disposition.TargetAt(0)
		ref, refOK := owner.Locate(canonical.coordinate)
		if !valueOK || !targetOK || !refOK || !owner.Schema().Equal(actual, canonical.fresh) || !engine.TargetMatchesRef(target, ref) {
			return engine.RuleEvidence{}, false
		}
		return derivation.Accept()
	}
}

func distinct(keys ...engine.SemanticKey) bool {
	for index, key := range keys {
		if !key.Available() {
			return false
		}
		for prior := 0; prior < index; prior++ {
			if keys[prior] == key {
				return false
			}
		}
	}
	return true
}
