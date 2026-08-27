package plan

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/carrier"
	"github.com/wippyai/go-lua/analysis/schema/rule/program"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

const (
	cellRelation      schema.Key = "relation/consumer-cell"
	cellDestination   schema.Key = "projection/consumer-cell-destination"
	cellReducer       schema.Key = "reducer/consumer-cell"
	strayRelation     schema.Key = "relation/consumer-stray"
	strayDestination  schema.Key = "projection/consumer-stray-destination"
	foreignExactRead  schema.Key = "relation/consumer-foreign-exact"
	foreignExactKeyed schema.Key = "projection/consumer-foreign-exact-key"
)

// configureForeignCandidateFixture builds the Typestate-shaped case: the
// candidate is another axis's sealed occurrence relation, the one join is an
// exact read of that axis's fact, and the write goes to a cell this axis
// declared for that same candidate.
func configureForeignCandidateFixture(t *testing.T, provider member.RelationRef, destinationProjection schema.Key) *planFixture {
	t.Helper()
	fixture := newPlanFixture(t)
	occurrenceAxis := schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: planOtherAxisKey}
	consumerAxis := schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: planAxisKey}
	candidate := member.RelationRef{Axis: occurrenceAxis, Member: heteroCandidateRelation}

	occurrenceCatalog, ok := member.NewCatalog(
		[]carrier.Authority{
			{Carrier: heteroCandidateCarrier, Capability: carrier.DecodeOnly},
			{Carrier: heteroKeyCarrier, Capability: carrier.Equatable},
			{Carrier: heteroFactCarrier, Capability: carrier.Ascending},
		},
		[]carrier.Binding{},
		[]member.Relation{
			{Key: heteroCandidateRelation, Subject: heteroCandidateCarrier, CandidateProvider: member.AxisRelationCandidate(candidate)},
			{Key: foreignExactRead, Subject: heteroFactCarrier, Inputs: []carrier.Key{heteroCandidateCarrier}, CandidateProvider: member.AxisRelationCandidate(candidate)},
		},
		[]member.Projection{{
			Key: foreignExactKeyed, Relation: foreignExactRead, Role: member.Key,
			Result: heteroKeyCarrier, CandidateProvider: member.AxisRelationCandidate(candidate),
		}},
		nil, nil,
	)
	if !ok {
		t.Fatal("occurrence-owner member catalog rejected")
	}
	consumerCatalog, ok := member.NewCatalog(
		[]carrier.Authority{
			{Carrier: heteroKeyCarrier, Capability: carrier.Equatable},
			{Carrier: heteroFactCarrier, Capability: carrier.Ascending},
		},
		[]carrier.Binding{{
			Use: heteroCandidateCarrier,
			Ref: carrier.Ref{Owner: occurrenceAxis, Carrier: heteroCandidateCarrier},
		}},
		[]member.Relation{
			{Key: cellRelation, Subject: heteroFactCarrier, Inputs: []carrier.Key{heteroCandidateCarrier}, CandidateProvider: member.AxisRelationCandidate(provider)},
			{Key: strayRelation, Subject: heteroFactCarrier, Inputs: []carrier.Key{heteroCandidateCarrier}, CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: occurrenceAxis, Member: foreignExactRead})},
		},
		[]member.Projection{
			{Key: cellDestination, Relation: cellRelation, Role: member.Destination, Result: heteroKeyCarrier, CandidateProvider: member.AxisRelationCandidate(provider)},
			{Key: strayDestination, Relation: strayRelation, Role: member.Destination, Result: heteroKeyCarrier, CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: occurrenceAxis, Member: foreignExactRead})},
		},
		[]member.Reducer{{
			Key: cellReducer,
			Inputs: []member.ReducerInput{
				{Axis: occurrenceAxis, Carrier: heteroFactCarrier, Form: member.ReadFormExact, Multiplicity: member.MultiplicityOne},
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
			Relation: member.RelationRef{Axis: occurrenceAxis, Member: foreignExactRead},
			Key:      member.ProjectionRef{Axis: occurrenceAxis, Member: foreignExactKeyed},
			Read: program.ReadDecl{
				PointBound: program.PointBound,
				Input:      0, Axis: program.AxisRef(occurrenceAxis), Form: program.Exact,
				Contract: program.ReadContract{
					Order: program.OrderCanonical, Sparse: program.SparseExplicit,
					OnOpaque: program.OnOpaquePropagateAuthenticated, Multiplicity: program.MultiplicityOne,
				},
			},
		}},
		Fold: program.FoldDecl{
			Reducer: member.ReducerRef{Axis: consumerAxis, Member: cellReducer},
			Inputs:  []program.JoinRef{0},
			Outputs: []program.OutputDecl{{
				Column:      axis.OutputRef{Axis: consumerAxis, Key: planOutput},
				Destination: member.ProjectionRef{Axis: consumerAxis, Member: destinationProjection},
				Mode:        program.ModeExact,
				ValueSlot:   0,
			}},
		},
	}
	if problem, valid := fixture.declaration.Check(); !valid {
		t.Fatalf("foreign-candidate program rejected before sealing: %+v", problem)
	}
	return fixture
}

// TestExactWriteAdmitsTheConsumersOwnProjectionOfAForeignCandidate is G11: a
// rule keyed on another axis's sealed occurrence relation writes exactly one
// cell per candidate row through a projection its own axis declared for that
// candidate. That is a total function of the candidate directory, so it needs
// no selected join and no denominator to bound it.
func TestExactWriteAdmitsTheConsumersOwnProjectionOfAForeignCandidate(t *testing.T) {
	occurrenceAxis := schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: planOtherAxisKey}
	provider := member.RelationRef{Axis: occurrenceAxis, Member: heteroCandidateRelation}
	fixture := configureForeignCandidateFixture(t, provider, cellDestination)
	compiled, failure := Compile(fixture.seal(t))
	if failure.Available() {
		t.Fatalf("foreign-candidate exact write rejected: contributor=%d law=%d disposition=%s", failure.Contributor, failure.Law, failure.Disposition)
	}
	planned, ok := compiled.At(0)
	if !ok || !planned.Present() {
		t.Fatal("compiled plan missing")
	}
	if planned.OutputCount() != 1 {
		t.Fatalf("outputs = %d, want 1", planned.OutputCount())
	}
	output, outputOK := planned.OutputAt(0)
	if !outputOK || output.Mode != program.ModeExact {
		t.Fatalf("output mode = %v/%t, want exact", output.Mode, outputOK)
	}
	if output.RouteJoinPresent {
		t.Fatal("an exact write carried a route join")
	}
	for index := 0; index < planned.JoinCount(); index++ {
		join, joinOK := planned.JoinAt(index)
		if !joinOK {
			t.Fatalf("join %d missing", index)
		}
		if join.ReadForm == program.Selected {
			t.Fatal("the exact normal form required a selected join")
		}
		if join.Denominator.Present {
			t.Fatal("the exact normal form required a denominator to bound its write")
		}
	}
}

// TestExactWriteRefusesAProjectionOfAnotherRelation is the nearest negative:
// the widening admits the consumer's projection of THIS candidate, and nothing
// else. A projection the consumer declared for some other relation is not a
// function of the candidate row, so it is still malformed.
func TestExactWriteRefusesAProjectionOfAnotherRelation(t *testing.T) {
	occurrenceAxis := schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: planOtherAxisKey}
	provider := member.RelationRef{Axis: occurrenceAxis, Member: heteroCandidateRelation}
	fixture := configureForeignCandidateFixture(t, provider, strayDestination)
	_, failure := Compile(fixture.seal(t))
	if !failure.Available() {
		t.Fatal("a projection of an unrelated relation was admitted as an exact write")
	}
}

// TestExactWriteRefusesAConsumerRelationKeyedOnADifferentCandidate states the
// other half: the consumer's own relation must be declared for the candidate
// the program names, not for a different provider that happens to belong to
// the same foreign axis.
func TestExactWriteRefusesAConsumerRelationKeyedOnADifferentCandidate(t *testing.T) {
	occurrenceAxis := schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: planOtherAxisKey}
	mismatched := member.RelationRef{Axis: occurrenceAxis, Member: foreignExactRead}
	fixture := configureForeignCandidateFixture(t, mismatched, cellDestination)
	_, failure := Compile(fixture.seal(t))
	if !failure.Available() {
		t.Fatal("a consumer cell keyed on a different provider was admitted")
	}
}
