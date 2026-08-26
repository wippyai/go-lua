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

// These laws keep broadcast validation independent of typing.  The global
// child is a different relation from the Q population, so an empty projection
// cannot be mistaken for a missing Q posting.
func TestAuthorityAcceptsSharedCompleteCorrelationChild(t *testing.T) {
	value := newSharedCorrelationAuthorityIDs(t)
	population := mustSharedDenominator(t, value.population, value.populationKey)
	global := mustSharedDenominator(t, value.global, value.globalKey)
	scalar, ok := signature.NewScalarDelivery()
	if !ok {
		t.Fatal("scalar delivery")
	}
	complete, ok := signature.NewCompleteSpanDelivery(value.globalKey)
	if !ok {
		t.Fatal("complete delivery")
	}
	shared := algebra.NewComplete(
		algebra.NewSelect(algebra.NewInput(value.global), algebra.NewSelectContract(algebra.SelectByScope, value.scope)),
		global,
	)
	correlation := algebra.NewApplyCorrelation(population, value.populationColumn, value.typeID, [][]model.ColumnID{{value.populationColumn}, {}})
	contract, ok := algebra.NewCorrelatedApplyContract(value.operation, []algebra.SlotSource{
		algebra.NewSlotSource(0, 0), algebra.NewSlotSource(1, 0),
	}, correlation, algebra.OwnerNamed())
	if !ok {
		t.Fatal("correlated contract")
	}
	apply := algebra.NewApply([]algebra.Expression{algebra.NewInput(value.population), shared}, contract)
	schema := sharedCorrelationAuthoritySchema(t, value, apply, []signature.Input{
		{Relation: value.population, Column: value.populationColumn, Type: value.typeID, Presence: signature.RequirePresent, Delivery: scalar, Denominator: population},
		{Relation: value.global, Column: value.globalColumn, Type: value.typeID, Presence: signature.RequirePresent, Delivery: complete, Denominator: global},
	})
	if report := Check(schema); !report.Valid() {
		t.Fatalf("authority rejected shared Complete child: %s", report.Error())
	}
}

func TestAuthorityRejectsSharedCompleteCorrelationMutations(t *testing.T) {
	value := newSharedCorrelationAuthorityIDs(t)
	population := mustSharedDenominator(t, value.population, value.populationKey)
	global := mustSharedDenominator(t, value.global, value.globalKey)
	scalar, ok := signature.NewScalarDelivery()
	if !ok {
		t.Fatal("scalar delivery")
	}
	globalComplete, ok := signature.NewCompleteSpanDelivery(value.globalKey)
	if !ok {
		t.Fatal("global complete delivery")
	}
	populationComplete, ok := signature.NewCompleteSpanDelivery(value.populationKey)
	if !ok {
		t.Fatal("population complete delivery")
	}
	correlation := algebra.NewApplyCorrelation(population, value.populationColumn, value.typeID, [][]model.ColumnID{{value.populationColumn}, {}})
	build := func(t *testing.T, shared algebra.Expression, input signature.Input) Report {
		t.Helper()
		contract, ok := algebra.NewCorrelatedApplyContract(value.operation, []algebra.SlotSource{
			algebra.NewSlotSource(0, 0), algebra.NewSlotSource(1, 0),
		}, correlation, algebra.OwnerNamed())
		if !ok {
			t.Fatal("correlated contract")
		}
		apply := algebra.NewApply([]algebra.Expression{algebra.NewInput(value.population), shared}, contract)
		return Check(sharedCorrelationAuthoritySchema(t, value, apply, []signature.Input{
			{Relation: value.population, Column: value.populationColumn, Type: value.typeID, Presence: signature.RequirePresent, Delivery: scalar, Denominator: population},
			input,
		}))
	}

	t.Run("non-direct Complete child", func(t *testing.T) {
		shared := algebra.NewComplete(algebra.NewGroup(algebra.NewInput(value.global), algebra.NewGroupContract(value.globalKey, mustAuthorityCardinality(t))), global)
		report := build(t, shared, signature.Input{Relation: value.global, Column: value.globalColumn, Type: value.typeID, Presence: signature.RequirePresent, Delivery: globalComplete, Denominator: global})
		if report.Valid() || !hasCode(report, CodeInvalidCorrelation) {
			t.Fatalf("authority accepted non-direct shared child: %s", report.Error())
		}
	})

	t.Run("population coordinate in shared child", func(t *testing.T) {
		shared := algebra.NewComplete(algebra.NewSelect(algebra.NewInput(value.population), algebra.NewSelectContract(algebra.SelectByScope, value.scope)), population)
		report := build(t, shared, signature.Input{Relation: value.population, Column: value.populationColumn, Type: value.typeID, Presence: signature.RequirePresent, Delivery: populationComplete, Denominator: population})
		if report.Valid() || !hasCode(report, CodeInvalidCorrelation) {
			t.Fatalf("authority accepted coordinate-bearing shared child: %s", report.Error())
		}
	})
}

type sharedCorrelationAuthorityIDs struct {
	owner            model.OwnerID
	schema           model.SchemaID
	population       model.RelationID
	populationColumn model.ColumnID
	populationKey    model.KeyID
	global           model.RelationID
	globalColumn     model.ColumnID
	globalKey        model.KeyID
	scope            model.ScopeID
	typeID           model.TypeID
	operation        signature.Identity
}

func newSharedCorrelationAuthorityIDs(t *testing.T) sharedCorrelationAuthorityIDs {
	t.Helper()
	owner := issueOwner(t, "shared-correlation-owner")
	schema := issueSchema(t, owner, "shared-correlation-schema")
	population := issueRelation(t, owner, "shared-correlation-population")
	populationColumn := issueColumn(t, population, "shared-correlation-coordinate")
	populationKey := issueKey(t, population, "shared-correlation-population-key")
	global := issueRelation(t, owner, "shared-correlation-global")
	globalColumn := issueColumn(t, global, "shared-correlation-global-column")
	globalKey := issueKey(t, global, "shared-correlation-global-key")
	scope := issueScope(t, owner, "shared-correlation-scope")
	typeID := issueType(t, owner, "shared-correlation-type")
	operation := issueOperation(t, owner, "shared-correlation-operation")
	return sharedCorrelationAuthorityIDs{
		owner: owner, schema: schema,
		population: population, populationColumn: populationColumn, populationKey: populationKey,
		global: global, globalColumn: globalColumn, globalKey: globalKey,
		scope: scope, typeID: typeID, operation: signature.Identity{Operation: operation, Version: 1},
	}
}

func mustSharedDenominator(t *testing.T, relation model.RelationID, key model.KeyID) model.DenominatorRef {
	t.Helper()
	denominator, ok := model.NewDenominatorRef(relation, key)
	if !ok {
		t.Fatal("denominator")
	}
	return denominator
}

func sharedCorrelationAuthoritySchema(t *testing.T, value sharedCorrelationAuthorityIDs, expression algebra.Expression, inputs []signature.Input) plan.ExecutionSchema {
	t.Helper()
	population := mustSharedDenominator(t, value.population, value.populationKey)
	accepted, ok := outcome.NewSet(outcome.Produced, outcome.NoCandidate, outcome.NoSelection, outcome.Refused)
	if !ok {
		t.Fatal("outcomes")
	}
	sig, ok := signature.Seal(signature.Spec{
		Identity: value.operation,
		Fence:    signature.Fence{Owner: value.owner, Schema: value.schema},
		Inputs:   inputs,
		Outputs: []signature.Output{{
			Relation: value.population, Column: value.populationColumn, Type: value.typeID, Presence: signature.ProducePresent, Denominator: population,
		}},
		Cardinality: mustAuthorityCardinality(t),
		Outcomes:    accepted,
	})
	if !ok {
		t.Fatal("signature")
	}
	builder := plan.NewBuilder(value.schema)
	if !builder.AddRelation(model.DefineRelationSchema(value.population, []model.ColumnID{value.populationColumn}, []model.KeyID{value.populationKey}, value.scope)) ||
		!builder.AddRelation(model.DefineRelationSchema(value.global, []model.ColumnID{value.globalColumn}, []model.KeyID{value.globalKey}, value.scope)) ||
		!builder.AddColumn(model.DefineColumnSchema(value.populationColumn, value.typeID)) ||
		!builder.AddColumn(model.DefineColumnSchema(value.globalColumn, value.typeID)) ||
		!builder.AddKey(model.DefineKeySchema(value.populationKey, []model.ColumnID{value.populationColumn})) ||
		!builder.AddKey(model.DefineKeySchema(value.globalKey, []model.ColumnID{value.globalColumn})) ||
		!builder.AddScope(model.DefineScopeSchema(value.scope, []model.ColumnID{value.populationColumn}, region.True())) ||
		!builder.AddSignature(sig) {
		t.Fatal("schema declarations")
	}
	if !builder.AddExpression(plan.DefineExpressionRef(issueExpression(t, value.owner, "shared-correlation-expression"), expression)) {
		t.Fatal("expression")
	}
	schema, ok := builder.Build()
	if !ok {
		t.Fatal("schema")
	}
	return schema
}
