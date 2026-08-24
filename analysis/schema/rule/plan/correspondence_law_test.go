package plan

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/rule/program"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

const (
	correspondingRelation   schema.Key = "relation/consumer-corresponding"
	correspondingKey        schema.Key = "projection/consumer-corresponding-key"
	correspondingWrite      schema.Key = "projection/consumer-corresponding-write"
	correspondingReducerKey schema.Key = "reducer/consumer-corresponding"
)

// configureCorrespondenceFixture is the two-directory case: the rule's
// candidate is one axis's occurrence relation, and the single row it joins
// belongs to another axis and is addressed by that axis's OWN directory. The
// two orders are independently enumerated, so the rule's ordinal means nothing
// in the joined one unless the two are declared to enumerate the same
// subjects.
func configureCorrespondenceFixture(t *testing.T, correspond bool) *planFixture {
	t.Helper()
	fixture := newPlanFixture(t)
	occurrenceAxis := schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: planOtherAxisKey}
	consumerAxis := schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: planAxisKey}
	candidate := member.RelationRef{Axis: occurrenceAxis, Member: heteroCandidateRelation}

	occurrenceCatalog, ok := member.NewCatalog(
		[]member.Relation{{
			Key: heteroCandidateRelation, Subject: heteroCandidateCarrier,
			CandidateProvider: member.AxisRelationCandidate(candidate),
		}},
		[]member.Projection{{
			Key: correspondingWrite, Relation: heteroCandidateRelation, Role: member.Destination,
			Result: heteroKeyCarrier, CandidateProvider: member.AxisRelationCandidate(candidate),
		}},
		nil, nil,
	)
	if !ok {
		t.Fatal("occurrence-owner member catalog rejected")
	}

	own := member.RelationRef{Axis: consumerAxis, Member: correspondingRelation}
	relation := member.Relation{
		Key: correspondingRelation, Subject: heteroFactCarrier,
		Inputs:            []member.Carrier{heteroCandidateCarrier},
		CandidateProvider: member.AxisRelationCandidate(own),
	}
	if correspond {
		relation.Correspondences = []member.RelationRef{candidate}
	}
	consumerCatalog, ok := member.NewCatalog(
		[]member.Relation{relation},
		[]member.Projection{
			{
				Key: correspondingKey, Relation: correspondingRelation, Role: member.Key,
				Result: heteroKeyCarrier, CandidateProvider: member.AxisRelationCandidate(own),
			},
		},
		[]member.Reducer{{
			Key: correspondingReducerKey,
			Inputs: []member.ReducerInput{
				{Axis: consumerAxis, Carrier: heteroFactCarrier, Form: member.ReadFormExact, Multiplicity: member.MultiplicityOne},
			},
			Outputs: []member.ReducerOutput{{Axis: consumerAxis, Carrier: heteroFactCarrier}},
		}},
		nil,
	)
	if !ok {
		t.Fatal("consumer member catalog rejected")
	}

	fixture.catalog = consumerCatalog
	fixture.otherCatalog = occurrenceCatalog
	fixture.mainSignature = axis.Signature{Key: heteroKeyCarrier, Fact: heteroFactCarrier}
	fixture.otherSignature = axis.Signature{Key: heteroKeyCarrier, Fact: heteroFactCarrier}
	fixture.declaration = program.Program{
		OperandRole: vocabulary.RoleKey("plan/operand"),
		Candidate:   member.AxisRelationCandidate(candidate),
		Joins: []program.JoinDecl{{
			Sources:  []program.SourceRef{program.CandidateSource()},
			Relation: own,
			Key:      member.ProjectionRef{Axis: consumerAxis, Member: correspondingKey},
			Read: program.ReadDecl{
				PointBound: program.PointBound,
				Input:      0, Axis: program.AxisRef(consumerAxis), Form: program.Exact,
				Contract: program.ReadContract{
					Order: program.OrderCanonical, Sparse: program.SparseExplicit,
					OnOpaque: program.OnOpaquePropagateAuthenticated, Multiplicity: program.MultiplicityOne,
				},
			},
		}},
		Fold: program.FoldDecl{
			Reducer: member.ReducerRef{Axis: consumerAxis, Member: correspondingReducerKey},
			Inputs:  []program.JoinRef{0},
			Outputs: []program.OutputDecl{{
				Column:      axis.OutputRef{Axis: consumerAxis, Key: planOutput},
				Destination: member.ProjectionRef{Axis: occurrenceAxis, Member: correspondingWrite},
				Mode:        program.ModeExact,
				ValueSlot:   0,
			}},
		},
	}
	if problem, valid := fixture.declaration.Check(); !valid {
		t.Fatalf("correspondence program rejected before sealing: %+v", problem)
	}
	return fixture
}

// TestAJoinAddressedByAnotherDirectoryIsRefusedWithoutACorrespondence is the
// soundness law of a rule's one candidate.
//
// A rule resolves one dense candidate and every relation it joins is indexed
// with that ordinal. Two axes describing one subject enumerate their rows
// independently, so indexing one axis's directory with the other's ordinal
// answers whichever row happens to sit at that position. Nothing refused it:
// the plan never compared the join's candidate authority with the rule's, and
// the only thing that stopped it downstream was a generated owner having no
// case to answer with - a refusal by absence, not a law.
func TestAJoinAddressedByAnotherDirectoryIsRefusedWithoutACorrespondence(t *testing.T) {
	fixture := configureCorrespondenceFixture(t, false)
	if _, failure := Compile(fixture.seal(t)); !failure.Available() {
		t.Fatal("a join addressed by a directory the rule's candidate never issued into was admitted")
	}
}

// TestADeclaredCorrespondenceAdmitsAJoinAcrossTwoDirectories is the other
// half. The refusal is not a ban on reading another axis's own rows: it is the
// demand that the two orders be declared to enumerate the same subjects. Once
// the joined relation states that its order corresponds to the candidate's,
// the ordinal means the same row in both and the join compiles.
func TestADeclaredCorrespondenceAdmitsAJoinAcrossTwoDirectories(t *testing.T) {
	fixture := configureCorrespondenceFixture(t, true)
	compiled, failure := Compile(fixture.seal(t))
	if failure.Available() {
		t.Fatalf("a declared correspondence did not admit the join: contributor=%d law=%d disposition=%s", failure.Contributor, failure.Law, failure.Disposition)
	}
	planned, ok := compiled.At(0)
	if !ok || !planned.Present() || planned.JoinCount() != 1 {
		t.Fatalf("compiled plan = present %t joins %d", ok && planned.Present(), planned.JoinCount())
	}
}

// TestACorrespondenceToAnotherOrderDoesNotAdmitTheJoin keeps the admission
// exact. A relation may correspond to several foreign orders, and stating one
// of them says nothing about a rule keyed by a different directory.
func TestACorrespondenceToAnotherOrderDoesNotAdmitTheJoin(t *testing.T) {
	fixture := configureCorrespondenceFixture(t, true)
	occurrenceAxis := schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: planOtherAxisKey}
	relations := append([]member.Relation(nil), fixture.catalog.Relations...)
	relations[0].Correspondences = []member.RelationRef{{Axis: occurrenceAxis, Member: heteroExactRelation}}
	restated, ok := member.NewCatalog(relations, fixture.catalog.Projections, fixture.catalog.Reducers, fixture.catalog.CarryTransforms)
	if !ok {
		t.Fatal("restated consumer catalog rejected")
	}
	fixture.catalog = restated
	if _, failure := Compile(fixture.seal(t)); !failure.Available() {
		t.Fatal("a correspondence to a different order admitted a join the rule's candidate does not address")
	}
}
