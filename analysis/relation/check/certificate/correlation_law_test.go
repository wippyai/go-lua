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

func TestCorrelationPartitionsRetainIndependentChildAuthorities(t *testing.T) {
	value := newFixture(t)
	population, ok := model.NewDenominatorRef(value.relation, value.key)
	if !ok {
		t.Fatal("population")
	}
	operationID, ok := model.IssueOperationID(value.owner, token(t, "correlation-partition-operation"))
	if !ok {
		t.Fatal("operation")
	}
	delivery, ok := signature.NewCompleteSpanDelivery(value.key)
	if !ok {
		t.Fatal("complete delivery")
	}
	cardinality, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("cardinality")
	}
	outcomes, ok := outcome.NewSet(outcome.Produced, outcome.NoSelection, outcome.Refused)
	if !ok {
		t.Fatal("outcomes")
	}
	semantic, ok := signature.Seal(signature.Spec{
		Identity: signature.Identity{Operation: operationID, Version: 1},
		Fence:    signature.Fence{Owner: value.owner, Schema: value.schemaID},
		Inputs: []signature.Input{
			{Relation: value.relation, Column: value.column, Type: value.typeID, Presence: signature.RequirePresent, Delivery: delivery, Denominator: population},
			{Relation: value.relation, Column: value.column, Type: value.typeID, Presence: signature.RequirePresent, Delivery: delivery, Denominator: population},
		},
		Outputs:     []signature.Output{{Relation: value.relation, Column: value.column, Type: value.typeID, Presence: signature.ProducePresent, Denominator: population}},
		Cardinality: cardinality,
		Outcomes:    outcomes,
	})
	if !ok {
		t.Fatal("signature")
	}
	expressionID := issueExpression(t, value.owner, "correlation-partition-expression")
	dependencyID, ok := model.IssueDependencyID(value.owner, token(t, "correlation-partition-dependency"))
	if !ok {
		t.Fatal("dependency")
	}
	input := algebra.NewInput(value.relation)
	selectExpression := algebra.NewSelect(input, algebra.NewSelectContract(algebra.SelectByScope, value.scope))
	complete := algebra.NewComplete(selectExpression, population)
	groupCardinality, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("group cardinality")
	}
	group := algebra.NewGroup(input, algebra.NewGroupContract(value.key, groupCardinality))
	completeGroup := algebra.NewComplete(group, population)
	correlation := algebra.NewApplyCorrelation(population, value.column, value.typeID, [][]model.ColumnID{{value.column}, {value.column}})
	contract, ok := algebra.NewCorrelatedApplyContract(semantic.Identity(), []algebra.SlotSource{
		algebra.NewSlotSource(0, 0), algebra.NewSlotSource(1, 0),
	}, correlation, algebra.OwnerNamed())
	if !ok {
		t.Fatal("correlated contract")
	}
	expression := algebra.NewApply([]algebra.Expression{complete, completeGroup}, contract)
	dependencyRef, ok := plan.NewRelationRef(value.relation)
	if !ok {
		t.Fatal("relation ref")
	}
	sccRef := plan.DefineDependencyRef(dependencyID)
	schema := buildSchema(t, value, false, func(builder *plan.Builder) {
		capability, capabilityOK := model.NewEquatableCapability(value.typeID)
		if !capabilityOK || !builder.AddTypeCapability(capability) {
			t.Fatal("type capability")
		}
		builder.AddSignature(semantic)
		builder.AddExpression(plan.DefineExpressionRef(expressionID, expression))
		builder.AddDependency(plan.DefineDependency(dependencyID, expressionID, []plan.RelationRef{dependencyRef}, nil, "correlation-partition"))
		builder.AddSCC(plan.DefineSCC([]plan.DependencyRef{sccRef}, nil, plan.DefineRecurrence(plan.Acyclic, nil)))
	})
	cert, refusal := certificate.Check(schema)
	if refusal != nil || !cert.Available() {
		t.Fatalf("correlated certificate refused: %v", refusal)
	}
	partitions := cert.CorrelationPartitions()
	if len(partitions) != 2 {
		t.Fatalf("correlation partitions = %d, want 2: %#v", len(partitions), partitions)
	}
	for ordinal, partition := range partitions {
		if !partition.Available() || partition.Apply() != expression.Digest() || partition.Ordinal() != uint32(ordinal) || partition.Population() != population || partition.Child() != population || partition.Projection() != value.column {
			t.Fatalf("partition %d lost sealed authority: %#v", ordinal, partition)
		}
	}
}
