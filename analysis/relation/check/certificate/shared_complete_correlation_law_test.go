package certificate_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/relation/check/certificate"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

// A shared Complete child is backed by its global Complete denominator, not a
// per-Q posting. The certificate must retain that denominator through its
// ordinary Complete projection while emitting no CorrelationPartition for the
// empty projection.
func TestSharedCompleteCorrelationOmitsPartitionAuthority(t *testing.T) {
	value := newFixture(t)
	population, ok := model.NewDenominatorRef(value.relation, value.key)
	if !ok {
		t.Fatal("population")
	}
	globalRelation := issueRelation(t, value.owner, "shared-complete-global")
	globalColumn, ok := model.IssueColumnID(globalRelation, token(t, "shared-complete-global-column"))
	if !ok {
		t.Fatal("global column")
	}
	globalKey, ok := model.IssueKeyID(globalRelation, token(t, "shared-complete-global-key"))
	if !ok {
		t.Fatal("global key")
	}
	global, ok := model.NewDenominatorRef(globalRelation, globalKey)
	if !ok {
		t.Fatal("global denominator")
	}
	operationID, ok := model.IssueOperationID(value.owner, token(t, "shared-complete-operation"))
	if !ok {
		t.Fatal("operation")
	}
	scalar, ok := signature.NewScalarDelivery()
	if !ok {
		t.Fatal("scalar delivery")
	}
	complete, ok := signature.NewCompleteSpanDelivery(globalKey)
	if !ok {
		t.Fatal("complete delivery")
	}
	cardinality, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("cardinality")
	}
	accepted, ok := outcome.NewSet(outcome.Produced, outcome.NoCandidate, outcome.NoSelection, outcome.Refused)
	if !ok {
		t.Fatal("outcomes")
	}
	semantic, ok := signature.Seal(signature.Spec{
		Identity: signature.Identity{Operation: operationID, Version: 1},
		Fence:    signature.Fence{Owner: value.owner, Schema: value.schemaID},
		Inputs: []signature.Input{
			{Relation: value.relation, Column: value.column, Type: value.typeID, Presence: signature.RequirePresent, Delivery: scalar, Denominator: population},
			{Relation: globalRelation, Column: globalColumn, Type: value.typeID, Presence: signature.RequirePresent, Delivery: complete, Denominator: global},
		},
		Outputs:     []signature.Output{{Relation: value.relation, Column: value.column, Type: value.typeID, Presence: signature.ProducePresent, Denominator: population}},
		Cardinality: cardinality,
		Outcomes:    accepted,
	})
	if !ok {
		t.Fatal("signature")
	}
	shared := algebra.NewComplete(
		algebra.NewSelect(algebra.NewInput(globalRelation), algebra.NewSelectContract(algebra.SelectByScope, value.scope)),
		global,
	)
	correlation := algebra.NewApplyCorrelation(population, value.column, value.typeID, [][]model.ColumnID{{value.column}, {}})
	contract, ok := algebra.NewCorrelatedApplyContract(semantic.Identity(), []algebra.SlotSource{
		algebra.NewSlotSource(0, 0), algebra.NewSlotSource(1, 0),
	}, correlation, algebra.OwnerNamed())
	if !ok {
		t.Fatal("correlated contract")
	}
	expression := algebra.NewApply([]algebra.Expression{algebra.NewInput(value.relation), shared}, contract)
	expressionID := issueExpression(t, value.owner, "shared-complete-expression")
	dependencyID, ok := model.IssueDependencyID(value.owner, token(t, "shared-complete-dependency"))
	if !ok {
		t.Fatal("dependency")
	}
	populationRef, ok := plan.NewRelationRef(value.relation)
	if !ok {
		t.Fatal("population relation ref")
	}
	globalRef, ok := plan.NewRelationRef(globalRelation)
	if !ok {
		t.Fatal("global relation ref")
	}
	dependencyRef := plan.DefineDependencyRef(dependencyID)
	schema := buildSchema(t, value, false, func(builder *plan.Builder) {
		capability, capabilityOK := model.NewEquatableCapability(value.typeID)
		if !capabilityOK || !builder.AddTypeCapability(capability) ||
			!builder.AddRelation(model.DefineRelationSchema(globalRelation, []model.ColumnID{globalColumn}, []model.KeyID{globalKey}, value.scope)) ||
			!builder.AddColumn(model.DefineColumnSchema(globalColumn, value.typeID)) ||
			!builder.AddKey(model.DefineKeySchema(globalKey, []model.ColumnID{globalColumn})) ||
			!builder.AddSignature(semantic) ||
			!builder.AddExpression(plan.DefineExpressionRef(expressionID, expression)) ||
			!builder.AddDependency(plan.DefineDependency(dependencyID, expressionID, []plan.RelationRef{populationRef, globalRef}, nil, "shared-complete")) ||
			!builder.AddSCC(plan.DefineSCC([]plan.DependencyRef{dependencyRef}, nil, plan.DefineRecurrence(plan.Acyclic, nil))) {
			t.Fatal("shared correlation schema")
		}
	})
	cert, refusal := certificate.Check(schema)
	if refusal != nil || !cert.Available() {
		t.Fatalf("shared correlation certificate refused: %v", refusal)
	}
	if partitions := cert.CorrelationPartitions(); len(partitions) != 0 {
		t.Fatalf("shared Complete child received partition authorities: %#v", partitions)
	}
	if denominators := cert.CompleteDenominators(); len(denominators) != 1 || denominators[0] != global {
		t.Fatalf("shared global Complete denominator = %#v, want %v", denominators, global)
	}
	if populations := cert.CorrelationDenominators(); len(populations) != 1 || populations[0] != population {
		t.Fatalf("correlation populations = %#v, want %v", populations, population)
	}
}
