package typing_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/relation/check/typing"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

// TestScalarPopulationCorrelationIsTheClosedTwoChildForm is the canonical
// summary shape: one population Input supplies the scalar coordinate and one
// Complete(Select(Input)) child supplies several slots from one exact span.
// The latter must remain one child group so the semantic invocation does not
// turn those sibling slots into a Cartesian product.
func TestScalarPopulationCorrelationIsTheClosedTwoChildForm(t *testing.T) {
	value := newFixture(t)
	completeDelivery, ok := signature.NewCompleteSpanDelivery(value.keyB)
	if !ok {
		t.Fatal("complete delivery")
	}
	scalarDelivery, ok := signature.NewScalarDelivery()
	if !ok {
		t.Fatal("scalar delivery")
	}
	accepted, ok := outcome.NewSet(outcome.Produced, outcome.NoCandidate, outcome.NoSelection, outcome.Refused)
	if !ok {
		t.Fatal("outcomes")
	}
	span := algebra.NewComplete(
		algebra.NewSelect(algebra.NewInput(value.relationB), algebra.NewSelectContract(algebra.SelectByScope, value.scope)),
		value.denominatorB,
	)
	correlation := algebra.NewApplyCorrelation(value.denominatorA, value.columnA, value.typeID, [][]model.ColumnID{{value.columnA}, {value.columnB}})
	contract, ok := algebra.NewCorrelatedApplyContract(value.operation, []algebra.SlotSource{
		algebra.NewSlotSource(0, 0),
		algebra.NewSlotSource(1, 0),
		algebra.NewSlotSource(1, 0),
	}, correlation, algebra.OwnerNamed())
	if !ok {
		t.Fatal("correlated contract")
	}
	apply := algebra.NewApply([]algebra.Expression{algebra.NewInput(value.relationA), span}, contract)

	schema := scalarPopulationSchema(t, value, apply, signature.Spec{
		Identity: value.operation,
		Fence:    signature.Fence{Owner: value.owner, Schema: value.schema},
		Inputs: []signature.Input{
			{Relation: value.relationA, Column: value.columnA, Type: value.typeID, Presence: signature.RequirePresent, Delivery: scalarDelivery, Denominator: value.denominatorA},
			{Relation: value.relationB, Column: value.columnB, Type: value.typeID, Presence: signature.RequirePresent, Delivery: completeDelivery, Denominator: value.denominatorB},
			{Relation: value.relationB, Column: value.columnB, Type: value.typeID, Presence: signature.RequirePresent, Delivery: completeDelivery, Denominator: value.denominatorB},
		},
		Outputs:     []signature.Output{{Relation: value.relationB, Column: value.columnB, Type: value.typeID, Presence: signature.ProducePresent, Denominator: value.denominatorB}},
		Cardinality: mustCardinality(t, model.ExactlyOne),
		Outcomes:    accepted,
	})
	if report := typing.Check(schema); !report.Valid() {
		t.Fatalf("closed scalar-population form rejected: %v", report.Error())
	}
}

// TestScalarPopulationCorrelationAdmitsSharedCompleteChild proves that an
// empty correlation projection is not an empty posting. The second child is
// one globally closed vector and therefore has no site coordinate or
// per-site partition relation to materialize.
func TestScalarPopulationCorrelationAdmitsSharedCompleteChild(t *testing.T) {
	value := newFixture(t)
	completeDelivery, ok := signature.NewCompleteSpanDelivery(value.keyB)
	if !ok {
		t.Fatal("complete delivery")
	}
	scalarDelivery, ok := signature.NewScalarDelivery()
	if !ok {
		t.Fatal("scalar delivery")
	}
	accepted, ok := outcome.NewSet(outcome.Produced, outcome.NoCandidate, outcome.NoSelection, outcome.Refused)
	if !ok {
		t.Fatal("outcomes")
	}
	shared := algebra.NewComplete(
		algebra.NewSelect(algebra.NewInput(value.relationB), algebra.NewSelectContract(algebra.SelectByScope, value.scope)),
		value.denominatorB,
	)
	correlation := algebra.NewApplyCorrelation(value.denominatorA, value.columnA, value.typeID, [][]model.ColumnID{{value.columnA}, {}})
	contract, ok := algebra.NewCorrelatedApplyContract(value.operation, []algebra.SlotSource{
		algebra.NewSlotSource(0, 0),
		algebra.NewSlotSource(1, 0),
	}, correlation, algebra.OwnerNamed())
	if !ok {
		t.Fatal("correlated contract")
	}
	apply := algebra.NewApply([]algebra.Expression{algebra.NewInput(value.relationA), shared}, contract)
	schema := scalarPopulationSchema(t, value, apply, signature.Spec{
		Identity: value.operation,
		Fence:    signature.Fence{Owner: value.owner, Schema: value.schema},
		Inputs: []signature.Input{
			{Relation: value.relationA, Column: value.columnA, Type: value.typeID, Presence: signature.RequirePresent, Delivery: scalarDelivery, Denominator: value.denominatorA},
			{Relation: value.relationB, Column: value.columnB, Type: value.typeID, Presence: signature.RequirePresent, Delivery: completeDelivery, Denominator: value.denominatorB},
		},
		Outputs:     []signature.Output{{Relation: value.relationB, Column: value.columnB, Type: value.typeID, Presence: signature.ProducePresent, Denominator: value.denominatorB}},
		Cardinality: mustCardinality(t, model.ExactlyOne),
		Outcomes:    accepted,
	})
	if report := typing.Check(schema); !report.Valid() {
		t.Fatalf("shared Complete child rejected: %v", report.Error())
	}
}

func TestScalarPopulationCorrelationRejectsMalformedSharedCompleteChild(t *testing.T) {
	value := newFixture(t)
	completeDelivery, ok := signature.NewCompleteSpanDelivery(value.keyB)
	if !ok {
		t.Fatal("complete delivery")
	}
	scalarDelivery, ok := signature.NewScalarDelivery()
	if !ok {
		t.Fatal("scalar delivery")
	}
	accepted, ok := outcome.NewSet(outcome.Produced, outcome.NoCandidate, outcome.NoSelection, outcome.Refused)
	if !ok {
		t.Fatal("outcomes")
	}
	correlation := algebra.NewApplyCorrelation(value.denominatorA, value.columnA, value.typeID, [][]model.ColumnID{{value.columnA}, {}})
	build := func(t *testing.T, shared algebra.Expression, sharedInput signature.Input) typing.Report {
		t.Helper()
		contract, ok := algebra.NewCorrelatedApplyContract(value.operation, []algebra.SlotSource{
			algebra.NewSlotSource(0, 0),
			algebra.NewSlotSource(1, 0),
		}, correlation, algebra.OwnerNamed())
		if !ok {
			t.Fatal("correlated contract")
		}
		apply := algebra.NewApply([]algebra.Expression{algebra.NewInput(value.relationA), shared}, contract)
		return typing.Check(scalarPopulationSchema(t, value, apply, signature.Spec{
			Identity: value.operation,
			Fence:    signature.Fence{Owner: value.owner, Schema: value.schema},
			Inputs: []signature.Input{
				{Relation: value.relationA, Column: value.columnA, Type: value.typeID, Presence: signature.RequirePresent, Delivery: scalarDelivery, Denominator: value.denominatorA},
				sharedInput,
			},
			Outputs:     []signature.Output{{Relation: value.relationB, Column: value.columnB, Type: value.typeID, Presence: signature.ProducePresent, Denominator: value.denominatorB}},
			Cardinality: mustCardinality(t, model.ExactlyOne),
			Outcomes:    accepted,
		}))
	}

	t.Run("non-direct Complete child", func(t *testing.T) {
		group := algebra.NewGroup(algebra.NewInput(value.relationB), algebra.NewGroupContract(value.keyB, mustCardinality(t, model.ExactlyOne)))
		shared := algebra.NewComplete(group, value.denominatorB)
		report := build(t, shared, signature.Input{Relation: value.relationB, Column: value.columnB, Type: value.typeID, Presence: signature.RequirePresent, Delivery: completeDelivery, Denominator: value.denominatorB})
		if report.Valid() || !hasIssue(report, typing.CodeCorrelationMismatch) {
			t.Fatalf("non-direct shared child accepted: %v", report.Issues())
		}
	})

	t.Run("scalar shared slot", func(t *testing.T) {
		shared := algebra.NewComplete(algebra.NewSelect(algebra.NewInput(value.relationB), algebra.NewSelectContract(algebra.SelectByScope, value.scope)), value.denominatorB)
		report := build(t, shared, signature.Input{Relation: value.relationB, Column: value.columnB, Type: value.typeID, Presence: signature.RequirePresent, Delivery: scalarDelivery, Denominator: value.denominatorB})
		if report.Valid() || !hasIssue(report, typing.CodeCorrelationMismatch) {
			t.Fatalf("scalar shared slot accepted: %v", report.Issues())
		}
	})

	t.Run("shared child retains population coordinate", func(t *testing.T) {
		completeA, completeAOK := signature.NewCompleteSpanDelivery(value.keyA)
		if !completeAOK {
			t.Fatal("complete A delivery")
		}
		shared := algebra.NewComplete(algebra.NewSelect(algebra.NewInput(value.relationA), algebra.NewSelectContract(algebra.SelectByScope, value.scope)), value.denominatorA)
		report := build(t, shared, signature.Input{Relation: value.relationA, Column: value.columnA, Type: value.typeID, Presence: signature.RequirePresent, Delivery: completeA, Denominator: value.denominatorA})
		if report.Valid() || !hasIssue(report, typing.CodeCorrelationMismatch) {
			t.Fatalf("coordinate-bearing shared child accepted: %v", report.Issues())
		}
	})
}

func TestScalarPopulationCorrelationRejectsNearestMixedShapes(t *testing.T) {
	value := newFixture(t)
	completeDelivery, ok := signature.NewCompleteSpanDelivery(value.keyB)
	if !ok {
		t.Fatal("complete delivery")
	}
	scalarDelivery, ok := signature.NewScalarDelivery()
	if !ok {
		t.Fatal("scalar delivery")
	}
	accepted, ok := outcome.NewSet(outcome.Produced, outcome.NoCandidate, outcome.NoSelection, outcome.Refused)
	if !ok {
		t.Fatal("outcomes")
	}
	span := algebra.NewComplete(
		algebra.NewSelect(algebra.NewInput(value.relationB), algebra.NewSelectContract(algebra.SelectByScope, value.scope)),
		value.denominatorB,
	)
	correlation := algebra.NewApplyCorrelation(value.denominatorA, value.columnA, value.typeID, [][]model.ColumnID{{value.columnA}, {value.columnB}})

	build := func(t *testing.T, slots []algebra.SlotSource, spanDenominator model.DenominatorRef) typing.Report {
		t.Helper()
		contract, ok := algebra.NewCorrelatedApplyContract(value.operation, slots, correlation, algebra.OwnerNamed())
		if !ok {
			t.Fatal("correlated contract")
		}
		apply := algebra.NewApply([]algebra.Expression{algebra.NewInput(value.relationA), span}, contract)
		return typing.Check(scalarPopulationSchema(t, value, apply, signature.Spec{
			Identity: value.operation,
			Fence:    signature.Fence{Owner: value.owner, Schema: value.schema},
			Inputs: []signature.Input{
				{Relation: value.relationA, Column: value.columnA, Type: value.typeID, Presence: signature.RequirePresent, Delivery: scalarDelivery, Denominator: value.denominatorA},
				{Relation: value.relationB, Column: value.columnB, Type: value.typeID, Presence: signature.RequirePresent, Delivery: completeDelivery, Denominator: spanDenominator},
				{Relation: value.relationB, Column: value.columnB, Type: value.typeID, Presence: signature.RequirePresent, Delivery: completeDelivery, Denominator: spanDenominator},
			},
			Outputs:     []signature.Output{{Relation: value.relationB, Column: value.columnB, Type: value.typeID, Presence: signature.ProducePresent, Denominator: value.denominatorB}},
			Cardinality: mustCardinality(t, model.ExactlyOne),
			Outcomes:    accepted,
		}))
	}

	t.Run("span slot addressed to scalar child", func(t *testing.T) {
		report := build(t, []algebra.SlotSource{
			algebra.NewSlotSource(0, 0),
			algebra.NewSlotSource(1, 0),
			algebra.NewSlotSource(0, 0),
		}, value.denominatorB)
		if report.Valid() || !hasIssue(report, typing.CodeCorrelationMismatch) {
			t.Fatalf("misaddressed scalar/span slots were accepted: %v", report.Issues())
		}
	})

	t.Run("span denominator differs from complete child", func(t *testing.T) {
		report := build(t, []algebra.SlotSource{
			algebra.NewSlotSource(0, 0),
			algebra.NewSlotSource(1, 0),
			algebra.NewSlotSource(1, 0),
		}, value.denominatorA)
		if report.Valid() || !hasIssue(report, typing.CodeCorrelationMismatch) {
			t.Fatalf("mixed-denominator scalar/span form was accepted: %v", report.Issues())
		}
	})
}

func scalarPopulationSchema(t *testing.T, value fixture, expression algebra.Expression, specification signature.Spec) plan.ExecutionSchema {
	t.Helper()
	base := schemaWith(t, value, nil, false)
	builder := plan.NewBuilder(value.schema)
	for _, declaration := range base.Relations() {
		if !builder.AddRelation(declaration) {
			t.Fatal("copy relation")
		}
	}
	for _, declaration := range base.Columns() {
		if !builder.AddColumn(declaration) {
			t.Fatal("copy column")
		}
	}
	for _, declaration := range base.Keys() {
		if !builder.AddKey(declaration) {
			t.Fatal("copy key")
		}
	}
	for _, declaration := range base.Scopes() {
		if !builder.AddScope(declaration) {
			t.Fatal("copy scope")
		}
	}
	for _, declaration := range base.TypeCapabilities() {
		if !builder.AddTypeCapability(declaration) {
			t.Fatal("copy type capability")
		}
	}
	sealed, ok := signature.Seal(specification)
	if !ok || !builder.AddSignature(sealed) {
		t.Fatal("add scalar-population signature")
	}
	if !builder.AddExpression(plan.DefineExpressionRef(issueExpression(t, value.owner, "scalar-population"), expression)) {
		t.Fatal("add scalar-population expression")
	}
	schema, ok := builder.Build()
	if !ok {
		t.Fatal("build scalar-population schema")
	}
	return schema
}
