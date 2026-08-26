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

type inputProjectionFixture struct {
	owner       model.OwnerID
	schema      model.SchemaID
	typeID      model.TypeID
	relation    model.RelationID
	first       model.ColumnID
	second      model.ColumnID
	key         model.KeyID
	scope       model.ScopeID
	denominator model.DenominatorRef
	operation   signature.Identity
}

func newInputProjectionFixture(t *testing.T) inputProjectionFixture {
	t.Helper()
	owner := issueOwner(t, "input-projection-owner")
	schema := issueSchema(t, owner, "input-projection-schema")
	typeID := issueType(t, owner, "input-projection-type")
	relation := issueRelation(t, owner, "input-projection-relation")
	first := issueColumn(t, relation, "input-projection-first")
	second := issueColumn(t, relation, "input-projection-second")
	key := issueKey(t, relation, "input-projection-key")
	scope := issueScope(t, owner, "input-projection-scope")
	denominator, ok := model.NewDenominatorRef(relation, key)
	if !ok {
		t.Fatal("denominator construction failed")
	}
	operation := signature.Identity{Operation: issueOperation(t, owner, "input-projection-operation"), Version: 1}
	return inputProjectionFixture{owner: owner, schema: schema, typeID: typeID, relation: relation, first: first, second: second, key: key, scope: scope, denominator: denominator, operation: operation}
}

func (value inputProjectionFixture) schemaFor(t *testing.T, input algebra.Input, inputs []signature.Input, slots []algebra.SlotSource) plan.ExecutionSchema {
	t.Helper()
	delivery, ok := signature.NewScalarDelivery()
	if !ok {
		t.Fatal("scalar delivery construction failed")
	}
	accepted, ok := outcome.NewSet(outcome.Produced, outcome.NoCandidate, outcome.NoSelection, outcome.Refused)
	if !ok {
		t.Fatal("outcome construction failed")
	}
	outputs := []signature.Output{{Relation: value.relation, Column: value.first, Type: value.typeID, Presence: signature.ProducePresent, Denominator: value.denominator}}
	for index := range inputs {
		inputs[index].Delivery = delivery
		inputs[index].Denominator = value.denominator
	}
	signatureValue, ok := signature.Seal(signature.Spec{
		Identity: value.operation,
		Fence:    signature.Fence{Owner: value.owner, Schema: value.schema},
		Inputs:   inputs, Outputs: outputs,
		Cardinality: mustCardinality(t, model.ExactlyOne), Outcomes: accepted,
	})
	if !ok || !signatureValue.Available() {
		t.Fatal("signature construction failed")
	}

	builder := plan.NewBuilder(value.schema)
	declarations := []model.RelationSchema{
		model.DefineRelationSchema(value.relation, []model.ColumnID{value.first, value.second}, []model.KeyID{value.key}, value.scope),
	}
	for _, declaration := range declarations {
		if !builder.AddRelation(declaration) {
			t.Fatal("add relation")
		}
	}
	for _, declaration := range []model.ColumnSchema{
		model.DefineColumnSchema(value.first, value.typeID),
		model.DefineColumnSchema(value.second, value.typeID),
	} {
		if !builder.AddColumn(declaration) {
			t.Fatal("add column")
		}
	}
	if !builder.AddKey(model.DefineKeySchema(value.key, []model.ColumnID{value.first})) ||
		!builder.AddScope(model.DefineScopeSchema(value.scope, nil, region.True())) {
		t.Fatal("add key/scope")
	}
	capability, ok := model.NewAscendingCapability(value.typeID)
	if !ok || !builder.AddTypeCapability(capability) {
		t.Fatal("add type capability")
	}
	apply := algebra.NewApply([]algebra.Expression{input}, algebra.NewApplyContract(value.operation, slots, algebra.OwnerNamed()))
	if !builder.AddExpression(plan.DefineExpressionRef(issueExpression(t, value.owner, "input-projection-expression"), apply)) || !builder.AddSignature(signatureValue) {
		t.Fatal("add expression/signature")
	}
	schema, ok := builder.Build()
	if !ok {
		t.Fatal("build schema")
	}
	return schema
}

func TestInputProjectionPreservesExactPositionalShape(t *testing.T) {
	value := newInputProjectionFixture(t)
	exact, ok := algebra.NewInputColumns(value.relation, []model.ColumnID{value.second})
	if !ok {
		t.Fatal("exact Input construction failed")
	}
	input := signature.Input{Relation: value.relation, Column: value.second, Type: value.typeID, Presence: signature.RequirePresent}
	exactSchema := value.schemaFor(t, exact, []signature.Input{input}, []algebra.SlotSource{algebra.NewSlotSource(0, 0)})
	if report := typing.Check(exactSchema); !report.Valid() {
		t.Fatalf("exact Input with positional cell zero was rejected: %v", report.Error())
	}

	all := algebra.NewInput(value.relation)
	allSchema := value.schemaFor(t, all, []signature.Input{input}, []algebra.SlotSource{algebra.NewSlotSource(0, 0)})
	report := typing.Check(allSchema)
	if report.Valid() || !hasIssue(report, typing.CodeMembership) {
		t.Fatalf("AllColumns silently widened a positional exact slot: %v", report.Issues())
	}
}

func TestInputProjectionOrderIsRetainedForDownstreamSlots(t *testing.T) {
	value := newInputProjectionFixture(t)
	exact, ok := algebra.NewInputColumns(value.relation, []model.ColumnID{value.second, value.first})
	if !ok {
		t.Fatal("exact Input construction failed")
	}
	inputs := []signature.Input{
		{Relation: value.relation, Column: value.second, Type: value.typeID, Presence: signature.RequirePresent},
		{Relation: value.relation, Column: value.first, Type: value.typeID, Presence: signature.RequirePresent},
	}
	schema := value.schemaFor(t, exact, inputs, []algebra.SlotSource{
		algebra.NewSlotSource(0, 0),
		algebra.NewSlotSource(0, 1),
	})
	if report := typing.Check(schema); !report.Valid() {
		t.Fatalf("reordered exact Input was not retained positionally: %v", report.Error())
	}
}
