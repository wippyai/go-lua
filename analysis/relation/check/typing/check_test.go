package typing_test

import (
	"bytes"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/check/typing"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
	"github.com/wippyai/go-lua/analysis/relation/schema/region"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

type fixture struct {
	owner        model.OwnerID
	schema       model.SchemaID
	typeID       model.TypeID
	relationA    model.RelationID
	relationB    model.RelationID
	columnA      model.ColumnID
	columnB      model.ColumnID
	keyA         model.KeyID
	keyB         model.KeyID
	scope        model.ScopeID
	denominatorA model.DenominatorRef
	denominatorB model.DenominatorRef
	operation    signature.Identity
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	owner := issueOwner(t, "owner")
	schema := issueSchema(t, owner, "schema")
	typeID := issueType(t, owner, "value")
	relationA := issueRelation(t, owner, "a")
	relationB := issueRelation(t, owner, "b")
	columnA := issueColumn(t, relationA, "a")
	columnB := issueColumn(t, relationB, "b")
	keyA := issueKey(t, relationA, "a")
	keyB := issueKey(t, relationB, "b")
	scope := issueScope(t, owner, "scope")
	denominatorA, ok := model.NewDenominatorRef(relationA, keyA)
	if !ok {
		t.Fatal("denominator A")
	}
	denominatorB, ok := model.NewDenominatorRef(relationB, keyB)
	if !ok {
		t.Fatal("denominator B")
	}
	operationID := issueOperation(t, owner, "operation")
	return fixture{owner: owner, schema: schema, typeID: typeID, relationA: relationA, relationB: relationB, columnA: columnA, columnB: columnB, keyA: keyA, keyB: keyB, scope: scope, denominatorA: denominatorA, denominatorB: denominatorB, operation: signature.Identity{Operation: operationID, Version: 1}}
}

func validSchema(t *testing.T, value fixture, expressions ...algebra.Expression) plan.ExecutionSchema {
	t.Helper()
	delivery, ok := signature.NewScalarDelivery()
	if !ok {
		t.Fatal("scalar delivery")
	}
	return schemaWithDelivery(t, value, expressions, true, delivery)
}

func schemaWith(t *testing.T, value fixture, expressions []algebra.Expression, includeSignature bool) plan.ExecutionSchema {
	t.Helper()
	delivery, ok := signature.NewScalarDelivery()
	if !ok {
		t.Fatal("scalar delivery")
	}
	return schemaWithDelivery(t, value, expressions, includeSignature, delivery)
}

func schemaWithDelivery(t *testing.T, value fixture, expressions []algebra.Expression, includeSignature bool, delivery signature.Delivery) plan.ExecutionSchema {
	t.Helper()
	scope := model.DefineScopeSchema(value.scope, []model.ColumnID{value.columnA, value.columnB}, region.True())
	relationA := model.DefineRelationSchema(value.relationA, []model.ColumnID{value.columnA}, []model.KeyID{value.keyA}, value.scope)
	relationB := model.DefineRelationSchema(value.relationB, []model.ColumnID{value.columnB}, []model.KeyID{value.keyB}, value.scope)
	columnA := model.DefineColumnSchema(value.columnA, value.typeID)
	columnB := model.DefineColumnSchema(value.columnB, value.typeID)
	keyA := model.DefineKeySchema(value.keyA, []model.ColumnID{value.columnA})
	keyB := model.DefineKeySchema(value.keyB, []model.ColumnID{value.columnB})
	builder := plan.NewBuilder(value.schema)
	for _, declaration := range []model.RelationSchema{relationA, relationB} {
		if !builder.AddRelation(declaration) {
			t.Fatal("add relation")
		}
	}
	for _, declaration := range []model.ColumnSchema{columnA, columnB} {
		if !builder.AddColumn(declaration) {
			t.Fatal("add column")
		}
	}
	for _, declaration := range []model.KeySchema{keyA, keyB} {
		if !builder.AddKey(declaration) {
			t.Fatal("add key")
		}
	}
	if !builder.AddScope(scope) {
		t.Fatal("add scope")
	}
	capability, capabilityOK := model.NewAscendingCapability(value.typeID)
	if !capabilityOK || !builder.AddTypeCapability(capability) {
		t.Fatal("add ascending type capability")
	}
	for index, expression := range expressions {
		id := issueExpression(t, value.owner, string(rune('a'+index)))
		if !builder.AddExpression(plan.DefineExpressionRef(id, expression)) {
			t.Fatal("add expression")
		}
	}
	if includeSignature {
		accepted, ok := outcome.NewSet(outcome.Produced, outcome.NoCandidate, outcome.NoSelection, outcome.Refused)
		if !ok {
			t.Fatal("outcomes")
		}
		signatureValue, ok := signature.Seal(signature.Spec{
			Identity:    value.operation,
			Fence:       signature.Fence{Owner: value.owner, Schema: value.schema},
			Inputs:      []signature.Input{{Relation: value.relationA, Column: value.columnA, Type: value.typeID, Presence: signature.RequirePresent, Delivery: delivery, Denominator: value.denominatorA}},
			Outputs:     []signature.Output{{Relation: value.relationB, Column: value.columnB, Type: value.typeID, Presence: signature.ProducePresent, Denominator: value.denominatorB}},
			Cardinality: mustCardinality(t, model.ExactlyOne), Outcomes: accepted,
		})
		if !ok || !builder.AddSignature(signatureValue) {
			t.Fatal("add signature")
		}
	}
	schema, ok := builder.Build()
	if !ok {
		t.Fatal("build schema")
	}
	return schema
}

func TestValidClosedOperatorSpecIsAccepted(t *testing.T) {
	value := newFixture(t)
	inputA := algebra.NewInput(value.relationA)
	inputB := algebra.NewInput(value.relationB)
	selectA := algebra.NewSelect(inputA, algebra.NewSelectContract(algebra.SelectByScope, value.scope))
	projectB := algebra.NewProject(inputA, algebra.NewProjectContract(value.relationB, []algebra.ColumnMapping{algebra.NewColumnMapping(value.columnA, value.columnB)}, value.keyB))
	join := algebra.NewJoin(inputA, inputB, algebra.NewJoinContract([]model.ColumnID{value.columnA}, []model.ColumnID{value.columnB}))
	merge := algebra.NewMerge([]algebra.Expression{inputA, inputA}, algebra.NewMergeContract(value.keyA))
	group := algebra.NewGroup(inputA, algebra.NewGroupContract(value.keyA, mustCardinality(t, model.ExactlyOne)))
	complete := algebra.NewComplete(inputA, value.denominatorA)
	apply := algebra.NewApply([]algebra.Expression{inputA}, algebra.NewApplyContract(value.operation, []algebra.SlotSource{algebra.NewSlotSource(0, 0)}, algebra.OwnerNamed()))
	publish := algebra.NewPublish(apply, algebra.NewPublishContract(value.relationB, value.keyB))
	schema := validSchema(t, value, inputA, selectA, projectB, join, merge, group, complete, apply, publish)
	report := typing.Check(schema)
	if !report.Valid() {
		t.Fatalf("valid closed operator schema rejected: %v", report.Error())
	}
	if got := len(report.MergeRequirements()); got != 0 {
		t.Fatalf("Merge incorrectly requested lattice ascent for its key column: got %d", got)
	}
	if got := len(report.EqualityRequirements()); got != 6 {
		t.Fatalf("key operators did not expose their semantic equality obligations: got %d", got)
	}
	requirements := report.AlgebraRequirements()
	if len(requirements) != 1 || requirements[0] != value.typeID {
		t.Fatalf("unexpected canonical algebra requirements: %+v", requirements)
	}
	requirements[0] = model.TypeID{}
	if got := report.AlgebraRequirements(); len(got) != 1 || got[0] != value.typeID {
		t.Fatal("algebra requirements exposed mutable storage")
	}
}

func TestMergeSeparatesKeyEqualityFromValueAscent(t *testing.T) {
	owner := issueOwner(t, "merge-capability-owner")
	schemaID := issueSchema(t, owner, "merge-capability-schema")
	relation := issueRelation(t, owner, "merge-capability-relation")
	keyColumn := issueColumn(t, relation, "merge-key")
	valueColumn := issueColumn(t, relation, "merge-value")
	keyID := issueKey(t, relation, "merge-key")
	scope := issueScope(t, owner, "merge-capability-scope")
	keyType := issueType(t, owner, "merge-key-type")
	valueType := issueType(t, owner, "merge-value-type")
	input := algebra.NewInput(relation)
	merge := algebra.NewMerge([]algebra.Expression{input, input}, algebra.NewMergeContract(keyID))

	build := func(t *testing.T, valueCapability model.TypeCapability) typing.Report {
		t.Helper()
		builder := plan.NewBuilder(schemaID)
		if !builder.AddRelation(model.DefineRelationSchema(relation, []model.ColumnID{keyColumn, valueColumn}, []model.KeyID{keyID}, scope)) ||
			!builder.AddColumn(model.DefineColumnSchema(keyColumn, keyType)) ||
			!builder.AddColumn(model.DefineColumnSchema(valueColumn, valueType)) ||
			!builder.AddKey(model.DefineKeySchema(keyID, []model.ColumnID{keyColumn})) ||
			!builder.AddScope(model.DefineScopeSchema(scope, nil, region.True())) {
			t.Fatal("add merge capability declarations")
		}
		keyCapability, ok := model.NewEquatableCapability(keyType)
		if !ok || !builder.AddTypeCapability(keyCapability) || !builder.AddTypeCapability(valueCapability) {
			t.Fatal("add merge capabilities")
		}
		if !builder.AddExpression(plan.DefineExpressionRef(issueExpression(t, owner, "merge-capability-expression"), merge)) {
			t.Fatal("add merge expression")
		}
		schema, ok := builder.Build()
		if !ok {
			t.Fatal("build merge capability schema")
		}
		return typing.Check(schema)
	}

	decodeOnly, ok := model.NewDecodeOnlyCapability(valueType)
	if !ok {
		t.Fatal("decode-only value capability")
	}
	decodeReport := build(t, decodeOnly)
	if decodeReport.Valid() || !hasIssue(decodeReport, typing.CodeTypeCapabilityMismatch) {
		t.Fatalf("decode-only Merge value was accepted: %v", decodeReport.Issues())
	}
	ascending, ok := model.NewAscendingCapability(valueType)
	if !ok {
		t.Fatal("ascending value capability")
	}
	ascendingReport := build(t, ascending)
	if !ascendingReport.Valid() {
		t.Fatalf("ascending Merge value was rejected: %v", ascendingReport.Issues())
	}
	if len(ascendingReport.EqualityRequirements()) != 1 || len(ascendingReport.MergeRequirements()) != 1 {
		t.Fatalf("Merge obligations were not separated: equality=%v ascent=%v", ascendingReport.EqualityRequirements(), ascendingReport.MergeRequirements())
	}
}

func TestProposalMergeRejectsAlternateDestinationKey(t *testing.T) {
	value := newFixture(t)
	alternate := issueKey(t, value.relationB, "alternate-output-key")
	input := algebra.NewInput(value.relationA)
	apply := algebra.NewApply([]algebra.Expression{input}, algebra.NewApplyContract(value.operation, []algebra.SlotSource{algebra.NewSlotSource(0, 0)}, algebra.OwnerNamed()))
	merge := algebra.NewMerge([]algebra.Expression{apply, apply}, algebra.NewMergeContract(alternate))

	delivery, ok := signature.NewScalarDelivery()
	if !ok {
		t.Fatal("scalar delivery")
	}
	accepted, ok := outcome.NewSet(outcome.Produced, outcome.NoCandidate, outcome.NoSelection, outcome.Refused)
	if !ok {
		t.Fatal("outcomes")
	}
	signatureValue, ok := signature.Seal(signature.Spec{
		Identity: value.operation, Fence: signature.Fence{Owner: value.owner, Schema: value.schema},
		Inputs:      []signature.Input{{Relation: value.relationA, Column: value.columnA, Type: value.typeID, Presence: signature.RequirePresent, Delivery: delivery, Denominator: value.denominatorA}},
		Outputs:     []signature.Output{{Relation: value.relationB, Column: value.columnB, Type: value.typeID, Presence: signature.ProducePresent, Denominator: value.denominatorB}},
		Cardinality: mustCardinality(t, model.ExactlyOne), Outcomes: accepted,
	})
	if !ok {
		t.Fatal("signature")
	}
	builder := plan.NewBuilder(value.schema)
	if !builder.AddRelation(model.DefineRelationSchema(value.relationA, []model.ColumnID{value.columnA}, []model.KeyID{value.keyA}, value.scope)) ||
		!builder.AddRelation(model.DefineRelationSchema(value.relationB, []model.ColumnID{value.columnB}, []model.KeyID{value.keyB, alternate}, value.scope)) ||
		!builder.AddColumn(model.DefineColumnSchema(value.columnA, value.typeID)) ||
		!builder.AddColumn(model.DefineColumnSchema(value.columnB, value.typeID)) ||
		!builder.AddKey(model.DefineKeySchema(value.keyA, []model.ColumnID{value.columnA})) ||
		!builder.AddKey(model.DefineKeySchema(value.keyB, []model.ColumnID{value.columnB})) ||
		!builder.AddKey(model.DefineKeySchema(alternate, []model.ColumnID{value.columnB})) ||
		!builder.AddScope(model.DefineScopeSchema(value.scope, []model.ColumnID{value.columnA, value.columnB}, region.True())) ||
		!builder.AddSignature(signatureValue) ||
		!builder.AddExpression(plan.DefineExpressionRef(issueExpression(t, value.owner, "alternate-proposal-merge"), merge)) {
		t.Fatal("add alternate-key proposal schema")
	}
	capability, ok := model.NewAscendingCapability(value.typeID)
	if !ok || !builder.AddTypeCapability(capability) {
		t.Fatal("capability")
	}
	schema, ok := builder.Build()
	if !ok {
		t.Fatal("build schema")
	}
	report := typing.Check(schema)
	if report.Valid() || !hasIssue(report, typing.CodeKeyMismatch) {
		t.Fatalf("proposal Merge accepted a key not issued by the Apply output denominator: %v", report.Issues())
	}
}

func TestSpanApplyRequiresAProvenRangeBoundary(t *testing.T) {
	value := newFixture(t)
	bounded, ok := signature.NewBoundedSpanDelivery(1, value.keyA)
	if !ok {
		t.Fatal("bounded span delivery")
	}
	completeSpan, ok := signature.NewCompleteSpanDelivery(value.keyA)
	if !ok {
		t.Fatal("complete span delivery")
	}
	input := algebra.NewInput(value.relationA)

	// A nominal Input has the right relation and key, but it is only a flat
	// row stream. Accepting it here would let Apply interpret incidental batch
	// boundaries as the authored complete denominator range.
	direct := algebra.NewApply([]algebra.Expression{input}, algebra.NewApplyContract(value.operation, []algebra.SlotSource{algebra.NewSlotSource(0, 0)}, algebra.OwnerNamed()))
	directReport := typing.Check(schemaWithDelivery(t, value, []algebra.Expression{direct}, true, bounded))
	if directReport.Valid() {
		t.Fatal("span Apply over an unbounded Input was accepted")
	}
	if !hasIssue(directReport, typing.CodeDeliveryMismatch) {
		t.Fatalf("span Apply lacked range-boundary refusal: %v", directReport.Issues())
	}

	// Group proves an ordered bounded range.
	group := algebra.NewGroup(input, algebra.NewGroupContract(value.keyA, mustCardinality(t, model.ExactlyOne)))
	groupApply := algebra.NewApply([]algebra.Expression{group}, algebra.NewApplyContract(value.operation, []algebra.SlotSource{algebra.NewSlotSource(0, 0)}, algebra.OwnerNamed()))
	groupReport := typing.Check(schemaWithDelivery(t, value, []algebra.Expression{groupApply}, true, bounded))
	if !groupReport.Valid() {
		t.Fatalf("span Apply over Group rejected: %v", groupReport.Error())
	}

	// CompleteSpan is stronger: it requires the exact closed denominator
	// materialized by Complete, rather than merely any range boundary.
	complete := algebra.NewComplete(input, value.denominatorA)
	completeApply := algebra.NewApply([]algebra.Expression{complete}, algebra.NewApplyContract(value.operation, []algebra.SlotSource{algebra.NewSlotSource(0, 0)}, algebra.OwnerNamed()))
	completeReport := typing.Check(schemaWithDelivery(t, value, []algebra.Expression{completeApply}, true, completeSpan))
	if !completeReport.Valid() {
		t.Fatalf("span Apply over Complete rejected: %v", completeReport.Error())
	}
	completeOverGroup := typing.Check(schemaWithDelivery(t, value, []algebra.Expression{groupApply}, true, completeSpan))
	if completeOverGroup.Valid() || !hasIssue(completeOverGroup, typing.CodeDeliveryMismatch) {
		t.Fatalf("CompleteSpan Apply over Group accepted: %v", completeOverGroup.Issues())
	}
}

func TestApplyRequiresAtLeastOneDeliveredChild(t *testing.T) {
	value := newFixture(t)
	input := algebra.NewInput(value.relationA)
	positive := algebra.NewApply([]algebra.Expression{input}, algebra.NewApplyContract(value.operation, []algebra.SlotSource{algebra.NewSlotSource(0, 0)}, algebra.OwnerNamed()))
	if report := typing.Check(validSchema(t, value, positive)); !report.Valid() {
		t.Fatalf("one-child Apply rejected: %v", report.Error())
	}

	// Owner seed/materializer signatures may legitimately have no inputs: they
	// publish base facts at seal/mount, not through a runtime judgment. The
	// signature must remain sealable; only an algebra.Apply which tries to
	// invoke it with zero delivered children is invalid.
	accepted, acceptedOK := outcome.NewSet(outcome.Produced, outcome.NoCandidate, outcome.NoSelection, outcome.Refused)
	if !acceptedOK {
		t.Fatal("seed outcomes")
	}
	seed, seedOK := signature.Seal(signature.Spec{
		Identity:    value.operation,
		Fence:       signature.Fence{Owner: value.owner, Schema: value.schema},
		Outputs:     []signature.Output{{Relation: value.relationB, Column: value.columnB, Type: value.typeID, Presence: signature.ProducePresent, Denominator: value.denominatorB}},
		Cardinality: mustCardinality(t, model.ExactlyOne),
		Outcomes:    accepted,
	})
	if !seedOK || !seed.Available() || seed.InputLen() != 0 {
		t.Fatal("zero-input seed signature did not seal")
	}
	base := schemaWith(t, value, nil, false)
	builder := plan.NewBuilder(value.schema)
	for _, relation := range base.Relations() {
		if !builder.AddRelation(relation) {
			t.Fatal("seed add relation")
		}
	}
	for _, column := range base.Columns() {
		if !builder.AddColumn(column) {
			t.Fatal("seed add column")
		}
	}
	for _, key := range base.Keys() {
		if !builder.AddKey(key) {
			t.Fatal("seed add key")
		}
	}
	for _, scope := range base.Scopes() {
		if !builder.AddScope(scope) {
			t.Fatal("seed add scope")
		}
	}
	zero := algebra.NewApply([]algebra.Expression{}, algebra.NewApplyContract(value.operation, nil, algebra.OwnerNamed()))
	if !builder.AddExpression(plan.DefineExpressionRef(issueExpression(t, value.owner, "zero-child-apply"), zero)) || !builder.AddSignature(seed) {
		t.Fatal("seed Apply schema declarations")
	}
	schema, schemaOK := builder.Build()
	if !schemaOK {
		t.Fatal("seed Apply schema")
	}
	report := typing.Check(schema)
	if report.Valid() || !hasIssue(report, typing.CodeShapeMismatch) {
		t.Fatalf("zero-child Apply accepted: %v", report.Issues())
	}
}

func hasIssue(report typing.Report, want typing.Code) bool {
	for _, issue := range report.Issues() {
		if issue.Code == want {
			return true
		}
	}
	return false
}

func TestAlgebraRequirementsAreDeduplicatedAndCanonical(t *testing.T) {
	value := newFixture(t)
	secondType := issueType(t, value.owner, "second-value")
	base := schemaWith(t, value, nil, false)
	builder := plan.NewBuilder(value.schema)
	for _, relation := range base.Relations() {
		if !builder.AddRelation(relation) {
			t.Fatal("add relation")
		}
	}
	for _, column := range base.Columns() {
		if column.ID() == value.columnB {
			column = model.DefineColumnSchema(column.ID(), secondType)
		}
		if !builder.AddColumn(column) {
			t.Fatal("add column")
		}
	}
	for _, key := range base.Keys() {
		if !builder.AddKey(key) {
			t.Fatal("add key")
		}
	}
	for _, scope := range base.Scopes() {
		if !builder.AddScope(scope) {
			t.Fatal("add scope")
		}
	}
	capability, capabilityOK := model.NewAscendingCapability(value.typeID)
	secondCapability, secondCapabilityOK := model.NewAscendingCapability(secondType)
	if !capabilityOK || !builder.AddTypeCapability(capability) || !secondCapabilityOK || !builder.AddTypeCapability(secondCapability) {
		t.Fatal("add ascending type capability")
	}
	delivery, ok := signature.NewScalarDelivery()
	if !ok {
		t.Fatal("scalar delivery")
	}
	cardinality := mustCardinality(t, model.ExactlyOne)
	outcomes, ok := outcome.NewSet(outcome.Produced, outcome.NoCandidate, outcome.NoSelection, outcome.Refused)
	if !ok {
		t.Fatal("outcomes")
	}
	secondOperation := issueOperation(t, value.owner, "second-operation")
	for _, specification := range []signature.Spec{
		{
			Identity:    value.operation,
			Fence:       signature.Fence{Owner: value.owner, Schema: value.schema},
			Inputs:      []signature.Input{{Relation: value.relationA, Column: value.columnA, Type: value.typeID, Presence: signature.RequireOpaque, Delivery: delivery, Denominator: value.denominatorA}},
			Outputs:     []signature.Output{{Relation: value.relationA, Column: value.columnA, Type: value.typeID, Presence: signature.ProducePresent, Denominator: value.denominatorA}},
			Cardinality: cardinality, Outcomes: outcomes,
		},
		{
			Identity:    signature.Identity{Operation: secondOperation, Version: 1},
			Fence:       signature.Fence{Owner: value.owner, Schema: value.schema},
			Inputs:      []signature.Input{{Relation: value.relationA, Column: value.columnA, Type: value.typeID, Presence: signature.RequireOpaque, Delivery: delivery, Denominator: value.denominatorA}},
			Outputs:     []signature.Output{{Relation: value.relationB, Column: value.columnB, Type: secondType, Presence: signature.ProducePresent, Denominator: value.denominatorB}},
			Cardinality: cardinality, Outcomes: outcomes,
		},
	} {
		sealed, sealedOK := signature.Seal(specification)
		if !sealedOK || !builder.AddSignature(sealed) {
			t.Fatal("add signature")
		}
	}
	firstApply := algebra.NewApply([]algebra.Expression{algebra.NewInput(value.relationA)}, algebra.NewApplyContract(value.operation, []algebra.SlotSource{algebra.NewSlotSource(0, 0)}, algebra.OwnerNamed()))
	secondApply := algebra.NewApply([]algebra.Expression{algebra.NewInput(value.relationA)}, algebra.NewApplyContract(signature.Identity{Operation: secondOperation, Version: 1}, []algebra.SlotSource{algebra.NewSlotSource(0, 0)}, algebra.OwnerNamed()))
	firstExpression := algebra.NewPublish(firstApply, algebra.NewPublishContract(value.relationA, value.keyA))
	secondExpression := algebra.NewPublish(secondApply, algebra.NewPublishContract(value.relationB, value.keyB))
	firstID := issueExpression(t, value.owner, "distinct-first")
	secondID := issueExpression(t, value.owner, "distinct-second")
	if !builder.AddExpression(plan.DefineExpressionRef(firstID, firstExpression)) || !builder.AddExpression(plan.DefineExpressionRef(secondID, secondExpression)) {
		t.Fatal("add distinct output expressions")
	}
	schema, ok := builder.Build()
	if !ok {
		t.Fatal("build schema")
	}
	report := typing.Check(schema)
	if !report.Valid() {
		t.Fatalf("distinct-type schema rejected: %v", report.Error())
	}
	requirements := report.AlgebraRequirements()
	if len(requirements) != 2 {
		t.Fatalf("expected one requirement per distinct TypeID: %+v", requirements)
	}
	leftOwner, rightOwner := requirements[0].Owner().Content(), requirements[1].Owner().Content()
	leftContent, rightContent := requirements[0].Content(), requirements[1].Content()
	if comparison := bytes.Compare(leftOwner[:], rightOwner[:]); comparison > 0 || (comparison == 0 && bytes.Compare(leftContent[:], rightContent[:]) > 0) {
		t.Fatal("algebra requirements are not in canonical owner/content order")
	}
}

func TestNearestMutationIsRejectedForEveryOperator(t *testing.T) {
	value := newFixture(t)
	inputA := algebra.NewInput(value.relationA)
	inputB := algebra.NewInput(value.relationB)
	mutations := []struct {
		name       string
		expression algebra.Expression
		want       typing.Code
	}{
		{"Input-missing-relation", algebra.NewInput(issueRelation(t, value.owner, "missing")), typing.CodeMissingReference},
		{"Select-missing-scope", algebra.NewSelect(inputA, algebra.NewSelectContract(algebra.SelectByScope, issueScope(t, value.owner, "missing"))), typing.CodeMissingReference},
		{"Project-foreign-target", algebra.NewProject(inputA, algebra.NewProjectContract(issueRelation(t, value.owner, "missing"), []algebra.ColumnMapping{algebra.NewColumnMapping(value.columnA, value.columnB)}, value.keyB)), typing.CodeMissingReference},
		{"Join-arity", algebra.NewJoin(inputA, inputB, algebra.NewJoinContract(nil, []model.ColumnID{value.columnB})), typing.CodeOperatorContract},
		{"Merge-missing-key", algebra.NewMerge([]algebra.Expression{inputA, inputA}, algebra.NewMergeContract(issueKey(t, value.relationA, "missing"))), typing.CodeMissingReference},
		{"Group-invalid-cardinality", algebra.NewGroup(inputA, algebra.NewGroupContract(value.keyA, model.Cardinality{})), typing.CodeOperatorContract},
		{"Complete-foreign-denominator", algebra.NewComplete(inputA, issueDenominator(t, value.owner, "missing")), typing.CodeDenominatorMismatch},
		{"Apply-unknown-operation", algebra.NewApply([]algebra.Expression{inputA}, algebra.NewApplyContract(signature.Identity{Operation: issueOperation(t, value.owner, "missing"), Version: 1}, []algebra.SlotSource{algebra.NewSlotSource(0, 0)}, algebra.OwnerNamed())), typing.CodeSignatureMismatch},
		{"Publish-foreign-destination", algebra.NewPublish(inputB, algebra.NewPublishContract(issueRelation(t, value.owner, "missing"), value.keyB)), typing.CodeMissingReference},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			schema := validSchema(t, value, mutation.expression)
			report := typing.Check(schema)
			if report.Valid() {
				t.Fatalf("nearest mutation was accepted")
			}
			for _, issue := range report.Issues() {
				if issue.Code == mutation.want {
					return
				}
			}
			t.Fatalf("nearest mutation lacked code %d: %v", mutation.want, report.Issues())
		})
	}
}

func TestExactSchemaFenceAndTypeMembershipAreIndependentChecks(t *testing.T) {
	value := newFixture(t)
	inputA := algebra.NewInput(value.relationA)
	otherSchema := issueSchema(t, value.owner, "other")
	delivery, ok := signature.NewScalarDelivery()
	if !ok {
		t.Fatal("delivery")
	}
	accepted, ok := outcome.NewSet(outcome.Produced)
	if !ok {
		t.Fatal("outcomes")
	}
	foreignSignature, ok := signature.Seal(signature.Spec{
		Identity:    value.operation,
		Fence:       signature.Fence{Owner: value.owner, Schema: otherSchema},
		Inputs:      []signature.Input{{Relation: value.relationA, Column: value.columnA, Type: issueType(t, value.owner, "foreign-type"), Presence: signature.RequirePresent, Delivery: delivery, Denominator: value.denominatorA}},
		Outputs:     []signature.Output{{Relation: value.relationB, Column: value.columnB, Type: value.typeID, Presence: signature.ProducePresent, Denominator: value.denominatorB}},
		Cardinality: mustCardinality(t, model.ExactlyOne), Outcomes: accepted,
	})
	if !ok {
		t.Fatal("foreign signature")
	}
	// Build a schema with the malformed signature directly; no compiler-side
	// validation is allowed to erase the nearest mutation.
	schema := schemaWith(t, value, []algebra.Expression{inputA}, false)
	builder := plan.NewBuilder(value.schema)
	for _, relation := range schema.Relations() {
		_ = builder.AddRelation(relation)
	}
	for _, column := range schema.Columns() {
		_ = builder.AddColumn(column)
	}
	for _, key := range schema.Keys() {
		_ = builder.AddKey(key)
	}
	for _, scope := range schema.Scopes() {
		_ = builder.AddScope(scope)
	}
	for _, expression := range schema.Expressions() {
		_ = builder.AddExpression(expression)
	}
	_ = builder.AddSignature(foreignSignature)
	malformed, ok := builder.Build()
	if !ok {
		t.Fatal("malformed schema build")
	}
	report := typing.Check(malformed)
	if report.Valid() {
		t.Fatal("foreign schema/type signature accepted")
	}
	hasSchema, hasType := false, false
	for _, issue := range report.Issues() {
		hasSchema = hasSchema || issue.Code == typing.CodeSchemaIdentity
		hasType = hasType || issue.Code == typing.CodeTypeMismatch
	}
	if !hasSchema || !hasType {
		t.Fatalf("independent checks missing: schema=%v type=%v issues=%v", hasSchema, hasType, report.Issues())
	}
}

func mustCardinality(t *testing.T, kind model.CardinalityKind) model.Cardinality {
	t.Helper()
	value, ok := model.NewCardinality(kind, 0)
	if !ok {
		t.Fatalf("cardinality %v", kind)
	}
	return value
}

func issueOwner(t *testing.T, label string) model.OwnerID {
	t.Helper()
	value, ok := model.IssueOwnerID(token(t, "owner/"+label))
	if !ok {
		t.Fatal("owner")
	}
	return value
}
func issueSchema(t *testing.T, owner model.OwnerID, label string) model.SchemaID {
	t.Helper()
	value, ok := model.IssueSchemaID(owner, token(t, "schema/"+label))
	if !ok {
		t.Fatal("schema")
	}
	return value
}
func issueType(t *testing.T, owner model.OwnerID, label string) model.TypeID {
	t.Helper()
	value, ok := model.IssueTypeID(owner, token(t, "type/"+label))
	if !ok {
		t.Fatal("type")
	}
	return value
}
func issueRelation(t *testing.T, owner model.OwnerID, label string) model.RelationID {
	t.Helper()
	value, ok := model.IssueRelationID(owner, token(t, "relation/"+label))
	if !ok {
		t.Fatal("relation")
	}
	return value
}
func issueColumn(t *testing.T, relation model.RelationID, label string) model.ColumnID {
	t.Helper()
	value, ok := model.IssueColumnID(relation, token(t, "column/"+label))
	if !ok {
		t.Fatal("column")
	}
	return value
}
func issueKey(t *testing.T, relation model.RelationID, label string) model.KeyID {
	t.Helper()
	value, ok := model.IssueKeyID(relation, token(t, "key/"+label))
	if !ok {
		t.Fatal("key")
	}
	return value
}
func issueScope(t *testing.T, owner model.OwnerID, label string) model.ScopeID {
	t.Helper()
	value, ok := model.IssueScopeID(owner, token(t, "scope/"+label))
	if !ok {
		t.Fatal("scope")
	}
	return value
}
func issueOperation(t *testing.T, owner model.OwnerID, label string) model.OperationID {
	t.Helper()
	value, ok := model.IssueOperationID(owner, token(t, "operation/"+label))
	if !ok {
		t.Fatal("operation")
	}
	return value
}
func issueExpression(t *testing.T, owner model.OwnerID, label string) model.ExpressionID {
	t.Helper()
	value, ok := model.IssueExpressionID(owner, token(t, "expression/"+label))
	if !ok {
		t.Fatal("expression")
	}
	return value
}
func issueDenominator(t *testing.T, owner model.OwnerID, label string) model.DenominatorRef {
	t.Helper()
	relation := issueRelation(t, owner, "denominator/"+label)
	key := issueKey(t, relation, "key")
	value, ok := model.NewDenominatorRef(relation, key)
	if !ok {
		t.Fatal("denominator")
	}
	return value
}
func token(t *testing.T, label string) identity.ContentID {
	t.Helper()
	value, ok := identity.DeriveContentID("relation/check/typing/test", []byte(label))
	if !ok {
		t.Fatal("token")
	}
	return value
}
