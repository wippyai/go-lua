package typing_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/relation/check/typing"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/region"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
)

// TestExpandRequiresOneSingletonReaderKey prevents mount from having to guess
// which physical coordinate layout a single semantic key column addresses.
// A composite reader key and two singleton keys are both ambiguous and must
// fail during schema checking.
func TestExpandRequiresOneSingletonReaderKey(t *testing.T) {
	value := newFixture(t)
	contract := model.DefineExpandContract(
		value.relationA,
		value.relationA,
		value.relationB,
		value.columnB,
		value.relationA,
	).WithScope(value.scope)
	expression := algebra.NewExpand(algebra.NewInput(value.relationA), contract)

	valid := buildExpandSchema(t, value, []model.KeySchema{
		model.DefineKeySchema(value.keyB, []model.ColumnID{value.columnB}),
	}, []model.KeyID{value.keyB}, nil, expression)
	if report := typing.Check(valid); !report.Valid() {
		t.Fatalf("singleton reader key rejected: %v", report.Error())
	}

	extraColumn := issueColumn(t, value.relationB, "expand-extra")
	composite := buildExpandSchema(t, value, []model.KeySchema{
		model.DefineKeySchema(value.keyB, []model.ColumnID{value.columnB, extraColumn}),
	}, []model.KeyID{value.keyB}, []model.ColumnID{extraColumn}, expression)
	if report := typing.Check(composite); report.Valid() || !hasIssue(report, typing.CodeKeyMismatch) {
		t.Fatalf("composite reader key was accepted: %v", report.Issues())
	}

	secondKey := issueKey(t, value.relationB, "expand-second-singleton")
	duplicate := buildExpandSchema(t, value, []model.KeySchema{
		model.DefineKeySchema(value.keyB, []model.ColumnID{value.columnB}),
		model.DefineKeySchema(secondKey, []model.ColumnID{value.columnB}),
	}, []model.KeyID{value.keyB, secondKey}, nil, expression)
	if report := typing.Check(duplicate); report.Valid() || !hasIssue(report, typing.CodeKeyMismatch) {
		t.Fatalf("duplicate singleton reader keys were accepted: %v", report.Issues())
	}
}

func buildExpandSchema(t *testing.T, value fixture, keySchemas []model.KeySchema, relationBKeys []model.KeyID, extraColumns []model.ColumnID, expression algebra.Expression) plan.ExecutionSchema {
	t.Helper()
	scope := model.DefineScopeSchema(value.scope, nil, region.True())
	relationA := model.DefineRelationSchema(value.relationA, []model.ColumnID{value.columnA}, []model.KeyID{value.keyA}, value.scope)
	relationBColumns := append([]model.ColumnID{value.columnB}, extraColumns...)
	relationB := model.DefineRelationSchema(value.relationB, relationBColumns, relationBKeys, value.scope)
	builder := plan.NewBuilder(value.schema)
	for _, relation := range []model.RelationSchema{relationA, relationB} {
		if !builder.AddRelation(relation) {
			t.Fatal("add relation")
		}
	}
	for _, column := range []model.ColumnSchema{
		model.DefineColumnSchema(value.columnA, value.typeID),
		model.DefineColumnSchema(value.columnB, value.typeID),
	} {
		if !builder.AddColumn(column) {
			t.Fatal("add column")
		}
	}
	for _, columnID := range extraColumns {
		if !builder.AddColumn(model.DefineColumnSchema(columnID, value.typeID)) {
			t.Fatal("add extra column")
		}
	}
	if !builder.AddKey(model.DefineKeySchema(value.keyA, []model.ColumnID{value.columnA})) {
		t.Fatal("add candidate key")
	}
	for _, key := range keySchemas {
		if !builder.AddKey(key) {
			t.Fatal("add reader key")
		}
	}
	if !builder.AddScope(scope) {
		t.Fatal("add scope")
	}
	equatable, ok := model.NewEquatableCapability(value.typeID)
	if !ok || !builder.AddTypeCapability(equatable) {
		t.Fatal("add key capability")
	}
	expressionID := issueExpression(t, value.owner, "expand-key-shape")
	if !builder.AddExpression(plan.DefineExpressionRef(expressionID, expression)) {
		t.Fatal("add expression")
	}
	schema, ok := builder.Build()
	if !ok {
		t.Fatal("build schema")
	}
	return schema
}
