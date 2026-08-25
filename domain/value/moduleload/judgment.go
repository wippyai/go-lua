// Package moduleload owns Value's module-load call-result judgment and the
// family its declaration is emitted into. The rule reads the Call fact of the
// mounted occurrence its candidate was sealed for and the Value fact of the
// single actual that call applies, and publishes the module root at the
// call-result coordinate Value already issued for it.
package moduleload

import (
	"github.com/wippyai/go-lua/analysis/schema/structure"
	calldomain "github.com/wippyai/go-lua/domain/call"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// Judgment is the sealed semantic state of the module-load rule: the Value
// schema its answer is expressed in.
//
// It is the family's state, not a rule payload. The schema is cold and
// immutable for the life of the binding it was issued by, so it is sealed once
// when the family is installed and read by every invocation. It is never a
// parameter of the fold: the fold takes the module-load row it is indexed by
// and the two facts it read, and nothing else.
type Judgment struct {
	values *valuedomain.Schema
}

// Derive seals the judgment against the Value schema that owns the candidate
// rows it answers for.
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

// Result is the one irreducible judgment of the module-load rule: the Value
// fact one scoped require call publishes, given the targets that call
// dispatches to and the value of the single actual it is applied to.
//
// A call with no known target and a Bottom actual carry no module evidence, so
// the row settles as an absent candidate. An opaque alternative admits every
// module the host can reach and answers Top. Otherwise the scoped-loader
// alternative is the only one this rule interprets: other selected targets are
// owned by their own result consumers, and a call that reaches no scoped
// loader contributes nothing here. A candidate that carries no required
// operation is refused rather than widened - Top would assert that every value
// is a possible module root, which is a claim about the program rather than an
// admission that the row is malformed.
func (judgment Judgment) Result(candidate valuedomain.ModuleLoadCall, argument valuedomain.Value, dispatched calldomain.Value) (valuedomain.Value, structure.ReductionOutcome) {
	if !judgment.Valid() || !callValueValid(dispatched) || !judgment.values.OwnsModuleLoadCall(candidate) {
		return valuedomain.Value{}, structure.Refuse
	}
	if argument.IsBottom() || dispatched.IsEmpty() {
		return judgment.values.Bottom(), structure.NoCandidate
	}
	if dispatched.HasOpaqueAlternative() {
		return judgment.values.Top(), structure.Concrete
	}
	if dispatched.KnownTargetCount() == 0 {
		return judgment.values.Bottom(), structure.NoCandidate
	}
	require, requireOK := candidate.RequireOperation()
	if !requireOK {
		return valuedomain.Value{}, structure.Refuse
	}
	scopedLoader := false
	for index := 0; index < dispatched.KnownTargetCount(); index++ {
		target, targetOK := dispatched.KnownTargetAt(index)
		if !targetOK {
			return valuedomain.Value{}, structure.Refuse
		}
		// Other selected targets are owned by their own result consumers. This
		// rule contributes only the scoped-loader alternative; widening here
		// would erase otherwise precise results for every ordinary unary call.
		if !target.IsScopedLoader() {
			continue
		}
		operation, operationOK := target.Operation()
		if !operationOK || operation != require {
			return valuedomain.Value{}, structure.Refuse
		}
		scopedLoader = true
	}
	if !scopedLoader {
		return judgment.values.Bottom(), structure.NoCandidate
	}
	expected, expectedOK := candidate.ExpectedArgument()
	fact, factOK := candidate.ResultFact()
	if !expectedOK || !factOK || argument.IsTop() || !judgment.values.Equal(argument, expected) {
		return judgment.values.Top(), structure.Concrete
	}
	return fact, structure.Concrete
}

// callValueValid states the shape a Call fact must have before this rule reads
// it: one of the four dispositions Call's algebra publishes.
func callValueValid(fact calldomain.Value) bool {
	return fact.IsTop() || fact.IsOpen() || fact.IsComplete() || fact.IsEmpty()
}
