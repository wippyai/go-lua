package typing_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/check/typing"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/region"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

// TestJoinKeepsRepeatedReadFamiliesThroughApply proves the checker-side
// shape rule for the arithmetic/equality/order form: one relation may be read
// at two distinct coordinates. The nominal ColumnID is the same on both
// reads, but each Join occurrence remains a distinct family until Apply
// resolves the operation's typed inputs.
func TestJoinKeepsRepeatedReadFamiliesThroughApply(t *testing.T) {
	value := newFixture(t)
	input := algebra.NewInput(value.relationA)
	contract := algebra.NewJoinContract([]model.ColumnID{value.columnA}, []model.ColumnID{value.columnA})
	first := algebra.NewJoin(input, input, contract)
	second := algebra.NewJoin(first, input, contract)
	apply := algebra.NewApply([]algebra.Expression{second}, algebra.NewApplyContract(value.operation, []algebra.SlotSource{algebra.NewSlotSource(0, 0)}, algebra.OwnerNamed()))

	report := typing.Check(validSchema(t, value, apply))
	if !report.Valid() {
		t.Fatalf("repeated relation reads were rejected: %v", report.Error())
	}
}

// TestApplySlotSourcePreservesSharedAndIndependentReads proves the sealed
// child-coordinate contract at the checker boundary. A shared child supplies
// two slots from one row ([0, 0]); two independently authored self-reads keep
// two child ordinals ([0, 1]). Both use the same nominal relation and columns,
// so accepting either shape by RelationID inference would be unsound.
func TestApplySlotSourcePreservesSharedAndIndependentReads(t *testing.T) {
	owner := issueOwner(t, "slot-source-owner")
	schemaID := issueSchema(t, owner, "slot-source-schema")
	typeID := issueType(t, owner, "slot-source-type")
	relation := issueRelation(t, owner, "slot-source-relation")
	left := issueColumn(t, relation, "slot-source-left")
	right := issueColumn(t, relation, "slot-source-right")
	key := issueKey(t, relation, "slot-source-key")
	output := issueRelation(t, owner, "slot-source-output")
	outputColumn := issueColumn(t, output, "slot-source-output-column")
	outputKey := issueKey(t, output, "slot-source-output-key")
	scope := issueScope(t, owner, "slot-source-scope")
	denominator, ok := model.NewDenominatorRef(relation, key)
	if !ok {
		t.Fatal("slot-source denominator")
	}
	delivery, ok := signature.NewScalarDelivery()
	if !ok {
		t.Fatal("slot-source delivery")
	}
	operationID := issueOperation(t, owner, "slot-source-operation")
	accepted, ok := outcome.NewSet(outcome.Produced, outcome.NoSelection, outcome.Refused)
	if !ok {
		t.Fatal("slot-source outcomes")
	}
	operation, ok := signature.Seal(signature.Spec{
		Identity: signature.Identity{Operation: operationID, Version: 1},
		Fence:    signature.Fence{Owner: owner, Schema: schemaID},
		Inputs: []signature.Input{
			{Relation: relation, Column: left, Type: typeID, Presence: signature.RequirePresent, Delivery: delivery, Denominator: denominator},
			{Relation: relation, Column: right, Type: typeID, Presence: signature.RequirePresent, Delivery: delivery, Denominator: denominator},
		},
		Outputs:     []signature.Output{{Relation: output, Column: outputColumn, Type: typeID, Presence: signature.ProducePresent, Denominator: mustDenominatorFor(t, output, outputKey)}},
		Cardinality: mustCardinality(t, model.ExactlyOne),
		Outcomes:    accepted,
	})
	if !ok {
		t.Fatal("slot-source operation")
	}
	shared := algebra.NewApply([]algebra.Expression{algebra.NewInput(relation)}, algebra.NewApplyContract(operation.Identity(), []algebra.SlotSource{algebra.NewSlotSource(0, 0), algebra.NewSlotSource(0, 1)}, algebra.OwnerNamed()))
	independent := algebra.NewApply([]algebra.Expression{algebra.NewInput(relation), algebra.NewInput(relation)}, algebra.NewApplyContract(operation.Identity(), []algebra.SlotSource{algebra.NewSlotSource(0, 0), algebra.NewSlotSource(1, 1)}, algebra.OwnerNamed()))

	builder := plan.NewBuilder(schemaID)
	if !builder.AddRelation(model.DefineRelationSchema(relation, []model.ColumnID{left, right}, []model.KeyID{key}, scope)) ||
		!builder.AddRelation(model.DefineRelationSchema(output, []model.ColumnID{outputColumn}, []model.KeyID{outputKey}, scope)) ||
		!builder.AddColumn(model.DefineColumnSchema(left, typeID)) ||
		!builder.AddColumn(model.DefineColumnSchema(right, typeID)) ||
		!builder.AddColumn(model.DefineColumnSchema(outputColumn, typeID)) ||
		!builder.AddKey(model.DefineKeySchema(key, []model.ColumnID{left})) ||
		!builder.AddKey(model.DefineKeySchema(outputKey, []model.ColumnID{outputColumn})) ||
		!builder.AddScope(model.DefineScopeSchema(scope, []model.ColumnID{left, right}, region.True())) ||
		!builder.AddSignature(operation) {
		t.Fatal("slot-source declarations")
	}
	capability, capabilityOK := model.NewAscendingCapability(typeID)
	if !capabilityOK || !builder.AddTypeCapability(capability) {
		t.Fatal("slot-source capability")
	}
	for index, expression := range []algebra.Expression{shared, independent} {
		id, issueOK := model.IssueExpressionID(owner, slotSourceToken(t, index))
		if !issueOK || !builder.AddExpression(plan.DefineExpressionRef(id, expression)) {
			t.Fatalf("slot-source expression %d", index)
		}
	}
	declaration, ok := builder.Build()
	if !ok {
		t.Fatal("slot-source schema")
	}
	report := typing.Check(declaration)
	if !report.Valid() {
		t.Fatalf("shared/independent slot-source reads rejected: %v", report.Error())
	}
}

// TestApplySlotSourcePreservesRepeatedColumnOccurrences proves the case that
// a nominal-column map cannot represent: a self-join reads the very same
// column twice. The signature has two positional inputs with equal relation
// and column identities; the sealed cell ordinals select the left and right
// row occurrence without asking the checker to guess which one was meant.
func TestApplySlotSourcePreservesRepeatedColumnOccurrences(t *testing.T) {
	owner := issueOwner(t, "repeated-slot-owner")
	schemaID := issueSchema(t, owner, "repeated-slot-schema")
	typeID := issueType(t, owner, "repeated-slot-type")
	relation := issueRelation(t, owner, "repeated-slot-relation")
	column := issueColumn(t, relation, "repeated-slot-column")
	key := issueKey(t, relation, "repeated-slot-key")
	output := issueRelation(t, owner, "repeated-slot-output")
	outputColumn := issueColumn(t, output, "repeated-slot-output-column")
	outputKey := issueKey(t, output, "repeated-slot-output-key")
	scope := issueScope(t, owner, "repeated-slot-scope")
	denominator, ok := model.NewDenominatorRef(relation, key)
	if !ok {
		t.Fatal("repeated-slot denominator")
	}
	delivery, ok := signature.NewScalarDelivery()
	if !ok {
		t.Fatal("repeated-slot delivery")
	}
	accepted, ok := outcome.NewSet(outcome.Produced, outcome.NoSelection, outcome.Refused)
	if !ok {
		t.Fatal("repeated-slot outcomes")
	}
	operationID := issueOperation(t, owner, "repeated-slot-operation")
	operation, ok := signature.Seal(signature.Spec{
		Identity: signature.Identity{Operation: operationID, Version: 1},
		Fence:    signature.Fence{Owner: owner, Schema: schemaID},
		Inputs: []signature.Input{
			{Relation: relation, Column: column, Type: typeID, Presence: signature.RequirePresent, Delivery: delivery, Denominator: denominator},
			{Relation: relation, Column: column, Type: typeID, Presence: signature.RequirePresent, Delivery: delivery, Denominator: denominator},
		},
		Outputs:     []signature.Output{{Relation: output, Column: outputColumn, Type: typeID, Presence: signature.ProducePresent, Denominator: mustDenominatorFor(t, output, outputKey)}},
		Cardinality: mustCardinality(t, model.ExactlyOne),
		Outcomes:    accepted,
	})
	if !ok {
		t.Fatal("repeated-slot operation")
	}

	selfJoin := algebra.NewJoin(
		algebra.NewInput(relation),
		algebra.NewInput(relation),
		algebra.NewJoinContract([]model.ColumnID{column}, []model.ColumnID{column}),
	)
	apply := algebra.NewApply([]algebra.Expression{selfJoin}, algebra.NewApplyContract(operation.Identity(), []algebra.SlotSource{
		algebra.NewSlotSource(0, 0), // left occurrence
		algebra.NewSlotSource(0, 1), // right occurrence
	}, algebra.OwnerNamed()))

	builder := plan.NewBuilder(schemaID)
	if !builder.AddRelation(model.DefineRelationSchema(relation, []model.ColumnID{column}, []model.KeyID{key}, scope)) ||
		!builder.AddRelation(model.DefineRelationSchema(output, []model.ColumnID{outputColumn}, []model.KeyID{outputKey}, scope)) ||
		!builder.AddColumn(model.DefineColumnSchema(column, typeID)) ||
		!builder.AddColumn(model.DefineColumnSchema(outputColumn, typeID)) ||
		!builder.AddKey(model.DefineKeySchema(key, []model.ColumnID{column})) ||
		!builder.AddKey(model.DefineKeySchema(outputKey, []model.ColumnID{outputColumn})) ||
		!builder.AddScope(model.DefineScopeSchema(scope, []model.ColumnID{column}, region.True())) ||
		!builder.AddSignature(operation) {
		t.Fatal("repeated-slot declarations")
	}
	capability, capabilityOK := model.NewAscendingCapability(typeID)
	if !capabilityOK || !builder.AddTypeCapability(capability) {
		t.Fatal("repeated-slot capability")
	}
	expression, issueOK := model.IssueExpressionID(owner, slotSourceToken(t, 23))
	if !issueOK || !builder.AddExpression(plan.DefineExpressionRef(expression, apply)) {
		t.Fatal("repeated-slot expression")
	}
	declaration, ok := builder.Build()
	if !ok {
		t.Fatal("repeated-slot schema")
	}
	if report := typing.Check(declaration); !report.Valid() {
		t.Fatalf("explicit repeated-column source cells rejected: %v", report.Error())
	}
}

func mustDenominatorFor(t *testing.T, relation model.RelationID, key model.KeyID) model.DenominatorRef {
	t.Helper()
	value, ok := model.NewDenominatorRef(relation, key)
	if !ok {
		t.Fatal("slot-source output denominator")
	}
	return value
}

func slotSourceToken(t *testing.T, index int) identity.ContentID {
	t.Helper()
	value, ok := identity.DeriveContentID("relation/check/typing/slot-source-expression", []byte{byte(index)})
	if !ok {
		t.Fatal("slot-source expression identity")
	}
	return value
}
