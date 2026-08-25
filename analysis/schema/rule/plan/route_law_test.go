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
	heteroCandidateRelation schema.Key     = "relation/hetero-candidate"
	heteroExactRelation     schema.Key     = "relation/hetero-value-exact"
	heteroRouteRelation     schema.Key     = "relation/hetero-placement-route"
	heteroSecondRelation    schema.Key     = "relation/hetero-placement-second"
	heteroExactKey          schema.Key     = "projection/hetero-value-key"
	heteroRouteKey          schema.Key     = "projection/hetero-route-key"
	heteroRoutePredicate    schema.Key     = "projection/hetero-route-predicate"
	heteroRouteSelection    schema.Key     = "selection/hetero-route"
	heteroSecondKey         schema.Key     = "projection/hetero-second-key"
	heteroSecondPredicate   schema.Key     = "projection/hetero-second-predicate"
	heteroSecondSelection   schema.Key     = "selection/hetero-second"
	heteroRouteDestination  schema.Key     = "projection/hetero-route-destination"
	heteroReducer           schema.Key     = "reducer/hetero-route"
	heteroCandidateCarrier  member.Carrier = "carrier/hetero-candidate"
	heteroFactCarrier       member.Carrier = "carrier/hetero-fact"
	heteroKeyCarrier        member.Carrier = "carrier/hetero-key"
	heteroTagCarrier        member.Carrier = "carrier/hetero-tag"
)

// configureHeterogeneousRouteFixture is the hard Store-shaped case. The
// Value-owned candidate and exact join feed two Placement-owned selected
// joins. Each selected relation is dependent on both the candidate and the
// earlier exact fact; the output explicitly routes through the first selected
// join even though another selected join is also a Fold input.
func configureHeterogeneousRouteFixture(t *testing.T) *planFixture {
	t.Helper()
	fixture := newPlanFixture(t)
	valueAxis := schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: planOtherAxisKey}
	placementAxis := schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: planAxisKey}

	valueCatalog, ok := member.NewCatalog(
		[]member.Relation{
			{
				Key:               heteroCandidateRelation,
				Subject:           heteroCandidateCarrier,
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: valueAxis, Member: heteroCandidateRelation}),
			},
			{
				Key:               heteroExactRelation,
				Subject:           heteroFactCarrier,
				Inputs:            []member.Carrier{heteroCandidateCarrier},
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: valueAxis, Member: heteroCandidateRelation}),
			},
		},
		[]member.Projection{{
			Key:               heteroExactKey,
			Relation:          heteroExactRelation,
			Role:              member.Key,
			Result:            heteroKeyCarrier,
			CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: valueAxis, Member: heteroCandidateRelation}),
		}},
		nil, nil,
	)
	if !ok {
		t.Fatal("Value member catalog rejected")
	}
	placementCatalog, ok := member.NewCatalog(
		[]member.Relation{
			{
				Key:               heteroRouteRelation,
				Subject:           heteroFactCarrier,
				Inputs:            []member.Carrier{heteroCandidateCarrier, heteroFactCarrier},
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: valueAxis, Member: heteroCandidateRelation}),
			},
			{
				Key:               heteroSecondRelation,
				Subject:           heteroFactCarrier,
				Inputs:            []member.Carrier{heteroCandidateCarrier, heteroFactCarrier},
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: valueAxis, Member: heteroCandidateRelation}),
			},
		},
		[]member.Projection{
			{
				Key:               heteroRouteKey,
				Relation:          heteroRouteRelation,
				Role:              member.Key,
				Result:            heteroKeyCarrier,
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: valueAxis, Member: heteroCandidateRelation}),
			},
			{
				Key:               heteroRoutePredicate,
				Relation:          heteroRouteRelation,
				Role:              member.Predicate,
				Result:            heteroTagCarrier,
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: valueAxis, Member: heteroCandidateRelation}),
			},
			{
				Key:               heteroSecondKey,
				Relation:          heteroSecondRelation,
				Role:              member.Key,
				Result:            heteroKeyCarrier,
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: valueAxis, Member: heteroCandidateRelation}),
			},
			{
				Key:               heteroSecondPredicate,
				Relation:          heteroSecondRelation,
				Role:              member.Predicate,
				Result:            heteroTagCarrier,
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: valueAxis, Member: heteroCandidateRelation}),
			},
			{
				Key:               heteroRouteDestination,
				Relation:          heteroRouteRelation,
				Role:              member.Destination,
				Result:            heteroKeyCarrier,
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: valueAxis, Member: heteroCandidateRelation}),
			},
		},
		[]member.Reducer{{
			Key: heteroReducer,
			Inputs: []member.ReducerInput{
				{Axis: valueAxis, Carrier: heteroFactCarrier, Form: member.ReadFormExact, Multiplicity: member.MultiplicityOne},
				{Axis: placementAxis, Carrier: heteroFactCarrier, Form: member.ReadFormSelected, Multiplicity: member.MultiplicityOne, Tag: heteroTagCarrier},
				{Axis: placementAxis, Carrier: heteroFactCarrier, Form: member.ReadFormSelected, Multiplicity: member.MultiplicityOne, Tag: heteroTagCarrier},
			},
			Outputs: []member.ReducerOutput{{Axis: placementAxis, Carrier: heteroFactCarrier}},
		}},
		nil,
	)
	if !ok {
		t.Fatal("Placement member catalog rejected")
	}

	fixture.catalog = placementCatalog
	fixture.otherCatalog = valueCatalog
	fixture.mainSignature = axis.Signature{Key: heteroKeyCarrier, Fact: heteroFactCarrier}
	fixture.otherSignature = axis.Signature{Key: heteroKeyCarrier, Fact: heteroFactCarrier}
	fixture.declaration = program.Program{
		OperandRole: vocabulary.RoleKey("plan/operand"),
		Candidate:   member.AxisRelationCandidate(member.RelationRef{Axis: valueAxis, Member: heteroCandidateRelation}),
		Joins: []program.JoinDecl{
			{
				Sources:  []program.SourceRef{program.CandidateSource()},
				Relation: member.RelationRef{Axis: valueAxis, Member: heteroExactRelation},
				Key:      member.ProjectionRef{Axis: valueAxis, Member: heteroExactKey},
				Read: program.ReadDecl{
					PointBound: program.PointBound,
					Input:      0, Axis: program.AxisRef(valueAxis), Form: program.Exact,
					Contract: program.ReadContract{Order: program.OrderCanonical, Sparse: program.SparseExplicit, OnOpaque: program.OnOpaqueRefuse, Multiplicity: program.MultiplicityOne, DenominatorRef: program.DenominatorRef{Surface: schema.SurfaceKindDenominator, Key: planOtherDenominator}},
				},
			},
			{
				Sources:   []program.SourceRef{program.CandidateSource(), program.PriorSource(0)},
				Relation:  member.RelationRef{Axis: placementAxis, Member: heteroRouteRelation},
				Key:       member.ProjectionRef{Axis: placementAxis, Member: heteroRouteKey},
				Predicate: member.ProjectionRef{Axis: placementAxis, Member: heteroRoutePredicate},
				Selection: member.SelectionRef{Axis: placementAxis, Member: heteroRouteSelection},
				Read: program.ReadDecl{
					PointBound: program.PointBound,
					Input:      1, Axis: program.AxisRef(placementAxis), Form: program.Selected,
					Contract: program.ReadContract{Order: program.OrderCanonical, Sparse: program.SparseExplicit, OnOpaque: program.OnOpaqueRefuse, Multiplicity: program.MultiplicityOne, DenominatorRef: program.DenominatorRef{Surface: schema.SurfaceKindDenominator, Key: planDenominator}},
				},
			},
			{
				Sources:   []program.SourceRef{program.CandidateSource(), program.PriorSource(0)},
				Relation:  member.RelationRef{Axis: placementAxis, Member: heteroSecondRelation},
				Key:       member.ProjectionRef{Axis: placementAxis, Member: heteroSecondKey},
				Predicate: member.ProjectionRef{Axis: placementAxis, Member: heteroSecondPredicate},
				Selection: member.SelectionRef{Axis: placementAxis, Member: heteroSecondSelection},
				Read: program.ReadDecl{
					PointBound: program.PointBound,
					Input:      2, Axis: program.AxisRef(placementAxis), Form: program.Selected,
					Contract: program.ReadContract{Order: program.OrderCanonical, Sparse: program.SparseExplicit, OnOpaque: program.OnOpaqueRefuse, Multiplicity: program.MultiplicityOne, DenominatorRef: program.DenominatorRef{Surface: schema.SurfaceKindDenominator, Key: planDenominator}},
				},
			},
		},
		Fold: program.FoldDecl{
			Reducer: member.ReducerRef{Axis: placementAxis, Member: heteroReducer},
			Inputs:  []program.JoinRef{0, 1, 2},
			Outputs: []program.OutputDecl{{
				Column:      axis.OutputRef{Axis: placementAxis, Key: planOutput},
				Destination: member.ProjectionRef{Axis: placementAxis, Member: heteroRouteDestination},
				Mode:        program.ModeRoute, ValueSlot: 0, RouteJoin: 1, RouteJoinPresent: true,
			}},
		},
	}
	fixture.outputWriter = planAxisKey
	if problem, valid := fixture.declaration.Check(); !valid {
		t.Fatalf("heterogeneous route declaration rejected: %+v", problem)
	}
	return fixture
}

func TestCompileLowersExplicitHeterogeneousRouteWithMultipleSelectedInputs(t *testing.T) {
	fixture := configureHeterogeneousRouteFixture(t)
	compiled, failure := Compile(fixture.seal(t))
	if failure.Available() || !compiled.Available() {
		t.Fatalf("heterogeneous route rejected: catalog=%#v failure=%+v", compiled, failure)
	}
	planned, ok := compiled.At(0)
	if !ok || !planned.Present() {
		t.Fatal("heterogeneous route plan missing")
	}
	if got, want := planned.Candidate().Axis, uint32(1); got != want {
		t.Fatalf("candidate Value owner axis=%d, want %d", got, want)
	}
	first, firstOK := planned.JoinAt(0)
	second, secondOK := planned.JoinAt(1)
	third, thirdOK := planned.JoinAt(2)
	if !firstOK || !secondOK || !thirdOK || first.Relation.Axis != 1 || second.Relation.Axis != 0 || third.Relation.Axis != 0 || second.ReadForm != program.Selected || third.ReadForm != program.Selected {
		t.Fatalf("heterogeneous join ownership/forms: first=%#v/%t second=%#v/%t third=%#v/%t", first, firstOK, second, secondOK, third, thirdOK)
	}
	output, outputOK := planned.OutputAt(0)
	if !outputOK || output.Mode != program.ModeRoute || !output.RouteJoinPresent || output.RouteJoin != 1 || output.Destination.Axis != 0 {
		t.Fatalf("explicit bounded route output=%#v/%t", output, outputOK)
	}
}

func TestCompileRejectsRouteJoinNotFoldedOrUnbounded(t *testing.T) {
	fixture := configureHeterogeneousRouteFixture(t)
	fixture.declaration.Fold.Inputs = []program.JoinRef{0, 2}
	if problem, valid := fixture.declaration.Check(); valid || problem.Kind != program.ProblemOutput {
		t.Fatalf("non-folded route valid=%v problem=%+v", valid, problem)
	}

	fixture = configureHeterogeneousRouteFixture(t)
	fixture.declaration.Joins[1].Read.Contract.Multiplicity = program.MultiplicityMany
	fixture.catalog.Reducers[0].Inputs[1].Multiplicity = member.MultiplicityMany
	if problem, valid := fixture.declaration.Check(); valid || problem.Kind != program.ProblemJoin {
		t.Fatalf("unbounded route valid=%v problem=%+v", valid, problem)
	}
}

func TestCompileRejectsRouteDependencySourceDrift(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*planFixture)
	}{
		{
			name: "dropped-candidate-source",
			mutate: func(fixture *planFixture) {
				fixture.declaration.Joins[1].Sources = []program.SourceRef{program.PriorSource(0)}
			},
		},
		{
			name: "swapped-source-order",
			mutate: func(fixture *planFixture) {
				fixture.declaration.Joins[1].Sources = []program.SourceRef{program.PriorSource(0), program.CandidateSource()}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := configureHeterogeneousRouteFixture(t)
			test.mutate(fixture)
			compiled, failure := Compile(fixture.seal(t))
			if !failure.Available() || compiled.Available() {
				t.Fatalf("source drift admitted: catalog=%#v failure=%+v", compiled, failure)
			}
		})
	}
}
