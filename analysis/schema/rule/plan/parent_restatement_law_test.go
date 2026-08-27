package plan

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/carrier"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/rule/program"
)

// This file proves the Parent restatement compiled by Compile (plan.go,
// Join.Parent/ParentPresent): a Summary join over a self-provided nested
// member set states, on the JoinDecl, the same Parent the resolved relation's
// own catalog entry declares, and Compile authenticates the two agree before
// emitting a dense Parent address.
//
// The fixture adds two relations to the base planFixture catalog:
//
//   - nestedMemberRelation is a nested member set: its own Parent names
//     planCandidateRelation and it declares an Ordinal carrier, so
//     member.Relation.Nested() holds.
//   - consumerRelation is an ordinary relation that reads the nested
//     relation's fact as its own Input, so a Program built from the two joins
//     has every join reachable from a single Fold input without needing the
//     nested Summary read itself to carry a reducer tag.

const (
	nestedMemberRelation      schema.Key  = "relation/plan-nested-member"
	nestedMemberKey           schema.Key  = "projection/plan-nested-member-key"
	nestedMemberOrdinal       carrier.Key = "carrier/plan/nested-member-ordinal"
	consumerRelationKey       schema.Key  = "relation/plan-nested-consumer"
	consumerRelationProj      schema.Key  = "projection/plan-nested-consumer-key"
	consumerRelationSelection schema.Key  = "selection/plan-nested-consumer"
)

// addNestedMemberSetCatalog extends fixture's member catalog with a nested
// member set relation (Parent=planCandidateRelation, Ordinal declared) and a
// downstream consumer relation that reads the nested relation's fact. Both
// are appended after the base fixture's own two relations, so their dense
// ordinals are 2 and 3.
func addNestedMemberSetCatalog(fixture *planFixture) {
	mainAxis := schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: planAxisKey}
	candidate := member.AxisRelationCandidate(member.RelationRef{Axis: mainAxis, Member: planCandidateRelation})
	fixture.catalog.Authorities = append(fixture.catalog.Authorities, carrier.Authority{
		Carrier: nestedMemberOrdinal, Capability: carrier.DecodeOnly,
	})

	fixture.catalog.Relations = append(fixture.catalog.Relations,
		member.Relation{
			Key:               nestedMemberRelation,
			Subject:           planFactCarrier,
			Inputs:            []carrier.Key{planCandidateCarrier},
			CandidateProvider: candidate,
			Parent:            member.RelationRef{Axis: mainAxis, Member: planCandidateRelation},
			Ordinal:           nestedMemberOrdinal,
		},
		member.Relation{
			Key:               consumerRelationKey,
			Subject:           planFactCarrier,
			Inputs:            []carrier.Key{planFactCarrier},
			CandidateProvider: candidate,
		},
	)
	fixture.catalog.Projections = append(fixture.catalog.Projections,
		member.Projection{
			Key:               nestedMemberKey,
			Relation:          nestedMemberRelation,
			Role:              member.Key,
			Result:            planKeyCarrier,
			CandidateProvider: candidate,
		},
		member.Projection{
			Key:               consumerRelationProj,
			Relation:          consumerRelationKey,
			Role:              member.Key,
			Result:            planKeyCarrier,
			CandidateProvider: candidate,
		},
	)
}

// nestedMemberSetJoins builds the two-join declaration list: a Summary read
// over nestedMemberRelation restating parentMember as its Parent, followed by
// an Exact join over consumerRelationKey that sources the nested join's fact
// (PriorSource(0)). The second join is what a single Fold input can name
// without requiring a reducer tag on the nested Summary read itself; the
// nested join is still reached, through the source graph, by
// Program.checkReachability.
func nestedMemberSetJoins(parentMember schema.Key) []program.JoinDecl {
	mainAxis := schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: planAxisKey}
	denominatorRef := program.DenominatorRef{Surface: schema.SurfaceKindDenominator, Key: planDenominator}
	return []program.JoinDecl{
		{
			Sources:  []program.SourceRef{program.CandidateSource()},
			Relation: member.RelationRef{Axis: mainAxis, Member: nestedMemberRelation},
			Key:      member.ProjectionRef{Axis: mainAxis, Member: nestedMemberKey},
			Parent:   member.RelationRef{Axis: mainAxis, Member: parentMember},
			Read: program.ReadDecl{
				PointBound: program.PointBound,
				Axis:       program.AxisRef(mainAxis),
				Form:       program.Summary,
				Contract: program.ReadContract{
					Order:          program.OrderCanonical,
					Sparse:         program.SparseExplicit,
					OnOpaque:       program.OnOpaqueRefuse,
					Multiplicity:   program.MultiplicityMany,
					DenominatorRef: denominatorRef,
				},
			},
		},
		{
			Sources:   []program.SourceRef{program.PriorSource(0)},
			Relation:  member.RelationRef{Axis: mainAxis, Member: consumerRelationKey},
			Key:       member.ProjectionRef{Axis: mainAxis, Member: consumerRelationProj},
			Selection: member.SelectionRef{Axis: mainAxis, Member: consumerRelationSelection},
			Read: program.ReadDecl{
				PointBound: program.PointBound,
				Axis:       program.AxisRef(mainAxis),
				Form:       program.Exact,
				Contract: program.ReadContract{
					Order:          program.OrderCanonical,
					Sparse:         program.SparseExplicit,
					OnOpaque:       program.OnOpaqueRefuse,
					Multiplicity:   program.MultiplicityOne,
					DenominatorRef: denominatorRef,
				},
			},
		},
	}
}

// newNestedMemberSetFixture returns a fixture whose declaration is the
// nestedMemberSetJoins pair, folded through the consumer join (index 1), with
// the nested join's Parent restatement set to parentMember.
func newNestedMemberSetFixture(t *testing.T, parentMember schema.Key) *planFixture {
	t.Helper()
	fixture := newPlanFixture(t)
	addNestedMemberSetCatalog(fixture)
	fixture.declaration.Joins = nestedMemberSetJoins(parentMember)
	fixture.declaration.Fold.Inputs = []program.JoinRef{1}
	return fixture
}

// TestCompileAuthenticatesRestatedParentAgainstNestedMemberSet proves law 1:
// a Summary join over a self-provided nested member set, whose JoinDecl
// restates the relation's own declared Parent, compiles to a Join with
// ParentPresent true and Parent naming the parent relation's dense ordinal on
// the relation's own axis.
func TestCompileAuthenticatesRestatedParentAgainstNestedMemberSet(t *testing.T) {
	fixture := newNestedMemberSetFixture(t, planCandidateRelation)
	table := fixture.seal(t)
	compiled, failure := Compile(table)
	if failure.Available() {
		t.Fatalf("valid nested-member-set restatement rejected: contributor=%d law=%d disposition=%s", failure.Contributor, failure.Law, failure.Disposition)
	}
	plan, ok := compiled.At(0)
	if !ok || !plan.Present() {
		t.Fatal("compiled plan missing")
	}

	nestedJoin, ok := plan.JoinAt(0)
	if !ok {
		t.Fatal("nested member-set join missing")
	}
	wantParent := plan.Candidate()
	if !nestedJoin.ParentPresent || nestedJoin.Parent != wantParent {
		t.Fatalf("nested join parent = %+v present=%t, want %+v present=true", nestedJoin.Parent, nestedJoin.ParentPresent, wantParent)
	}
	if nestedJoin.Relation != (RelationAddr{Axis: 0, Member: 2}) {
		t.Fatalf("nested join relation address = %+v, want axis 0 member 2", nestedJoin.Relation)
	}
	if nestedJoin.PredicatePresent {
		t.Fatal("nested member-set join unexpectedly carries a predicate")
	}
	if nestedJoin.ReadForm != program.Summary {
		t.Fatalf("nested join read form = %v, want Summary", nestedJoin.ReadForm)
	}

	consumerJoin, ok := plan.JoinAt(1)
	if !ok {
		t.Fatal("consumer join missing")
	}
	if consumerJoin.ParentPresent {
		t.Fatal("consumer join unexpectedly carries a parent restatement")
	}
	if consumerJoin.Relation != (RelationAddr{Axis: 0, Member: 3}) {
		t.Fatalf("consumer join relation address = %+v, want axis 0 member 3", consumerJoin.Relation)
	}
}

// TestCompileRejectsParentRestatementOverNonNestedRelation proves law 2: a
// JoinDecl restating a Parent over a relation that the catalog does not
// declare as a nested member set (no Parent/Ordinal of its own) is malformed.
// The declaration itself stays well-formed - ReadFormAddressing does not
// condition an Exact read's addressing on whether a Parent is declared - so
// the refusal is Compile's own authentication against the resolved relation.
func TestCompileRejectsParentRestatementOverNonNestedRelation(t *testing.T) {
	fixture := newPlanFixture(t)
	mainAxis := schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: planAxisKey}
	fixture.declaration.Joins[0].Parent = member.RelationRef{Axis: mainAxis, Member: planCandidateRelation}
	if _, valid := fixture.declaration.Check(); !valid {
		t.Fatal("restating a parent over an exact join unexpectedly failed declaration-level Check")
	}
	table := fixture.seal(t)
	compiled, failure := Compile(table)
	if !failure.Available() {
		t.Fatal("parent restatement over a non-nested relation compiled without a failure")
	}
	if failure.Contributor != schema.SurfaceKindRule || failure.Law != rule.LawProgramShape || failure.Disposition != schema.DispositionMalformed {
		t.Fatalf("failure = contributor %d law %d disposition %s, want rule/LawProgramShape/malformed", failure.Contributor, failure.Law, failure.Disposition)
	}
	if compiled.Available() {
		t.Fatal("failed compilation was not fail-closed")
	}
}

// TestCompileRejectsParentRestatementNamingWrongRelation proves law 3: a
// JoinDecl whose restated Parent names a different, real relation than the
// one the resolved relation's own catalog entry declares as its Parent is
// malformed. The mismatch is caught before the named relation is even
// resolved, since the two RelationRef values already disagree.
func TestCompileRejectsParentRestatementNamingWrongRelation(t *testing.T) {
	fixture := newNestedMemberSetFixture(t, consumerRelationKey)
	if _, valid := fixture.declaration.Check(); !valid {
		t.Fatal("restating a real but wrong parent unexpectedly failed declaration-level Check")
	}
	table := fixture.seal(t)
	compiled, failure := Compile(table)
	if !failure.Available() {
		t.Fatal("parent restatement naming the wrong relation compiled without a failure")
	}
	if failure.Contributor != schema.SurfaceKindRule || failure.Law != rule.LawProgramShape || failure.Disposition != schema.DispositionMalformed {
		t.Fatalf("failure = contributor %d law %d disposition %s, want rule/LawProgramShape/malformed", failure.Contributor, failure.Law, failure.Disposition)
	}
	if compiled.Available() {
		t.Fatal("failed compilation was not fail-closed")
	}
}

// TestProgramRejectsUncorrelatedSummaryOverNestedMemberSet proves law 4: a
// Summary join that restates no Parent and declares no Predicate correlates
// its cells with nothing. ReadFormAddressing refuses that combination in the
// declaration's own normal form, so the JoinDecl never reaches sealing, let
// alone Compile.
func TestProgramRejectsUncorrelatedSummaryOverNestedMemberSet(t *testing.T) {
	fixture := newPlanFixture(t)
	fixture.declaration.Joins[0].Read.Form = program.Summary
	problem, valid := fixture.declaration.Check()
	if valid {
		t.Fatal("an uncorrelated summary join was accepted by declaration-level Check")
	}
	if problem.Kind != program.ProblemJoin || problem.Join != 0 {
		t.Fatalf("problem = %+v, want ProblemJoin at join 0", problem)
	}
}

// TestANestedMemberSetIsFoldableAtItsOwnOrdinal states the tag law a nested
// member set has always owed and never been asked for.
//
// A join's reducer tag names WHICH member of a many-valued delivery each cell
// is. For a selection that is the Predicate projection. For a nested member
// set there is no predicate and there never should be: the set is addressed by
// (parent, ordinal), and member.Relation.Ordinal is by its own declaration
// "the address a member is reached by under its parent ... what a CHILD
// Program consumes". So the ordinal carrier IS the tag.
//
// Until this held, a nested Summary could be declared and compiled but could
// never be a Fold input, because the compiler offered its reducer no tag to
// agree with. The fixture above says so in as many words - its consumer join
// exists "without requiring a reducer tag on the nested Summary read itself".
// A rule whose whole judgment is over the member set - one disposition per
// branch of one trigger - has no such second join to fold through.
func TestANestedMemberSetIsFoldableAtItsOwnOrdinal(t *testing.T) {
	fixture := newPlanFixture(t)
	addNestedMemberSetCatalog(fixture)
	// The nested Summary read is the fold's own input, not a materialization
	// some later exact join consumes, so it is the only join there is.
	fixture.declaration.Joins = nestedMemberSetJoins(planCandidateRelation)[:1]
	fixture.declaration.Fold.Inputs = []program.JoinRef{0}
	fixture.catalog.Reducers[0].Inputs = []member.ReducerInput{{
		Axis:         schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: planAxisKey},
		Carrier:      planFactCarrier,
		Form:         member.ReadFormSummary,
		Multiplicity: member.MultiplicityMany,
		Tag:          nestedMemberOrdinal,
	}}
	table := fixture.seal(t)
	compiled, failure := Compile(table)
	if failure.Available() {
		t.Fatalf("a fold over a nested member set was rejected: contributor=%d law=%d disposition=%s", failure.Contributor, failure.Law, failure.Disposition)
	}
	plan, ok := compiled.At(0)
	if !ok || !plan.Present() {
		t.Fatal("compiled plan missing")
	}
	join, joinOK := plan.JoinAt(0)
	if !joinOK || !join.ParentPresent || join.PredicatePresent {
		t.Fatalf("nested join parent=%t predicate=%t, want a parent-addressed member set", join.ParentPresent, join.PredicatePresent)
	}
}

// TestANestedMemberSetFoldRefusesAForeignTag keeps the tag exact. The ordinal
// carrier the relation declares is the only address its members have; a
// reducer naming any other carrier is agreeing with a delivery it will not be
// handed.
func TestANestedMemberSetFoldRefusesAForeignTag(t *testing.T) {
	fixture := newPlanFixture(t)
	addNestedMemberSetCatalog(fixture)
	fixture.declaration.Joins = nestedMemberSetJoins(planCandidateRelation)[:1]
	fixture.declaration.Fold.Inputs = []program.JoinRef{0}
	fixture.catalog.Reducers[0].Inputs = []member.ReducerInput{{
		Axis:         schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: planAxisKey},
		Carrier:      planFactCarrier,
		Form:         member.ReadFormSummary,
		Multiplicity: member.MultiplicityMany,
		Tag:          planKeyCarrier,
	}}
	table := fixture.seal(t)
	if _, failure := Compile(table); !failure.Available() {
		t.Fatal("a fold over a nested member set accepted a tag the relation does not address its members by")
	}
}
