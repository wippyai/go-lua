// Package allocation owns Value's constructor-result judgment and the family
// its declaration is emitted into. The rule reads nothing: its candidate is the
// allocation receipt Value sealed for one Heap root, and it publishes that
// receipt's Recent fact at the coordinate the same receipt was issued with,
// aging every carried coordinate through the receipt's own transition.
package allocation

import (
	"github.com/wippyai/go-lua/analysis/schema/structure"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// Judgment is the sealed semantic state of the allocation rule: the Value
// schema its answer is expressed in.
//
// It is the family's state, not a rule payload. The schema is cold and
// immutable for the life of the binding it was issued by, so it is sealed once
// when the family is installed and read by every invocation. It is never a
// parameter of the fold: the fold takes the allocation receipt it is indexed
// by, and nothing else.
type Judgment struct {
	values *valuedomain.Schema
}

// Derive seals the judgment against the Value schema that owns the receipts it
// answers for.
func Derive(values *valuedomain.Schema) (Judgment, bool) {
	if values == nil || !values.Valid() {
		return Judgment{}, false
	}
	return Judgment{values: values}, true
}

// Valid reports whether this state was sealed by Derive.
func (judgment Judgment) Valid() bool {
	return judgment.values != nil && judgment.values.Valid()
}

// Result is the one irreducible judgment of the allocation rule: the canonical
// Recent fact the constructor's receipt was sealed with.
//
// The receipt authenticates itself first. It must belong to this Value schema,
// and it must answer the four questions its issuance settled: the Heap key it
// is paired with, the stable identity that key was issued under, the coordinate
// it publishes at, and the Recent fact it carries. A candidate that answers
// none of those has no result to publish and is refused rather than widened -
// the row is malformed, and Top would be a claim about the program instead.
func (judgment Judgment) Result(candidate *valuedomain.AllocationResult) (valuedomain.Value, structure.ReductionOutcome) {
	if !judgment.Valid() || !candidate.Owns(judgment.values) {
		return valuedomain.Value{}, structure.Refuse
	}
	_, keyOK := candidate.Key()
	_, identityOK := candidate.KeyID()
	_, coordinateOK := candidate.Coordinate()
	fresh, freshOK := candidate.Fresh()
	if !keyOK || !identityOK || !coordinateOK || !freshOK {
		return valuedomain.Value{}, structure.Refuse
	}
	return fresh, structure.Concrete
}
