package plan

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/rule/program"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

// activation_branch_law_test.go states how the branch vocabulary of a
// structural publication compiles: each identity is authenticated against the
// relation it must be a column of, and against the role that says it is an
// identity at all.
//
// A local projected here would be a dense coordinate standing in for a module
// or a semantic axis - an address of a row this analyzer minted, put where the
// name of something it did not mint belongs.

const (
	branchIdentityCarrier member.Carrier = "carrier/plan/branch-identity"
	branchApplicationKey  schema.Key     = "projection/plan-branch-application"
	branchTargetKey       schema.Key     = "projection/plan-branch-target"
	branchEndpointKey     schema.Key     = "projection/plan-branch-endpoint"
	branchMountKey        schema.Key     = "projection/plan-branch-mount"
	branchBodyKey         schema.Key     = "projection/plan-branch-body"
)

// newActivationBranchFixture is the whole A form at the plan layer: the cold
// branch set of the nested-member-set fixture, folded directly, publishing
// structurally, with a transport vector, a family and the five identities.
func newActivationBranchFixture(t *testing.T) *planFixture {
	t.Helper()
	mainAxis := schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: planAxisKey}
	candidate := member.AxisRelationCandidate(member.RelationRef{Axis: mainAxis, Member: planCandidateRelation})

	fixture := newPlanFixture(t)
	addNestedMemberSetCatalog(fixture)
	identity := func(key schema.Key, relation schema.Key) member.Projection {
		return member.Projection{
			Key: key, Relation: relation, Role: member.Identity,
			Result: branchIdentityCarrier, CandidateProvider: candidate,
		}
	}
	fixture.catalog.Projections = append(fixture.catalog.Projections,
		identity(branchApplicationKey, planCandidateRelation),
		identity(branchTargetKey, nestedMemberRelation),
		identity(branchEndpointKey, nestedMemberRelation),
		identity(branchMountKey, nestedMemberRelation),
		identity(branchBodyKey, nestedMemberRelation),
	)
	fixture.declaration.Joins = nestedMemberSetJoins(planCandidateRelation)[:1]
	fixture.declaration.Fold.Inputs = []program.JoinRef{0}
	fixture.declaration.Fold.Outputs[0].Mode = program.ModeStructural
	fixture.catalog.Reducers[0].Inputs = []member.ReducerInput{{
		Axis:         mainAxis,
		Carrier:      planFactCarrier,
		Form:         member.ReadFormSummary,
		Multiplicity: member.MultiplicityMany,
		Tag:          nestedMemberOrdinal,
	}}
	fixture.declaration.Transport = []program.TransportDecl{{Axis: program.AxisRef(mainAxis)}}
	fixture.declaration.ActivationRole = vocabulary.RoleKey("plan/activation-family")
	fixture.declaration.Activation = &program.ActivationDecl{
		Branch:      0,
		Application: member.ProjectionRef{Axis: mainAxis, Member: branchApplicationKey},
		Target:      member.ProjectionRef{Axis: mainAxis, Member: branchTargetKey},
		Endpoint:    member.ProjectionRef{Axis: mainAxis, Member: branchEndpointKey},
		Mount:       member.ProjectionRef{Axis: mainAxis, Member: branchMountKey},
		Body:        member.ProjectionRef{Axis: mainAxis, Member: branchBodyKey},
	}
	return fixture
}

func compileActivationBranchFixture(t *testing.T, fixture *planFixture) (Activation, bool) {
	t.Helper()
	compiled, failure := Compile(fixture.seal(t))
	if failure.Available() {
		return Activation{}, false
	}
	plan, planOK := compiled.At(0)
	if !planOK || !plan.Present() {
		t.Fatal("compiled plan missing")
	}
	return plan.ActivationBranch()
}

// TestTheBranchVocabularyCompilesToDenseIdentityAddresses is the positive law.
// Every identity resolves to its own axis's projection ordinal, and the branch
// reference resolves to the join whose members the vocabulary describes.
func TestTheBranchVocabularyCompilesToDenseIdentityAddresses(t *testing.T) {
	branch, present := compileActivationBranchFixture(t, newActivationBranchFixture(t))
	if !present {
		t.Fatal("a structural publication compiled no branch vocabulary")
	}
	if branch.Branch != 0 {
		t.Fatalf("branch join = %d, want the vector read its members hang under", branch.Branch)
	}
	addresses := map[string]ProjectionAddr{
		"application": branch.Application, "target": branch.Target,
		"endpoint": branch.Endpoint, "mount": branch.Mount, "body": branch.Body,
	}
	seen := map[ProjectionAddr]string{}
	for name, address := range addresses {
		if address.Axis != 0 {
			t.Fatalf("%s resolved on axis %d, want the rule's own axis", name, address.Axis)
		}
		if other, duplicate := seen[address]; duplicate {
			t.Fatalf("%s and %s resolved to one projection ordinal", name, other)
		}
		seen[address] = name
	}
}

// TestABranchIdentityIsAColumnOfTheRelationItDescribes keeps each identity
// where it belongs. The application is the TRIGGER's - every branch of one
// trigger is an alternative of the same application - and the other four
// distinguish one branch from another, so they are the branch row's.
func TestABranchIdentityIsAColumnOfTheRelationItDescribes(t *testing.T) {
	fixture := newActivationBranchFixture(t)
	mainAxis := schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: planAxisKey}
	fixture.declaration.Activation.Application = member.ProjectionRef{Axis: mainAxis, Member: branchTargetKey}
	if _, present := compileActivationBranchFixture(t, fixture); present {
		t.Fatal("an application projected from a branch row compiled")
	}

	fixture = newActivationBranchFixture(t)
	fixture.declaration.Activation.Target = member.ProjectionRef{Axis: mainAxis, Member: branchApplicationKey}
	if _, present := compileActivationBranchFixture(t, fixture); present {
		t.Fatal("a target projected from the trigger row compiled")
	}
}

// TestABranchIdentityIsDeclaredInTheIdentityRole is the fence that keeps a
// dense coordinate out of a position that names a module. The nested set's own
// Key projection is a perfectly good projection of the right relation, and it
// is refused because a key is a local.
func TestABranchIdentityIsDeclaredInTheIdentityRole(t *testing.T) {
	fixture := newActivationBranchFixture(t)
	mainAxis := schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: planAxisKey}
	fixture.declaration.Activation.Mount = member.ProjectionRef{Axis: mainAxis, Member: nestedMemberKey}
	if _, present := compileActivationBranchFixture(t, fixture); present {
		t.Fatal("a local projection compiled as a branch identity")
	}
}

// TestAFactWritingRuleCompilesNoBranchVocabulary is the negative half of the
// biconditional at the layer that seals it: a plan with no transport vector
// carries no branch, so nothing downstream has to ask whether one is
// meaningful.
func TestAFactWritingRuleCompilesNoBranchVocabulary(t *testing.T) {
	compiled, failure := Compile(newPlanFixture(t).seal(t))
	if failure.Available() {
		t.Fatalf("the ordinary fixture was rejected: %+v", failure)
	}
	plan, planOK := compiled.At(0)
	if !planOK || !plan.Present() {
		t.Fatal("compiled plan missing")
	}
	if branch, present := plan.ActivationBranch(); present || branch != (Activation{}) {
		t.Fatalf("a fact-writing plan carries a branch vocabulary: %+v/%t", branch, present)
	}
}
