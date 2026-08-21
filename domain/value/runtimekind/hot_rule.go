package runtimekind

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/domain/call"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	runtimekindenum "github.com/wippyai/go-lua/domain/runtimekind"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// HotRule is the Value-owned runtime-kind transfer.  It keeps only the two
// owner-issued exact reads and the sealed Value operand; Call behavior is
// projected through its existing Target capability.
type HotRule struct {
	implementation    *valueowner.RuleImplementation[valuedomain.RuntimeKindCall]
	callRead          engine.Read[engine.OrderedCells[call.Value]]
	valueRead         engine.Read[engine.OrderedCells[valuedomain.Value]]
	comparisonRead    engine.Read[engine.OrderedCells[valuedomain.Value]]
	values            *valueowner.HotOwner
	calls             *callowner.HotOwner
	semantic          identity.SemanticKey
	relation          schema.EntryID
	predicateRelation schema.EntryID
}

func BindHot(fragment *SchemaFragment, values *valueowner.HotOwner, calls *callowner.HotOwner) (*HotRule, bool) {
	if fragment == nil || fragment.slot == nil || values == nil || calls == nil || values.Schema() == nil || calls.Algebra() == nil ||
		!fragment.semantic.Available() {
		return nil, false
	}
	relation := schema.NewEntryID(schema.SurfaceKindStructure, runtimekindenum.RuntimeKindResultRelationKey)
	predicateRelation := schema.NewEntryID(schema.SurfaceKindStructure, runtimekindenum.RuntimeKindPredicateRelationKey)
	if !relation.Available() || !predicateRelation.Available() {
		return nil, false
	}
	hot := &HotRule{values: values, calls: calls, semantic: fragment.semantic, relation: relation, predicateRelation: predicateRelation}
	var callRead engine.Read[engine.OrderedCells[call.Value]]
	var valueRead engine.Read[engine.OrderedCells[valuedomain.Value]]
	var comparisonRead engine.Read[engine.OrderedCells[valuedomain.Value]]
	var implementation *valueowner.RuleImplementation[valuedomain.RuntimeKindCall]
	var bindSpec = engine.HotRuleSpec[valuedomain.Value, valuedomain.RuntimeKindCall]{
		OperandContent: func(row valuedomain.RuntimeKindCall) (valuedomain.RuntimeKindCall, [32]byte, bool) {
			return hotContent(values.Schema(), row)
		},
		OperandResolver: hot.resolveOperand,
		Fold: func(frame engine.Frame[valuedomain.Value, valuedomain.RuntimeKindCall]) engine.RuleResult[valuedomain.Value] {
			operand, operandOK := engine.Operand(frame)
			_, _, endpointsOK := hotEndpoints(values.Schema(), operand)
			if !operandOK || !endpointsOK {
				return engine.RuleResult[valuedomain.Value]{}
			}
			if _, writeOK := hotWriteTarget(values.Schema(), operand); !writeOK {
				return engine.RuleResult[valuedomain.Value]{}
			}
			callCells, callOK := engine.ReadValue(frame, callRead)
			valueCells, valueOK := engine.ReadValue(frame, valueRead)
			comparisonCells, comparisonOK := engine.ReadValue(frame, comparisonRead)
			if !callOK || !valueOK || !comparisonOK || callCells.Count() != 1 || valueCells.Count() != 1 || comparisonCells.Count() != 1 {
				return engine.RuleResult[valuedomain.Value]{}
			}
			callFact, callPresent, callAvailable := callCells.At(0)
			valueFact, valuePresent, valueAvailable := valueCells.At(0)
			comparisonFact, comparisonPresent, comparisonAvailable := comparisonCells.At(0)
			if !callAvailable || !valueAvailable || !comparisonAvailable {
				return engine.RuleResult[valuedomain.Value]{}
			}
			if !callPresent || !valuePresent || !comparisonPresent {
				return engine.NoCandidate(frame)
			}
			projected, decision, ok := classify(values.Schema(), callFact, valueFact, relation)
			if _, op, truth, refinement := operand.Refinement(); refinement {
				projected, decision, ok = classifyRefinement(values.Schema(), callFact, valueFact, comparisonFact, predicateRelation, op, truth)
			}
			if !ok {
				return engine.RuleResult[valuedomain.Value]{}
			}
			switch decision {
			case decisionNoCandidate:
				return engine.NoCandidate(frame)
			case decisionStage:
				return engine.Staged(frame, projected)
			default:
				return engine.RuleResult[valuedomain.Value]{}
			}
		},
	}
	implementation, bound := valueowner.BindSelectedRuleDirect(values, fragment.slot, fragment.carry, fragment.write, values.FactorRef(), bindSpec, engine.HotCarrySpec[valuedomain.Value, valuedomain.RuntimeKindCall]{}, func(row valuedomain.RuntimeKindCall) (uint64, bool) {
		target, ok := hotWriteTarget(values.Schema(), row)
		index, indexOK := values.Schema().CoordinateIndex(target)
		return uint64(index), ok && indexOK
	})
	if !bound {
		return nil, false
	}
	var callOK, valueOK, comparisonOK bool
	callRead, callOK = valueowner.AddSelectedRuleDirectExactRead(implementation, fragment.callRead, calls.FactorRef(), func(row valuedomain.RuntimeKindCall) (uint64, bool) {
		module, occurrence, ok := callOccurrence(values.Schema(), row)
		return projectCall(calls.Algebra(), module, occurrence, ok)
	})
	valueRead, valueOK = valueowner.AddSelectedRuleDirectExactRead(implementation, fragment.valueRead, values.FactorRef(), func(row valuedomain.RuntimeKindCall) (uint64, bool) {
		_, input, ok := hotEndpoints(values.Schema(), row)
		index, indexOK := values.Schema().CoordinateIndex(input)
		return uint64(index), ok && indexOK
	})
	comparisonRead, comparisonOK = valueowner.AddSelectedRuleDirectExactRead(implementation, fragment.comparisonRead, values.FactorRef(), func(row valuedomain.RuntimeKindCall) (uint64, bool) {
		comparison, _, _, refinement := row.Refinement()
		if !refinement {
			_, input, ok := hotEndpoints(values.Schema(), row)
			index, indexOK := values.Schema().CoordinateIndex(input)
			return uint64(index), ok && indexOK
		}
		index, indexOK := values.Schema().CoordinateIndex(comparison)
		return uint64(index), indexOK
	})
	if !callOK || !valueOK || !comparisonOK {
		return nil, false
	}
	hot.callRead, hot.valueRead, hot.comparisonRead, hot.implementation = callRead, valueRead, comparisonRead, implementation
	return hot, true
}

// resolveOperand obtains the Value-owned operand from the sealed computation
// directory for the exact mounted occurrence.
func (rule *HotRule) resolveOperand(coords engine.OperandCoords) (valuedomain.RuntimeKindCall, bool) {
	return rule.OperandForOccurrence(coords.Mount, coords.Occurrence)
}

// OperandForOccurrence is the only operand lookup exposed by this rule.  It
// is occurrence-scoped and owner-fenced; no target or Program data is rebuilt.
func (rule *HotRule) OperandForOccurrence(mount, occurrence identity.ContentID) (valuedomain.RuntimeKindCall, bool) {
	if rule == nil || rule.values == nil || rule.values.Schema() == nil || !mount.Available() || !occurrence.Available() {
		return valuedomain.RuntimeKindCall{}, false
	}
	row, ok := rule.values.Schema().RuntimeKindCall(mount, occurrence)
	return row, ok && rule.values.Schema().OwnsRuntimeKindCall(row)
}

func (rule *HotRule) Implementation() (*valueowner.RuleImplementation[valuedomain.RuntimeKindCall], bool) {
	if rule == nil || rule.implementation == nil {
		return nil, false
	}
	_, ok := valueowner.ResolveRuleImplementation(rule.implementation)
	return rule.implementation, ok
}

// SealProgramRule is this typed rule's schema registration.
func SealProgramRule(rule *HotRule) (engine.ProgramRule, bool) {
	if rule == nil {
		return engine.ProgramRule{}, false
	}
	implementation, ok := valueowner.ResolveRuleImplementationFor(rule.values, rule.implementation)
	if !ok {
		return engine.ProgramRule{}, false
	}
	return engine.SealProgramRule(implementation)
}

func hotContent(schema *valuedomain.Schema, row valuedomain.RuntimeKindCall) (valuedomain.RuntimeKindCall, [32]byte, bool) {
	id, ok := row.ID()
	if schema == nil || !schema.OwnsRuntimeKindCall(row) || !ok || [32]byte(id) == ([32]byte{}) {
		return valuedomain.RuntimeKindCall{}, [32]byte{}, false
	}
	return row, [32]byte(id), true
}

// hotEndpoints returns the two Value coordinates.  Mount and occurrence
// identity remain a separate projection so semantic coordinates cannot be
// mistaken for a Call key.
func hotEndpoints(schema *valuedomain.Schema, row valuedomain.RuntimeKindCall) (result, input valuedomain.Coordinate, ok bool) {
	if schema == nil || !schema.OwnsRuntimeKindCall(row) {
		return valuedomain.Coordinate{}, valuedomain.Coordinate{}, false
	}
	result, input, ok = row.Endpoints()
	if !ok {
		return valuedomain.Coordinate{}, valuedomain.Coordinate{}, false
	}
	if _, resultOK := schema.CoordinateIndex(result); !resultOK {
		return valuedomain.Coordinate{}, valuedomain.Coordinate{}, false
	}
	if _, inputOK := schema.CoordinateIndex(input); !inputOK {
		return valuedomain.Coordinate{}, valuedomain.Coordinate{}, false
	}
	return result, input, true
}

func hotWriteTarget(schema *valuedomain.Schema, row valuedomain.RuntimeKindCall) (valuedomain.Coordinate, bool) {
	if schema == nil || !schema.OwnsRuntimeKindCall(row) {
		return valuedomain.Coordinate{}, false
	}
	target, ok := row.WriteTarget()
	if !ok {
		return valuedomain.Coordinate{}, false
	}
	_, indexOK := schema.CoordinateIndex(target)
	return target, indexOK
}

func callOccurrence(schema *valuedomain.Schema, row valuedomain.RuntimeKindCall) (module, occurrence identity.ContentID, ok bool) {
	if schema == nil || !schema.OwnsRuntimeKindCall(row) {
		return identity.ContentID{}, identity.ContentID{}, false
	}
	module, occurrence, ok = row.CallOccurrence()
	return module, occurrence, ok && module.Available() && occurrence.Available()
}

func projectCall(algebra *call.Algebra, module, occurrence identity.ContentID, ok bool) (uint64, bool) {
	key, keyOK := projectCallKey(algebra, module, occurrence, ok)
	index, indexOK := algebra.KeyIndex(key)
	return uint64(index), keyOK && indexOK
}

func projectCallKey(algebra *call.Algebra, module, occurrence identity.ContentID, ok bool) (call.Key, bool) {
	if !ok || algebra == nil || !module.Available() || !occurrence.Available() {
		return call.Key{}, false
	}
	mounted, mountedOK := algebra.MountedCallForOccurrence(module, occurrence)
	applicationID, _, mountedModule, _, _, identityOK := algebra.MountedCallIdentity(mounted)
	if !mountedOK || !identityOK || mountedModule != module || !applicationID.Available() {
		return call.Key{}, false
	}
	return algebra.KeyForApplicationID(applicationID)
}

type decision uint8

const (
	decisionInvalid decision = iota
	decisionNoCandidate
	decisionStage
)

func classify(values *valuedomain.Schema, callFact call.Value, input valuedomain.Value, expectedRelation schema.EntryID) (valuedomain.Value, decision, bool) {
	if values == nil || !callValueValid(callFact) {
		return valuedomain.Value{}, decisionInvalid, false
	}
	if _, inputOK := values.RuntimeKindNames(input); !inputOK {
		return valuedomain.Value{}, decisionInvalid, false
	}
	if input.IsBottom() {
		return values.Bottom(), decisionNoCandidate, true
	}
	if callFact.IsEmpty() {
		return values.Bottom(), decisionNoCandidate, true
	}
	if callFact.KnownTargetCount() == 0 {
		return values.Bottom(), decisionNoCandidate, true
	}
	owned := false
	for index := 0; index < callFact.KnownTargetCount(); index++ {
		target, targetOK := callFact.KnownTargetAt(index)
		if !targetOK {
			return valuedomain.Value{}, decisionInvalid, false
		}
		if targetProducesRuntimeKind(target, expectedRelation) {
			owned = true
		}
	}
	// RuntimeKind is a compositional result producer, not the fallback result
	// authority. Targets and opaque alternatives outside its declared relation
	// are left to their owning result domains; emitting Top here would erase an
	// exact ModuleLoad (or any other independent result producer).
	if !owned {
		return values.Bottom(), decisionNoCandidate, true
	}
	projected, projectedOK := values.RuntimeKindNames(input)
	if !projectedOK {
		return valuedomain.Value{}, decisionInvalid, false
	}
	if projected.IsBottom() {
		return projected, decisionNoCandidate, true
	}
	return projected, decisionStage, true
}

func classifyRefinement(values *valuedomain.Schema, callFact call.Value, input, comparison valuedomain.Value, expectedRelation schema.EntryID, op flowkind.BinaryOp, branchTruth bool) (valuedomain.Value, decision, bool) {
	if values == nil || !callValueValid(callFact) || (op != flowkind.BinaryEqual && op != flowkind.BinaryNotEqual) {
		return valuedomain.Value{}, decisionInvalid, false
	}
	if _, inputOK := values.RuntimeKindNames(input); !inputOK {
		return valuedomain.Value{}, decisionInvalid, false
	}
	if input.IsBottom() || callFact.IsEmpty() {
		return values.Bottom(), decisionNoCandidate, true
	}
	if callFact.HasOpaqueAlternative() {
		return input, decisionStage, true
	}
	if callFact.KnownTargetCount() == 0 {
		return values.Bottom(), decisionNoCandidate, true
	}
	for index := 0; index < callFact.KnownTargetCount(); index++ {
		target, targetOK := callFact.KnownTargetAt(index)
		if !targetOK || !targetProducesRuntimeKindPredicate(target, expectedRelation) {
			return input, decisionStage, true
		}
	}
	var selected runtimekindenum.Set
	for kind := runtimekindenum.Invalid + 1; kind < runtimekindenum.Count; kind++ {
		mayEqual, mayDiffer, matchOK := values.RuntimeKindNameMatch(comparison, kind)
		if !matchOK {
			return valuedomain.Value{}, decisionInvalid, false
		}
		include := mayEqual
		if op == flowkind.BinaryEqual {
			if !branchTruth {
				include = mayDiffer
			}
		} else if branchTruth {
			include = mayDiffer
		}
		if include {
			selected |= runtimekindenum.Bit(kind)
		}
	}
	projected, projectedOK := values.FilterRuntimeKinds(input, selected)
	if !projectedOK {
		return valuedomain.Value{}, decisionInvalid, false
	}
	if projected.IsBottom() {
		return projected, decisionNoCandidate, true
	}
	return projected, decisionStage, true
}

func callValueValid(fact call.Value) bool {
	return fact.IsTop() || fact.IsOpen() || fact.IsComplete() || fact.IsEmpty()
}

func targetProducesRuntimeKind(target call.Target, expectedRelation schema.EntryID) bool {
	if !target.Valid() || !expectedRelation.Available() || target.BehaviorResultCount() != 1 {
		return false
	}
	outcome, result, source, relation, ok := target.BehaviorResultAt(0)
	return ok && outcome == 0 && result == 0 &&
		source.Kind == vocabulary.InputSourceValueFormal && source.Ordinal == 0 && relation == expectedRelation
}

func targetProducesRuntimeKindPredicate(target call.Target, expectedRelation schema.EntryID) bool {
	if !target.Valid() || !expectedRelation.Available() || target.BehaviorPredicateCount() != 1 {
		return false
	}
	outcome, result, subject, relation, ok := target.BehaviorPredicateAt(0)
	return ok && outcome == 0 && result == 0 &&
		subject.Kind == vocabulary.InputSourceValueFormal && subject.Ordinal == 0 && relation == expectedRelation
}
