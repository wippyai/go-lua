package authority

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/region"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

func TestAuthorityAcceptsTwoChildScalarPopulationCorrelation(t *testing.T) {
	value := newScalarAuthorityIDs(t)
	good, ok := model.NewDenominatorRef(value.relation, value.key)
	if !ok {
		t.Fatal("population denominator")
	}
	span := algebra.NewComplete(
		algebra.NewSelect(algebra.NewInput(value.relation), algebra.NewSelectContract(algebra.SelectByScope, value.scope)),
		good,
	)
	correlation := algebra.NewApplyCorrelation(good, value.column, value.typeID, [][]model.ColumnID{{value.column}, {value.column}})
	contract, ok := algebra.NewCorrelatedApplyContract(value.operation, []algebra.SlotSource{
		algebra.NewSlotSource(0, 0), algebra.NewSlotSource(1, 0), algebra.NewSlotSource(1, 0),
	}, correlation, algebra.OwnerNamed())
	if !ok {
		t.Fatal("correlated contract")
	}
	apply := algebra.NewApply([]algebra.Expression{algebra.NewInput(value.relation), span}, contract)
	schema := scalarAuthoritySchema(t, value, apply, good)
	if report := Check(schema); !report.Valid() {
		t.Fatalf("authority rejected closed scalar-population form: %s", report.Error())
	}
}

func TestAuthorityRejectsNearestScalarPopulationCorrelationMutations(t *testing.T) {
	value := newScalarAuthorityIDs(t)
	good, ok := model.NewDenominatorRef(value.relation, value.key)
	if !ok {
		t.Fatal("population denominator")
	}
	wrongKey := issueKey(t, value.relation, "scalar-population-wrong-key")
	wrong, ok := model.NewDenominatorRef(value.relation, wrongKey)
	if !ok {
		t.Fatal("wrong denominator")
	}
	span := algebra.NewComplete(
		algebra.NewSelect(algebra.NewInput(value.relation), algebra.NewSelectContract(algebra.SelectByScope, value.scope)),
		good,
	)
	correlation := algebra.NewApplyCorrelation(good, value.column, value.typeID, [][]model.ColumnID{{value.column}, {value.column}})

	build := func(t *testing.T, slots []algebra.SlotSource, spanDenominator model.DenominatorRef) Report {
		t.Helper()
		contract, ok := algebra.NewCorrelatedApplyContract(value.operation, slots, correlation, algebra.OwnerNamed())
		if !ok {
			t.Fatal("correlated contract")
		}
		apply := algebra.NewApply([]algebra.Expression{algebra.NewInput(value.relation), span}, contract)
		return Check(scalarAuthoritySchema(t, value, apply, spanDenominator))
	}

	t.Run("span slot addressed to scalar child", func(t *testing.T) {
		report := build(t, []algebra.SlotSource{
			algebra.NewSlotSource(0, 0), algebra.NewSlotSource(1, 0), algebra.NewSlotSource(0, 0),
		}, good)
		if report.Valid() || !hasCode(report, CodeInvalidCorrelation) {
			t.Fatalf("authority accepted misaddressed scalar/span slots: %s", report.Error())
		}
	})

	t.Run("span denominator differs from complete child", func(t *testing.T) {
		report := build(t, []algebra.SlotSource{
			algebra.NewSlotSource(0, 0), algebra.NewSlotSource(1, 0), algebra.NewSlotSource(1, 0),
		}, wrong)
		if report.Valid() || !hasCode(report, CodeInvalidCorrelation) {
			t.Fatalf("authority accepted mixed-denominator scalar/span form: %s", report.Error())
		}
	})
}

type scalarAuthorityIDs struct {
	owner     model.OwnerID
	schema    model.SchemaID
	relation  model.RelationID
	column    model.ColumnID
	key       model.KeyID
	scope     model.ScopeID
	typeID    model.TypeID
	operation signature.Identity
}

func newScalarAuthorityIDs(t *testing.T) scalarAuthorityIDs {
	t.Helper()
	owner := issueOwner(t, "scalar-population-owner")
	schema := issueSchema(t, owner, "scalar-population-schema")
	relation := issueRelation(t, owner, "scalar-population-relation")
	column := issueColumn(t, relation, "scalar-population-column")
	key := issueKey(t, relation, "scalar-population-key")
	scope := issueScope(t, owner, "scalar-population-scope")
	typeID := issueType(t, owner, "scalar-population-type")
	operation := issueOperation(t, owner, "scalar-population-operation")
	return scalarAuthorityIDs{owner: owner, schema: schema, relation: relation, column: column, key: key, scope: scope, typeID: typeID, operation: signature.Identity{Operation: operation, Version: 1}}
}

func scalarAuthoritySchema(t *testing.T, value scalarAuthorityIDs, expression algebra.Expression, spanDenominator model.DenominatorRef) plan.ExecutionSchema {
	t.Helper()
	good, ok := model.NewDenominatorRef(value.relation, value.key)
	if !ok {
		t.Fatal("population denominator")
	}
	wrongKey := issueKey(t, value.relation, "scalar-population-wrong-key")
	scope := model.DefineScopeSchema(value.scope, []model.ColumnID{value.column}, region.True())
	relation := model.DefineRelationSchema(value.relation, []model.ColumnID{value.column}, []model.KeyID{value.key, wrongKey}, value.scope)
	column := model.DefineColumnSchema(value.column, value.typeID)
	key := model.DefineKeySchema(value.key, []model.ColumnID{value.column})
	extraKey := model.DefineKeySchema(wrongKey, []model.ColumnID{value.column})
	scalar, ok := signature.NewScalarDelivery()
	if !ok {
		t.Fatal("scalar delivery")
	}
	complete, ok := signature.NewCompleteSpanDelivery(value.key)
	if !ok {
		t.Fatal("complete delivery")
	}
	accepted, ok := outcome.NewSet(outcome.Produced, outcome.NoCandidate, outcome.NoSelection, outcome.Refused)
	if !ok {
		t.Fatal("outcomes")
	}
	sig, ok := signature.Seal(signature.Spec{
		Identity: value.operation,
		Fence:    signature.Fence{Owner: value.owner, Schema: value.schema},
		Inputs: []signature.Input{
			{Relation: value.relation, Column: value.column, Type: value.typeID, Presence: signature.RequirePresent, Delivery: scalar, Denominator: good},
			{Relation: value.relation, Column: value.column, Type: value.typeID, Presence: signature.RequirePresent, Delivery: complete, Denominator: spanDenominator},
			{Relation: value.relation, Column: value.column, Type: value.typeID, Presence: signature.RequirePresent, Delivery: complete, Denominator: spanDenominator},
		},
		Outputs:     []signature.Output{{Relation: value.relation, Column: value.column, Type: value.typeID, Presence: signature.ProducePresent, Denominator: good}},
		Cardinality: mustAuthorityCardinality(t),
		Outcomes:    accepted,
	})
	if !ok {
		t.Fatal("signature")
	}
	builder := plan.NewBuilder(value.schema)
	if !builder.AddRelation(relation) || !builder.AddColumn(column) || !builder.AddKey(key) || !builder.AddKey(extraKey) || !builder.AddScope(scope) || !builder.AddSignature(sig) {
		t.Fatal("schema declarations")
	}
	if !builder.AddExpression(plan.DefineExpressionRef(issueExpression(t, value.owner, "scalar-population-expression"), expression)) {
		t.Fatal("expression")
	}
	schema, ok := builder.Build()
	if !ok {
		t.Fatal("schema")
	}
	return schema
}

func mustAuthorityCardinality(t *testing.T) model.Cardinality {
	t.Helper()
	value, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("cardinality")
	}
	return value
}
