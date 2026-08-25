package relcompile_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/rule/relcompile"
)

// The four canonical diagnostic observation kinds and the diagnostic-code
// register.
//
// analysis/schema/structure/diagnostic_observation.go declares exactly four
// observation kinds and their ordinals are ABI-bearing. The type-conformance
// kind is minted at four authored sites discriminated by an observation-site
// scalar, so it is one relation with four alternative derivations rather than
// four kinds.
//
// Each kind's population is a Complete over the denominator its declaration
// names, and every skip today spelled as an early return in the observation
// builder is one of two relational statements: a row that is not in the
// population is filtered by an equijoin onto the fact relation that states the
// filter, and a row that is in the population but incomplete is a refusal the
// Complete makes visible. Neither is a silent exit.
//
// The evidence and path sidecars disappear as columns. Their offset and count
// arithmetic is child-relation membership: one child row per evidence point,
// keyed by its parent observation.

// census2BranchConditionPlan is B1.
//
// The population is the guarded routes: a route with no decision states no
// branch condition, so the guard is a join onto the relation that states a
// route carries a decision, and the observation is closed against that
// population. A scope-sensitive body is likewise a declared fact joined onto
// the branch, never a body-shape test the builder performs.
func census2BranchConditionPlan(t *testing.T) *census2Plane {
	t.Helper()
	plane := newCensus2Plane(t)
	route := plane.program("program/causal-final-route")
	guarded := plane.program("program/route-guard-proof")
	branch := plane.program("program/authored-branch")
	insensitive := plane.program("program/containment-scope-insensitive-body")
	span := plane.program("program/span")
	source := plane.program("program/source-span")
	pointPath := plane.program("program/local-wto-point-path")
	observation := plane.observation("observation/branch-condition")
	evidence := plane.observation("observation/branch-condition/evidence")
	admit := plane.exactOperation(plane.program("program/branch-condition-admit"), observation)

	plane.add(census2Step{
		Key: "observation/branch-condition", Candidate: route,
		Joins: []relcompile.JoinSpec{
			plane.join(route, guarded),
			plane.join(route, branch),
			plane.join(branch, insensitive),
			plane.join(route, span),
			plane.join(span, source)},
		Complete: census2Ref(guarded),
		Apply:    admit,
		Publish:  observation,
	})
	plane.add(census2Step{
		Key: "observation/branch-condition-evidence", Candidate: observation,
		Joins:   []relcompile.JoinSpec{plane.join(observation, pointPath)},
		Publish: evidence,
	})
	return plane
}

// census2TypeReferenceUnresolvedPlan is B2.
//
// The population is the declared static types; the unresolved reference is the
// join onto the resolution relation that states a reference reached no target.
// Closing the observation against the static-type denominator is what makes an
// unresolved reference an answer rather than an absence nothing reports.
func census2TypeReferenceUnresolvedPlan(t *testing.T) *census2Plane {
	t.Helper()
	plane := newCensus2Plane(t)
	reference := plane.program("program/static-reference")
	unresolved := plane.program("program/static-reference-unresolved")
	staticType := plane.program("program/static-type")
	source := plane.program("program/static-reference-source")
	span := plane.program("program/source-span")
	observation := plane.observation("observation/type-reference-unresolved")
	path := plane.observation("observation/type-reference-unresolved/path")
	admit := plane.exactOperation(plane.program("program/type-reference-admit"), observation)

	plane.add(census2Step{
		Key: "observation/type-reference-unresolved", Candidate: reference,
		Joins: []relcompile.JoinSpec{
			plane.join(reference, unresolved),
			plane.join(reference, staticType),
			plane.join(reference, source),
			plane.join(reference, span)},
		Complete: census2Ref(staticType),
		Apply:    admit,
		Publish:  observation,
	})
	plane.add(census2Step{
		Key: "observation/type-reference-unresolved-path", Candidate: observation,
		Joins:   []relcompile.JoinSpec{plane.join(observation, source)},
		Publish: path,
	})
	return plane
}

// census2ValueReferenceUnresolvedPlan is B3.
//
// The population is the implicit reads; the unresolved value reference is the
// join onto the global storage cell that no body declares. Both halves are
// declared facts of the authored storage relations.
func census2ValueReferenceUnresolvedPlan(t *testing.T) *census2Plane {
	t.Helper()
	plane := newCensus2Plane(t)
	read := plane.program("program/authored-storage-read")
	implicit := plane.program("program/authored-storage-read-implicit")
	cell := plane.program("program/authored-storage-cell")
	global := plane.program("program/authored-storage-cell-global-undeclared")
	key := plane.program("program/source-key")
	span := plane.program("program/source-span")
	observation := plane.observation("observation/value-reference-unresolved")
	admit := plane.exactOperation(plane.program("program/value-reference-admit"), observation)

	plane.add(census2Step{
		Key: "observation/value-reference-unresolved", Candidate: read,
		Joins: []relcompile.JoinSpec{
			plane.join(read, implicit),
			plane.join(read, cell),
			plane.join(cell, global),
			plane.join(read, key),
			plane.join(read, span)},
		Complete: census2Ref(implicit),
		Apply:    admit,
		Publish:  observation,
	})
	return plane
}

// census2TypeConformancePlan is B4: one relation, four derivations.
//
// A call argument, an assignment bind, an assignment write and a structural
// member are four authored sites that mint one observation kind. They are four
// alternative derivations of one relation, so the relation's rows are their
// merge and the observation-site scalar is a column of the row rather than a
// discriminator a consumer switches on.
//
// The structural arm is recursive: a declared structural target's member is
// itself measured against its declared type. The visited set the recursive
// walker maintains is SCC membership, and the open-tail suppression is a join
// onto the allocation row that states the tail is closed.
//
// The completion is separate from the arms. The four arms are the derivations;
// closing their relation against the declared-typed positions is what states
// that a declared position with no measurement is a missing answer rather than
// a conforming one.
func census2TypeConformancePlan(t *testing.T) *census2Plane {
	t.Helper()
	plane := newCensus2Plane(t)
	call := plane.program("program/authored-call")
	formal := plane.program("program/function-boundary-formal")
	argumentSource := plane.program("program/call-argument-source")
	declaredType := plane.program("program/declared-static-type")
	pointPath := plane.program("program/local-wto-point-path")
	bind := plane.program("program/authored-storage-bind")
	assign := plane.program("program/authored-storage-assign")
	write := plane.program("program/authored-storage-write")
	member := plane.program("program/authored-values-member")
	allocationField := plane.program("program/allocation-field")
	structuralTarget := plane.program("program/declared-structural-target")
	declaredPositions := plane.program("program/declared-typed-position")
	observation := plane.observation("observation/type-conformance")
	evidence := plane.observation("observation/type-conformance/evidence")
	closed := plane.observation("observation/type-conformance/closed")
	measure := plane.exactOperation(plane.program("program/type-conformance-measure"), observation)

	plane.add(census2Step{
		Key: "observation/type-conformance-call-argument", Candidate: call,
		Joins: []relcompile.JoinSpec{
			plane.join(call, formal),
			plane.join(call, argumentSource),
			plane.join(call, declaredType),
			plane.join(call, pointPath)},
		Apply:   measure,
		Publish: observation,
	})
	plane.add(census2Step{
		Key: "observation/type-conformance-assignment-bind", Candidate: bind,
		Joins: []relcompile.JoinSpec{
			plane.join(bind, member),
			plane.join(bind, declaredType),
			plane.join(bind, pointPath)},
		Apply:   measure,
		Publish: observation,
	})
	plane.add(census2Step{
		Key: "observation/type-conformance-assignment-write", Candidate: assign,
		Joins: []relcompile.JoinSpec{
			plane.join(assign, write),
			plane.join(assign, member),
			plane.join(assign, declaredType),
			plane.join(assign, pointPath)},
		Apply:   measure,
		Publish: observation,
	})
	plane.add(census2Step{
		Key: "observation/type-conformance-structural", Candidate: allocationField,
		Joins: []relcompile.JoinSpec{
			plane.join(allocationField, structuralTarget),
			plane.join(allocationField, member),
			plane.join(allocationField, observation)},
		Apply:   measure,
		Publish: observation,
	})
	plane.add(census2Step{
		Key: "observation/type-conformance-closure", Candidate: observation,
		Complete: census2Ref(declaredPositions),
		Publish:  closed,
	})
	plane.add(census2Step{
		Key: "observation/type-conformance-evidence", Candidate: observation,
		Joins:   []relcompile.JoinSpec{plane.join(observation, pointPath)},
		Publish: evidence,
	})
	return plane
}

// The composite diagnostic-code vocabulary: the two halves of one published
// external identity.
//
// domain/composite holds a sealed declaration table whose lanes install
// producers, and a declared-not-composed register that names a code with the
// surface that owes its judgment. Membership in the table is not the line
// between them: two table rows are in the register too, because their lane
// installs no collector. The line is the producer.
//
// Both halves are read-only relations over the same code relation, so neither
// publishes and neither ends in a semantic operation. The composed half closes
// the code against the producer a sealed lane installs; the declared half is
// the register's own rows, which carry the owing surface and the missing
// observation as columns rather than as prose a consumer parses.

// census2DiagnosticComposedPlan is the composed half.
func census2DiagnosticComposedPlan(t *testing.T) *census2Plane {
	t.Helper()
	plane := newCensus2Plane(t)
	code := plane.diagnostic("diagnostic/code")
	producer := plane.diagnostic("diagnostic/producer")
	plane.add(census2Step{
		Key: "diagnostic/code-composed", Candidate: code,
		Joins: []relcompile.JoinSpec{plane.completedJoin(code, producer)},
	})
	return plane
}

// census2DiagnosticDeclaredPlan is the declared-not-composed half.
func census2DiagnosticDeclaredPlan(t *testing.T) *census2Plane {
	t.Helper()
	plane := newCensus2Plane(t)
	code := plane.diagnostic("diagnostic/code")
	register := plane.diagnostic("diagnostic/declared-register")
	plane.add(census2Step{
		Key: "diagnostic/code-declared", Candidate: register,
		Joins: []relcompile.JoinSpec{plane.join(register, code)},
	})
	return plane
}
