package relcompile

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

type relationFixture struct {
	owner  model.OwnerID
	schema model.SchemaID
	scope  model.ScopeSchema
	decl   Declaration
}

func newRelationFixture(t *testing.T) relationFixture {
	t.Helper()
	owner := issueOwner(t, "owner")
	schemaID, ok := model.IssueSchemaID(owner, token(t, "schema"))
	if !ok {
		t.Fatal("issue schema")
	}
	scopeID, ok := model.IssueScopeID(owner, token(t, "scope"))
	if !ok {
		t.Fatal("issue scope")
	}
	return relationFixture{
		owner:  owner,
		schema: schemaID,
		scope:  model.DefineScopeSchema(scopeID, nil),
		decl:   Declaration{SchemaID: schemaID},
	}
}

func (fixture *relationFixture) addRelation(t *testing.T, label string, columnLabels ...string) (model.RelationID, []model.ColumnID, model.KeyID) {
	t.Helper()
	relation, ok := model.IssueRelationID(fixture.owner, token(t, "relation/"+label))
	if !ok {
		t.Fatalf("issue relation %s", label)
	}
	columns := make([]model.ColumnID, 0, len(columnLabels))
	columnSchemas := make([]model.ColumnSchema, 0, len(columnLabels))
	for _, columnLabel := range columnLabels {
		column, columnOK := model.IssueColumnID(relation, token(t, "column/"+label+"/"+columnLabel))
		if !columnOK {
			t.Fatalf("issue column %s/%s", label, columnLabel)
		}
		typeID, typeOK := model.IssueTypeID(fixture.owner, token(t, "type/"+label+"/"+columnLabel))
		if !typeOK {
			t.Fatalf("issue type %s/%s", label, columnLabel)
		}
		columns = append(columns, column)
		columnSchemas = append(columnSchemas, model.DefineColumnSchema(column, typeID))
	}
	key, ok := model.IssueKeyID(relation, token(t, "key/"+label))
	if !ok {
		t.Fatalf("issue key %s", label)
	}
	fixture.decl.Relations = append(fixture.decl.Relations, model.DefineRelationSchema(relation, columns, []model.KeyID{key}, fixture.scope.ID()))
	fixture.decl.Columns = append(fixture.decl.Columns, columnSchemas...)
	fixture.decl.Keys = append(fixture.decl.Keys, model.DefineKeySchema(key, columns))
	return relation, columns, key
}

func (fixture relationFixture) addSignature(t *testing.T, operationLabel string, operation model.OperationID, input model.RelationID, inputColumn model.ColumnID, output model.RelationID, outputColumn model.ColumnID, denominator model.DenominatorRef, cardinality model.Cardinality, codes ...outcome.Code) signature.Signature {
	t.Helper()
	accepted, ok := outcome.NewSet(codes...)
	if !ok {
		t.Fatal("construct outcome set")
	}
	delivery, ok := signature.NewScalarDelivery()
	if !ok {
		t.Fatal("construct scalar delivery")
	}
	owner := fixture.owner
	typeID, ok := model.IssueTypeID(owner, token(t, "signature-type/"+operationLabel))
	if !ok {
		t.Fatal("issue signature type")
	}
	value := signature.Spec{
		Identity: signature.Identity{Operation: operation, Version: 1},
		Fence:    signature.Fence{Owner: owner, Schema: fixture.schema},
		Inputs: []signature.Input{{
			Relation: input, Column: inputColumn, Type: typeID,
			Presence: signature.AllowMissing, Delivery: delivery, Denominator: denominator,
		}},
		Outputs: []signature.Output{{
			Relation: output, Column: outputColumn, Type: typeID, Presence: signature.ProduceOptional,
		}},
		Authority:   signature.OutputAuthority{Denominator: denominator},
		Cardinality: cardinality, Outcomes: accepted,
	}
	sealed, ok := signature.Seal(value)
	if !ok {
		t.Fatal("seal signature")
	}
	return sealed
}

func issueOwner(t *testing.T, label string) model.OwnerID {
	t.Helper()
	owner, ok := model.IssueOwnerID(token(t, "owner/"+label))
	if !ok {
		t.Fatal("issue owner")
	}
	return owner
}

func token(t *testing.T, label string) identity.ContentID {
	t.Helper()
	value, ok := identity.DeriveContentID("relcompile-specimen/v1", []byte(label))
	if !ok {
		t.Fatalf("derive token %q", label)
	}
	return value
}

func compileFixture(t *testing.T, fixture *relationFixture) plan.ExecutionSchema {
	t.Helper()
	compiled, err := Compile(fixture.decl)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return compiled
}

func findExpression(t *testing.T, compiled plan.ExecutionSchema, id model.ExpressionID) algebra.Expression {
	t.Helper()
	for _, entry := range compiled.Expressions() {
		if entry.ID() == id {
			return entry.Expression()
		}
	}
	t.Fatalf("expression %v not found", id)
	return nil
}

func expressionID(t *testing.T, owner model.OwnerID, label string) model.ExpressionID {
	t.Helper()
	id, ok := model.IssueExpressionID(owner, token(t, "expression/"+label))
	if !ok {
		t.Fatal("issue expression")
	}
	return id
}

func dependencyID(t *testing.T, owner model.OwnerID, label string) model.DependencyID {
	t.Helper()
	id, ok := model.IssueDependencyID(owner, token(t, "dependency/"+label))
	if !ok {
		t.Fatal("issue dependency")
	}
	return id
}

func operationID(t *testing.T, owner model.OwnerID, label string) model.OperationID {
	t.Helper()
	id, ok := model.IssueOperationID(owner, token(t, "operation/"+label))
	if !ok {
		t.Fatal("issue operation")
	}
	return id
}

func TestHeapFormalFreezeParentPredicateLowerToJoins(t *testing.T) {
	fixture := newRelationFixture(t)
	heap, heapColumns, heapKey := fixture.addRelation(t, "heap", "parent", "predicate")
	parent, parentColumns, _ := fixture.addRelation(t, "parent", "identity")
	predicate, predicateColumns, _ := fixture.addRelation(t, "predicate", "identity")
	freeze, freezeColumns, freezeKey := fixture.addRelation(t, "formal-freeze", "value")
	operation := operationID(t, fixture.owner, "formal-freeze")
	semantic := fixture.addSignature(t, "formal-freeze", operation, heap, heapColumns[0], freeze, freezeColumns[0], denominator(t, heap, heapKey), exactCardinality(t), outcome.Produced, outcome.NoCandidate, outcome.Refused)
	fixture.decl.Signatures = append(fixture.decl.Signatures, semantic)
	root := expressionID(t, fixture.owner, "formal-freeze")
	fixture.decl.Rules = append(fixture.decl.Rules, Rule{
		ID: dependencyID(t, fixture.owner, "formal-freeze"), Expression: root, Candidate: heap,
		Joins: []JoinSpec{
			{Relation: parent, LeftColumns: []model.ColumnID{heapColumns[0]}, RightColumns: []model.ColumnID{parentColumns[0]}},
			{Relation: predicate, LeftColumns: []model.ColumnID{heapColumns[1]}, RightColumns: []model.ColumnID{predicateColumns[0]}},
		},
		Apply:   signature.Identity{Operation: operation, Version: 1},
		Publish: &Publication{Relation: freeze, Key: freezeKey},
	})

	compiled := compileFixture(t, &fixture)
	expression := findExpression(t, compiled, root)
	if _, ok := expression.(algebra.Publish); !ok {
		t.Fatalf("formal-freeze root = %T, want Publish", expression)
	}
	if got := countJoins(expression); got != 2 {
		t.Fatalf("formal-freeze joins = %d, want parent+predicate", got)
	}
	if len(compiled.Dependencies()) != 1 || len(compiled.Dependencies()[0].Reads()) != 3 {
		t.Fatal("parent and predicate were not retained as relation reads")
	}
}

func TestPlacementCaptureSourceTagRouteUsesSameJoinAndPublish(t *testing.T) {
	fixture := newRelationFixture(t)
	capture, captureColumns, captureKey := fixture.addRelation(t, "capture", "source", "tag")
	source, sourceColumns, _ := fixture.addRelation(t, "source", "identity")
	tag, tagColumns, _ := fixture.addRelation(t, "tag", "identity")
	placement, placementColumns, placementKey := fixture.addRelation(t, "placement", "stack")
	operation := operationID(t, fixture.owner, "capture-route")
	semantic := fixture.addSignature(t, "capture-route", operation, capture, captureColumns[0], placement, placementColumns[0], denominator(t, capture, captureKey), exactCardinality(t), outcome.Produced, outcome.NoSelection, outcome.Refused)
	fixture.decl.Signatures = append(fixture.decl.Signatures, semantic)
	root := expressionID(t, fixture.owner, "capture-route")
	fixture.decl.Rules = append(fixture.decl.Rules, Rule{
		ID: dependencyID(t, fixture.owner, "capture-route"), Expression: root, Candidate: capture,
		Joins: []JoinSpec{
			{Relation: source, LeftColumns: []model.ColumnID{captureColumns[0]}, RightColumns: []model.ColumnID{sourceColumns[0]}},
			{Relation: tag, LeftColumns: []model.ColumnID{captureColumns[1]}, RightColumns: []model.ColumnID{tagColumns[0]}},
		},
		Apply:   signature.Identity{Operation: operation, Version: 1},
		Publish: &Publication{Relation: placement, Key: placementKey},
	})

	compiled := compileFixture(t, &fixture)
	expression := findExpression(t, compiled, root)
	if _, ok := expression.(algebra.Publish); !ok {
		t.Fatalf("capture route root = %T, want Publish", expression)
	}
	if got := countJoins(expression); got != 2 {
		t.Fatalf("capture route joins = %d, want source+tag", got)
	}
}

func TestCallActivationSixAxisTransportUsesOutputCardinalityAndJoins(t *testing.T) {
	fixture := newRelationFixture(t)
	activation, activationColumns, activationKey := fixture.addRelation(t, "activation", "axis-0", "axis-1", "axis-2", "axis-3", "axis-4", "axis-5")
	axisRelations := make([]model.RelationID, 0, 6)
	axisColumns := make([]model.ColumnID, 0, 6)
	for index := 0; index < 6; index++ {
		relation, columns, _ := fixture.addRelation(t, "transport-axis"+string(rune('0'+index)), "identity")
		axisRelations = append(axisRelations, relation)
		axisColumns = append(axisColumns, columns[0])
	}
	activationOut, activationOutColumns, activationOutKey := fixture.addRelation(t, "activation-result", "result")
	operation := operationID(t, fixture.owner, "call-activation-expansion")
	semantic := fixture.addSignature(t, "call-activation", operation, activation, activationColumns[0], activationOut, activationOutColumns[0], denominator(t, activation, activationKey), boundedCardinality(t, 6), outcome.Produced, outcome.NoSelection, outcome.Opaque, outcome.Refused)
	fixture.decl.Signatures = append(fixture.decl.Signatures, semantic)
	root := expressionID(t, fixture.owner, "call-activation")
	joins := make([]JoinSpec, 0, 6)
	for index, relation := range axisRelations {
		joins = append(joins, JoinSpec{Relation: relation, LeftColumns: []model.ColumnID{activationColumns[index]}, RightColumns: []model.ColumnID{axisColumns[index]}})
	}
	fixture.decl.Rules = append(fixture.decl.Rules, Rule{
		ID: dependencyID(t, fixture.owner, "call-activation"), Expression: root, Candidate: activation,
		Joins: joins, Apply: signature.Identity{Operation: operation, Version: 1},
		Publish: &Publication{Relation: activationOut, Key: activationOutKey},
	})

	compiled := compileFixture(t, &fixture)
	expression := findExpression(t, compiled, root)
	if got := countJoins(expression); got != 6 {
		t.Fatalf("activation transport joins = %d, want six", got)
	}
	if _, ok := expression.(algebra.Publish); !ok {
		t.Fatalf("activation root = %T, want Publish", expression)
	}
	if !compiled.Signatures()[0].Allows(outcome.Opaque) || !compiled.Signatures()[0].Allows(outcome.Refused) {
		t.Fatal("opaque/refused outcomes were not retained by the activation signature")
	}
}

func TestAbsenceMappingKeepsCompleteAndSparseDistinct(t *testing.T) {
	fixture := newRelationFixture(t)
	input, inputColumns, inputKey := fixture.addRelation(t, "input", "value")
	output, outputColumns, outputKey := fixture.addRelation(t, "output", "value")
	operation := operationID(t, fixture.owner, "absence")
	semantic := fixture.addSignature(t, "absence", operation, input, inputColumns[0], output, outputColumns[0], denominator(t, input, inputKey), exactCardinality(t), outcome.Produced, outcome.NoCandidate, outcome.NoSelection, outcome.Opaque, outcome.Refused)
	fixture.decl.Signatures = append(fixture.decl.Signatures, semantic)
	rootComplete := expressionID(t, fixture.owner, "complete")
	rootSparse := expressionID(t, fixture.owner, "sparse")
	completeDenominator := denominator(t, input, inputKey)
	fixture.decl.Rules = append(fixture.decl.Rules,
		Rule{ID: dependencyID(t, fixture.owner, "complete"), Expression: rootComplete, Candidate: input, Complete: &completeDenominator, Apply: signature.Identity{Operation: operation, Version: 1}, Publish: &Publication{Relation: output, Key: outputKey}},
		Rule{ID: dependencyID(t, fixture.owner, "sparse"), Expression: rootSparse, Candidate: input, Apply: signature.Identity{Operation: operation, Version: 1}, Publish: &Publication{Relation: output, Key: outputKey}},
	)
	compiled := compileFixture(t, &fixture)
	completeExpr := findExpression(t, compiled, rootComplete)
	sparseExpr := findExpression(t, compiled, rootSparse)
	if !containsKind(completeExpr, algebra.KindComplete) {
		t.Fatal("complete delivery did not materialize Complete")
	}
	if containsKind(sparseExpr, algebra.KindComplete) {
		t.Fatal("sparse delivery fabricated Complete")
	}
	if !compiled.Signatures()[0].Allows(outcome.NoCandidate) || !compiled.Signatures()[0].Allows(outcome.NoSelection) || !compiled.Signatures()[0].Allows(outcome.Opaque) {
		t.Fatal("absence and opaque outcome vocabulary was not explicit in the signature")
	}
}

func denominator(t *testing.T, relation model.RelationID, key model.KeyID) model.DenominatorRef {
	t.Helper()
	value, ok := model.NewDenominatorRef(relation, key)
	if !ok {
		t.Fatal("construct denominator")
	}
	return value
}

func exactCardinality(t *testing.T) model.Cardinality {
	t.Helper()
	value, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("construct exact cardinality")
	}
	return value
}

func boundedCardinality(t *testing.T, bound uint32) model.Cardinality {
	t.Helper()
	value, ok := model.NewCardinality(model.BoundedMany, bound)
	if !ok {
		t.Fatal("construct bounded cardinality")
	}
	return value
}

func countJoins(expression algebra.Expression) int {
	count := 0
	for expression != nil {
		join, ok := expression.(algebra.Join)
		if !ok {
			if publish, isPublish := expression.(algebra.Publish); isPublish {
				expression = publish.Child()
				continue
			}
			if apply, isApply := expression.(algebra.Apply); isApply {
				inputs := apply.Inputs()
				if len(inputs) == 1 {
					expression = inputs[0]
					continue
				}
			}
			return count
		}
		count++
		expression = join.Left()
	}
	return count
}

func containsKind(expression algebra.Expression, kind algebra.Kind) bool {
	if expression == nil {
		return false
	}
	if expression.Kind() == kind {
		return true
	}
	switch value := expression.(type) {
	case algebra.Publish:
		return containsKind(value.Child(), kind)
	case algebra.Apply:
		for _, input := range value.Inputs() {
			if containsKind(input, kind) {
				return true
			}
		}
	case algebra.Join:
		return containsKind(value.Left(), kind) || containsKind(value.Right(), kind)
	case algebra.Select:
		return containsKind(value.Child(), kind)
	case algebra.Complete:
		return containsKind(value.Child(), kind)
	}
	return false
}
