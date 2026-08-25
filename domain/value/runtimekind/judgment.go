// Package runtimekind owns Value's runtime-kind judgment and the family its
// declaration is emitted into. The rule reads the Call fact of the mounted
// occurrence its candidate names, the Value fact of the value that call
// observes, and the Value fact the sealed predicate is evaluated against, and
// publishes at the coordinate Value already issued for the row: the call
// result for the ordinary transfer, the narrowed subject for the guarded arm.
package runtimekind

import (
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	schemaapi "github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	calldomain "github.com/wippyai/go-lua/domain/call"
	runtimekindenum "github.com/wippyai/go-lua/domain/runtimekind"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// Judgment is the sealed semantic state of the runtime-kind rule: the Value
// schema its answer is expressed in, and the two structural relation
// identities a target must declare its behavior under for this rule to
// interpret it.
//
// It is the family's state, not a rule payload. The schema is cold and
// immutable for the life of the binding it was issued by, and the two relation
// identities are constants of the structure surface, so all three are sealed
// once when the family is installed and read by every invocation.
type Judgment struct {
	values    *valuedomain.Schema
	result    schemaapi.EntryID
	predicate schemaapi.EntryID
}

// Derive seals the judgment against the Value schema that owns the candidate
// rows it answers for.
func Derive(values *valuedomain.Schema) (Judgment, bool) {
	if values == nil || !values.Valid() {
		return Judgment{}, false
	}
	result := schemaapi.NewEntryID(schemaapi.SurfaceKindStructure, runtimekindenum.RuntimeKindResultRelationKey)
	predicate := schemaapi.NewEntryID(schemaapi.SurfaceKindStructure, runtimekindenum.RuntimeKindPredicateRelationKey)
	if !result.Available() || !predicate.Available() {
		return Judgment{}, false
	}
	return Judgment{values: values, result: result, predicate: predicate}, true
}

// Valid reports whether this state was sealed by Derive.
func (judgment Judgment) Valid() bool {
	return judgment.values != nil && judgment.values.Valid() && judgment.result.Available() && judgment.predicate.Available()
}

// Result is the one irreducible judgment of the runtime-kind rule.
//
// A candidate that declares a guarded predicate is answered by the refinement
// interpretation, which narrows the observed value against the compared one;
// every other candidate is answered by the ordinary projection, which publishes
// the runtime-kind names of the value the call observes. A row without a sealed
// write coordinate is refused rather than answered, because the rule would have
// nowhere to publish what it decided.
func (judgment Judgment) Result(candidate valuedomain.RuntimeKindCall, dispatched calldomain.Value, subject, comparison valuedomain.Value) (valuedomain.Value, structure.ReductionOutcome) {
	if !judgment.Valid() || !judgment.values.OwnsRuntimeKindCall(candidate) {
		return valuedomain.Value{}, structure.Refuse
	}
	if _, writeOK := candidate.Endpoint(valuedomain.EndpointWrite); !writeOK {
		return valuedomain.Value{}, structure.Refuse
	}
	if _, operation, truth, refinement := candidate.Refinement(); refinement {
		return judgment.refinement(dispatched, subject, comparison, operation, truth)
	}
	return judgment.projection(dispatched, subject)
}

// projection is the ordinary runtime-kind transfer: the names the observed
// value may carry, published at the call result.
//
// A Bottom subject, an empty call, and a call with no known target carry no
// runtime-kind evidence and settle absent. RuntimeKind is a compositional
// result producer, not the fallback result authority: targets and opaque
// alternatives outside its declared relation are left to their owning result
// domains, and emitting Top here would erase an exact ModuleLoad or any other
// independent result producer.
func (judgment Judgment) projection(dispatched calldomain.Value, subject valuedomain.Value) (valuedomain.Value, structure.ReductionOutcome) {
	if !callValueValid(dispatched) {
		return valuedomain.Value{}, structure.Refuse
	}
	if _, subjectOK := judgment.values.RuntimeKindNames(subject); !subjectOK {
		return valuedomain.Value{}, structure.Refuse
	}
	if subject.IsBottom() {
		return judgment.values.Bottom(), structure.NoCandidate
	}
	if dispatched.IsEmpty() {
		return judgment.values.Bottom(), structure.NoCandidate
	}
	if dispatched.KnownTargetCount() == 0 {
		return judgment.values.Bottom(), structure.NoCandidate
	}
	owned := false
	for index := 0; index < dispatched.KnownTargetCount(); index++ {
		target, targetOK := dispatched.KnownTargetAt(index)
		if !targetOK {
			return valuedomain.Value{}, structure.Refuse
		}
		if targetProducesRuntimeKind(target, judgment.result) {
			owned = true
		}
	}
	if !owned {
		return judgment.values.Bottom(), structure.NoCandidate
	}
	projected, projectedOK := judgment.values.RuntimeKindNames(subject)
	if !projectedOK {
		return valuedomain.Value{}, structure.Refuse
	}
	if projected.IsBottom() {
		return projected, structure.NoCandidate
	}
	return projected, structure.Concrete
}

// refinement is the guarded arm: the observed value narrowed to the runtime
// kinds the sealed predicate admits on this branch.
//
// An opaque alternative, and any target that does not declare the predicate
// relation, leave the subject as it stands - the guard proves nothing about it.
// The admitted kind set is derived from the compared value one kind at a time,
// under the equality polarity and branch truth the row carries.
func (judgment Judgment) refinement(dispatched calldomain.Value, subject, comparison valuedomain.Value, operation flowkind.BinaryOp, branchTruth bool) (valuedomain.Value, structure.ReductionOutcome) {
	if !callValueValid(dispatched) || (operation != flowkind.BinaryEqual && operation != flowkind.BinaryNotEqual) {
		return valuedomain.Value{}, structure.Refuse
	}
	if _, subjectOK := judgment.values.RuntimeKindNames(subject); !subjectOK {
		return valuedomain.Value{}, structure.Refuse
	}
	if subject.IsBottom() || dispatched.IsEmpty() {
		return judgment.values.Bottom(), structure.NoCandidate
	}
	if dispatched.HasOpaqueAlternative() {
		return subject, structure.Concrete
	}
	if dispatched.KnownTargetCount() == 0 {
		return judgment.values.Bottom(), structure.NoCandidate
	}
	for index := 0; index < dispatched.KnownTargetCount(); index++ {
		target, targetOK := dispatched.KnownTargetAt(index)
		if !targetOK || !targetProducesRuntimeKindPredicate(target, judgment.predicate) {
			return subject, structure.Concrete
		}
	}
	var selected runtimekindenum.Set
	for kind := runtimekindenum.Invalid + 1; kind < runtimekindenum.Count; kind++ {
		mayEqual, mayDiffer, matchOK := judgment.values.RuntimeKindNameMatch(comparison, kind)
		if !matchOK {
			return valuedomain.Value{}, structure.Refuse
		}
		include := mayEqual
		if operation == flowkind.BinaryEqual {
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
	projected, projectedOK := judgment.values.FilterRuntimeKinds(subject, selected)
	if !projectedOK {
		return valuedomain.Value{}, structure.Refuse
	}
	if projected.IsBottom() {
		return projected, structure.NoCandidate
	}
	return projected, structure.Concrete
}

// callValueValid states the shape a Call fact must have before this rule reads
// it: one of the four dispositions Call's algebra publishes.
func callValueValid(fact calldomain.Value) bool {
	return fact.IsTop() || fact.IsOpen() || fact.IsComplete() || fact.IsEmpty()
}

// targetProducesRuntimeKind states the declared behavior a selected target must
// carry for the ordinary transfer to interpret it: one result, derived from the
// first value formal, under the runtime-kind result relation.
func targetProducesRuntimeKind(target calldomain.Target, expectedRelation schemaapi.EntryID) bool {
	if !target.Valid() || !expectedRelation.Available() || target.BehaviorResultCount() != 1 {
		return false
	}
	outcome, result, source, relation, ok := target.BehaviorResultAt(0)
	return ok && outcome == 0 && result == 0 &&
		source.Kind == vocabulary.InputSourceValueFormal && source.Ordinal == 0 && relation == expectedRelation
}

// targetProducesRuntimeKindPredicate states the same for the guarded arm, over
// the predicate the target declares rather than the result it returns.
func targetProducesRuntimeKindPredicate(target calldomain.Target, expectedRelation schemaapi.EntryID) bool {
	if !target.Valid() || !expectedRelation.Available() || target.BehaviorPredicateCount() != 1 {
		return false
	}
	outcome, result, subject, relation, ok := target.BehaviorPredicateAt(0)
	return ok && outcome == 0 && result == 0 &&
		subject.Kind == vocabulary.InputSourceValueFormal && subject.Ordinal == 0 && relation == expectedRelation
}
