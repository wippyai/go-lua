package typing_test

import (
	"bytes"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/check/typing"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
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
	return schemaWith(t, value, expressions, true)
}

func schemaWith(t *testing.T, value fixture, expressions []algebra.Expression, includeSignature bool) plan.ExecutionSchema {
	t.Helper()
	scope := model.DefineScopeSchema(value.scope, []model.ColumnID{value.columnA, value.columnB})
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
	for index, expression := range expressions {
		id := issueExpression(t, value.owner, string(rune('a'+index)))
		if !builder.AddExpression(plan.DefineExpressionRef(id, expression)) {
			t.Fatal("add expression")
		}
	}
	if includeSignature {
		delivery, ok := signature.NewScalarDelivery()
		if !ok {
			t.Fatal("scalar delivery")
		}
		accepted, ok := outcome.NewSet(outcome.Produced, outcome.NoCandidate, outcome.NoSelection, outcome.Refused)
		if !ok {
			t.Fatal("outcomes")
		}
		signatureValue, ok := signature.Seal(signature.Spec{
			Identity:    value.operation,
			Fence:       signature.Fence{Owner: value.owner, Schema: value.schema},
			Inputs:      []signature.Input{{Relation: value.relationA, Column: value.columnA, Type: value.typeID, Presence: signature.RequirePresent, Delivery: delivery, Denominator: value.denominatorA}},
			Outputs:     []signature.Output{{Relation: value.relationB, Column: value.columnB, Type: value.typeID, Presence: signature.ProducePresent}},
			Authority:   signature.OutputAuthority{Denominator: value.denominatorB},
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
	apply := algebra.NewApply([]algebra.Expression{inputA}, algebra.NewApplyContract(value.operation))
	publish := algebra.NewPublish(inputB, algebra.NewPublishContract(value.relationB, value.keyB))
	schema := validSchema(t, value, inputA, selectA, projectB, join, merge, group, complete, apply, publish)
	report := typing.Check(schema)
	if !report.Valid() {
		t.Fatalf("valid closed operator schema rejected: %v", report.Error())
	}
	if got := len(report.MergeRequirements()); got != 1 {
		t.Fatalf("Merge did not expose one TypeID obligation per output column: got %d", got)
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
		{"Apply-unknown-operation", algebra.NewApply([]algebra.Expression{inputA}, algebra.NewApplyContract(signature.Identity{Operation: issueOperation(t, value.owner, "missing"), Version: 1})), typing.CodeSignatureMismatch},
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
		Identity:  value.operation,
		Fence:     signature.Fence{Owner: value.owner, Schema: otherSchema},
		Inputs:    []signature.Input{{Relation: value.relationA, Column: value.columnA, Type: issueType(t, value.owner, "foreign-type"), Presence: signature.RequirePresent, Delivery: delivery, Denominator: value.denominatorA}},
		Outputs:   []signature.Output{{Relation: value.relationB, Column: value.columnB, Type: value.typeID, Presence: signature.ProducePresent}},
		Authority: signature.OutputAuthority{Denominator: value.denominatorB}, Cardinality: mustCardinality(t, model.ExactlyOne), Outcomes: accepted,
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

func TestARelationReadAtTwoCoordinatesJoinsWithItself(t *testing.T) {
	value := newFixture(t)
	left := algebra.NewInput(value.relationA)
	right := algebra.NewInput(value.relationA)
	selfJoin := algebra.NewJoin(left, right, algebra.NewJoinContract([]model.ColumnID{value.columnA}, []model.ColumnID{value.columnA}))
	schema := validSchema(t, value, selfJoin)
	report := typing.Check(schema)
	if !report.Valid() {
		t.Fatalf("a relation read at two coordinates cannot form a join result: %v", report.Error())
	}
}
