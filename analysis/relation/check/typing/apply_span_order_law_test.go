package typing_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/relation/check/typing"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/region"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

// spanOrderSchema adds a second declared key to relation A. The denominator
// remains key A while the alternate key proves that span order is an
// independent declared coordinate—not an inference from the denominator.
func spanOrderSchema(t *testing.T, value fixture, alternate model.KeyID, alternateColumn model.ColumnID, delivery signature.Delivery, expression algebra.Expression) plan.ExecutionSchema {
	t.Helper()
	builder := plan.NewBuilder(value.schema)
	if !builder.AddRelation(model.DefineRelationSchema(value.relationA, []model.ColumnID{value.columnA, alternateColumn}, []model.KeyID{value.keyA, alternate}, value.scope)) ||
		!builder.AddRelation(model.DefineRelationSchema(value.relationB, []model.ColumnID{value.columnB}, []model.KeyID{value.keyB}, value.scope)) ||
		!builder.AddColumn(model.DefineColumnSchema(value.columnA, value.typeID)) ||
		!builder.AddColumn(model.DefineColumnSchema(alternateColumn, value.typeID)) ||
		!builder.AddColumn(model.DefineColumnSchema(value.columnB, value.typeID)) ||
		!builder.AddKey(model.DefineKeySchema(value.keyA, []model.ColumnID{value.columnA})) ||
		!builder.AddKey(model.DefineKeySchema(alternate, []model.ColumnID{alternateColumn})) ||
		!builder.AddKey(model.DefineKeySchema(value.keyB, []model.ColumnID{value.columnB})) ||
		!builder.AddScope(model.DefineScopeSchema(value.scope, []model.ColumnID{value.columnA, alternateColumn, value.columnB}, region.True())) ||
		!builder.AddExpression(plan.DefineExpressionRef(issueExpression(t, value.owner, "span-order"), expression)) {
		t.Fatal("span-order declarations")
	}
	capability, capabilityOK := model.NewAscendingCapability(value.typeID)
	if !capabilityOK || !builder.AddTypeCapability(capability) {
		t.Fatal("span-order capability")
	}
	accepted, acceptedOK := outcome.NewSet(outcome.Produced, outcome.NoCandidate, outcome.NoSelection, outcome.Refused)
	if !acceptedOK {
		t.Fatal("span-order outcomes")
	}
	operation, operationOK := signature.Seal(signature.Spec{
		Identity: value.operation,
		Fence:    signature.Fence{Owner: value.owner, Schema: value.schema},
		Inputs: []signature.Input{{
			Relation: value.relationA, Column: value.columnA, Type: value.typeID,
			Presence: signature.RequirePresent, Delivery: delivery, Denominator: value.denominatorA,
		}},
		Outputs:     []signature.Output{{Relation: value.relationB, Column: value.columnB, Type: value.typeID, Presence: signature.ProducePresent, Denominator: value.denominatorB}},
		Cardinality: mustCardinality(t, model.ExactlyOne),
		Outcomes:    accepted,
	})
	if !operationOK || !builder.AddSignature(operation) {
		t.Fatal("span-order signature")
	}
	schema, schemaOK := builder.Build()
	if !schemaOK {
		t.Fatal("span-order schema")
	}
	return schema
}

func alternateOrder(t *testing.T, value fixture) (model.KeyID, model.ColumnID) {
	t.Helper()
	column := issueColumn(t, value.relationA, "span-order")
	key := issueKey(t, value.relationA, "span-order")
	return key, column
}

func TestApplyBoundedGroupRedeemsDeclaredOrderNotDenominatorKey(t *testing.T) {
	value := newFixture(t)
	orderKey, orderColumn := alternateOrder(t, value)
	delivery, deliveryOK := signature.NewBoundedSpanDelivery(1, orderKey)
	if !deliveryOK {
		t.Fatal("bounded delivery")
	}
	group := algebra.NewGroup(algebra.NewInput(value.relationA), algebra.NewGroupContract(orderKey, mustCardinality(t, model.ExactlyOne)))
	apply := algebra.NewApply([]algebra.Expression{group}, algebra.NewApplyContract(value.operation, []algebra.SlotSource{algebra.NewSlotSource(0, 0)}, algebra.OwnerNamed()))
	if report := typing.Check(spanOrderSchema(t, value, orderKey, orderColumn, delivery, apply)); !report.Valid() {
		t.Fatalf("bounded Group ordered by non-denominator key rejected: %v", report.Error())
	}
}

func TestApplyCompleteSpanRejectsOrderOtherThanItsDenominatorKey(t *testing.T) {
	value := newFixture(t)
	orderKey, orderColumn := alternateOrder(t, value)
	delivery, deliveryOK := signature.NewCompleteSpanDelivery(orderKey)
	if !deliveryOK {
		t.Fatal("complete delivery")
	}
	complete := algebra.NewComplete(algebra.NewInput(value.relationA), value.denominatorA)
	apply := algebra.NewApply([]algebra.Expression{complete}, algebra.NewApplyContract(value.operation, []algebra.SlotSource{algebra.NewSlotSource(0, 0)}, algebra.OwnerNamed()))
	report := typing.Check(spanOrderSchema(t, value, orderKey, orderColumn, delivery, apply))
	if report.Valid() || !hasIssue(report, typing.CodeDeliveryMismatch) {
		t.Fatalf("CompleteSpan order mismatch accepted: %v", report.Issues())
	}
}
