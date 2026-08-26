package recurrence_test

import (
	"bytes"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/check/recurrence"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/region"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

type fixture struct {
	owner       model.OwnerID
	schemaID    model.SchemaID
	relationA   model.RelationID
	relationB   model.RelationID
	dependencyA model.DependencyID
	dependencyB model.DependencyID
	expressionA model.ExpressionID
	expressionB model.ExpressionID
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	owner := issueOwner(t, "owner")
	return fixture{
		owner:       owner,
		schemaID:    issueSchema(t, owner, "schema"),
		relationA:   issueRelation(t, owner, "relation-a"),
		relationB:   issueRelation(t, owner, "relation-b"),
		dependencyA: issueDependency(t, owner, "dependency-a"),
		dependencyB: issueDependency(t, owner, "dependency-b"),
		expressionA: issueExpression(t, owner, "expression-a"),
		expressionB: issueExpression(t, owner, "expression-b"),
	}
}

func validSchema(t *testing.T, value fixture) plan.ExecutionSchema {
	t.Helper()
	refA, _ := plan.NewRelationRef(value.relationA)
	refB, _ := plan.NewRelationRef(value.relationB)
	dependencyRefA := plan.DefineDependencyRef(value.dependencyA)
	dependencyRefB := plan.DefineDependencyRef(value.dependencyB)
	expressionA := plan.DefineExpressionRef(value.expressionA, algebra.NewPublish(
		algebra.NewInput(value.relationB), algebra.NewPublishContract(value.relationA, model.KeyID{})))
	expressionB := plan.DefineExpressionRef(value.expressionB, algebra.NewPublish(
		algebra.NewInput(value.relationA), algebra.NewPublishContract(value.relationB, model.KeyID{})))
	dependencyA := plan.DefineDependency(value.dependencyA, value.expressionA, []plan.RelationRef{refB}, []plan.RelationRef{refA}, "a")
	dependencyB := plan.DefineDependency(value.dependencyB, value.expressionB, []plan.RelationRef{refA}, []plan.RelationRef{refB}, "b")
	edgeAB := plan.DefineDependencyEdge(dependencyRefA, dependencyRefB)
	edgeBA := plan.DefineDependencyEdge(dependencyRefB, dependencyRefA)
	headA := plan.DefineWideningHead(dependencyRefA, refA)
	headB := plan.DefineWideningHead(dependencyRefB, refB)
	recurrencePolicy := plan.DefineRecurrence(plan.Positive, []plan.WideningHead{headA, headB})
	component := plan.DefineSCC([]plan.DependencyRef{dependencyRefA, dependencyRefB}, []plan.DependencyEdge{edgeAB, edgeBA}, recurrencePolicy)
	builder := plan.NewBuilder(value.schemaID)
	for _, entry := range []plan.ExpressionRef{expressionA, expressionB} {
		if !builder.AddExpression(entry) {
			t.Fatal("add expression")
		}
	}
	for _, entry := range []plan.Dependency{dependencyA, dependencyB} {
		if !builder.AddDependency(entry) {
			t.Fatal("add dependency")
		}
	}
	if !builder.AddSCC(component) {
		t.Fatal("add component")
	}
	schema, ok := builder.Build()
	if !ok {
		t.Fatal("build schema")
	}
	return schema
}

func TestCheckDerivesPositiveCycleAndCopiesProof(t *testing.T) {
	value := newFixture(t)
	proof, err := recurrence.Check(validSchema(t, value))
	if err != nil {
		t.Fatalf("valid recurrence rejected: %v", err)
	}
	if len(proof.Projections()) != 2 || len(proof.Edges()) != 2 || len(proof.Components()) != 1 {
		t.Fatalf("unexpected proof shape: projections=%d edges=%d components=%d", len(proof.Projections()), len(proof.Edges()), len(proof.Components()))
	}
	heads := proof.WideningHeads()
	if len(heads) != 2 {
		t.Fatalf("unexpected widening-head shape: %d", len(heads))
	}
	if heads[0].Digest() == heads[1].Digest() || !heads[0].Available() || !heads[1].Available() {
		t.Fatal("proof did not expose validated widening heads")
	}
	firstDigest, secondDigest := heads[0].Digest(), heads[1].Digest()
	if bytes.Compare(firstDigest[:], secondDigest[:]) >= 0 {
		t.Fatal("widening heads are not in canonical digest order")
	}
	firstHead := heads[0]
	headDigest := firstHead.Digest()
	heads[0] = plan.WideningHead{}
	if proof.WideningHeads()[0].Digest() != headDigest {
		t.Fatal("proof exposed mutable widening-head storage")
	}
	projections := proof.Projections()
	projections[0].Reads[0] = plan.RelationRef{}
	if len(proof.Projections()[0].Reads) == 0 || !proof.Projections()[0].Reads[0].Available() {
		t.Fatal("proof exposed mutable projection storage")
	}
	components := proof.Components()
	components[0].Members[0] = plan.DependencyRef{}
	if !proof.Components()[0].Members[0].Available() {
		t.Fatal("proof exposed mutable component storage")
	}
}

func TestCheckRejectsNearestProjectionMutation(t *testing.T) {
	value := newFixture(t)
	schema := validSchema(t, value)
	builder := plan.NewBuilder(schema.SchemaID())
	expressionA := plan.DefineExpressionRef(value.expressionA, algebra.NewInput(value.relationB))
	refA, _ := plan.NewRelationRef(value.relationA)
	refB, _ := plan.NewRelationRef(value.relationB)
	dependencyA := plan.DefineDependency(value.dependencyA, value.expressionA, []plan.RelationRef{refB}, []plan.RelationRef{refA}, "a")
	dependencyB := plan.DefineDependency(value.dependencyB, value.expressionB, []plan.RelationRef{refA}, []plan.RelationRef{refB}, "b")
	for _, entry := range []plan.ExpressionRef{expressionA, validExpressionB(value)} {
		if !builder.AddExpression(entry) {
			t.Fatal("add expression")
		}
	}
	for _, entry := range []plan.Dependency{dependencyA, dependencyB} {
		if !builder.AddDependency(entry) {
			t.Fatal("add dependency")
		}
	}
	refDependencyA := plan.DefineDependencyRef(value.dependencyA)
	refDependencyB := plan.DefineDependencyRef(value.dependencyB)
	if !builder.AddSCC(plan.DefineSCC(
		[]plan.DependencyRef{refDependencyA, refDependencyB},
		[]plan.DependencyEdge{
			plan.DefineDependencyEdge(refDependencyA, refDependencyB),
			plan.DefineDependencyEdge(refDependencyB, refDependencyA),
		},
		plan.DefineRecurrence(plan.Positive, []plan.WideningHead{
			plan.DefineWideningHead(refDependencyA, refA), plan.DefineWideningHead(refDependencyB, refB),
		}),
	)) {
		t.Fatal("add component")
	}
	mutated, ok := builder.Build()
	if !ok {
		t.Fatal("build mutated schema")
	}
	_, err := recurrence.Check(mutated)
	if err == nil {
		t.Fatal("nearest expression mutation was accepted")
	}
	refusal, ok := err.(*recurrence.Error)
	if !ok || refusal.Code != recurrence.CodeProjectionMismatch {
		t.Fatalf("wrong refusal: %T %v", err, err)
	}
	_ = schema
}

func TestCheckRejectsMissingInternalEdge(t *testing.T) {
	value := newFixture(t)
	schema := validSchema(t, value)
	// Rebuild the same declarations with only one internal edge.  This is a
	// nearest graph mutation: expression projections remain canonical.
	refA, _ := plan.NewRelationRef(value.relationA)
	refB, _ := plan.NewRelationRef(value.relationB)
	dependencyRefA := plan.DefineDependencyRef(value.dependencyA)
	dependencyRefB := plan.DefineDependencyRef(value.dependencyB)
	builder := plan.NewBuilder(schema.SchemaID())
	if !builder.AddExpression(validExpressionA(value)) || !builder.AddExpression(validExpressionB(value)) {
		t.Fatal("add expressions")
	}
	if !builder.AddDependency(plan.DefineDependency(value.dependencyA, value.expressionA, []plan.RelationRef{refB}, []plan.RelationRef{refA}, "a")) ||
		!builder.AddDependency(plan.DefineDependency(value.dependencyB, value.expressionB, []plan.RelationRef{refA}, []plan.RelationRef{refB}, "b")) {
		t.Fatal("add dependencies")
	}
	if !builder.AddSCC(plan.DefineSCC(
		[]plan.DependencyRef{dependencyRefA, dependencyRefB},
		[]plan.DependencyEdge{plan.DefineDependencyEdge(dependencyRefA, dependencyRefB)},
		plan.DefineRecurrence(plan.Positive, []plan.WideningHead{
			plan.DefineWideningHead(dependencyRefA, refA), plan.DefineWideningHead(dependencyRefB, refB),
		}),
	)) {
		t.Fatal("add component")
	}
	mutated, ok := builder.Build()
	if !ok {
		t.Fatal("build edge mutation")
	}
	_, err := recurrence.Check(mutated)
	if err == nil {
		t.Fatal("missing edge was accepted")
	}
	refusal, ok := err.(*recurrence.Error)
	if !ok || refusal.Code != recurrence.CodeMissingEdge {
		t.Fatalf("wrong refusal: %T %v", err, err)
	}
}

func TestCheckProjectsStateOnlyAtPublishBoundary(t *testing.T) {
	owner := issueOwner(t, "state-boundary-owner")
	schemaID := issueSchema(t, owner, "state-boundary-schema")
	source := issueRelation(t, owner, "state-boundary-source")
	target := issueRelation(t, owner, "state-boundary-target")
	sourceColumn := issueColumn(t, source, "state-boundary-source-column")
	targetColumn := issueColumn(t, target, "state-boundary-target-column")
	sourceKey := issueKey(t, source, "state-boundary-source-key")
	targetKey := issueKey(t, target, "state-boundary-target-key")
	sourceScope := issueScope(t, owner, "state-boundary-source-scope")
	targetScope := issueScope(t, owner, "state-boundary-target-scope")
	typeID := issueType(t, owner, "state-boundary-type")
	operation := issueOperation(t, owner, "state-boundary-operation")
	applyExpressionID := issueExpression(t, owner, "state-boundary-apply")
	projectExpressionID := issueExpression(t, owner, "state-boundary-project")
	publishExpressionID := issueExpression(t, owner, "state-boundary-publish")
	applyDependencyID := issueDependency(t, owner, "state-boundary-apply-dependency")
	projectDependencyID := issueDependency(t, owner, "state-boundary-project-dependency")
	publishDependencyID := issueDependency(t, owner, "state-boundary-publish-dependency")
	sourceRef, _ := plan.NewRelationRef(source)
	targetRef, _ := plan.NewRelationRef(target)
	sourceDenominator, ok := model.NewDenominatorRef(source, sourceKey)
	if !ok {
		t.Fatal("source denominator")
	}
	targetDenominator, ok := model.NewDenominatorRef(target, targetKey)
	if !ok {
		t.Fatal("target denominator")
	}
	delivery, ok := signature.NewScalarDelivery()
	if !ok {
		t.Fatal("scalar delivery")
	}
	outcomes, ok := outcome.NewSet(outcome.Produced, outcome.NoSelection, outcome.Refused)
	if !ok {
		t.Fatal("outcomes")
	}
	cardinality, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("cardinality")
	}
	operationSignature, ok := signature.Seal(signature.Spec{
		Identity:    signature.Identity{Operation: operation, Version: 1},
		Fence:       signature.Fence{Owner: owner, Schema: schemaID},
		Inputs:      []signature.Input{{Relation: source, Column: sourceColumn, Type: typeID, Presence: signature.RequirePresent, Delivery: delivery, Denominator: sourceDenominator}},
		Outputs:     []signature.Output{{Relation: target, Column: targetColumn, Type: typeID, Presence: signature.ProducePresent, Denominator: targetDenominator}},
		Cardinality: cardinality, Outcomes: outcomes,
	})
	if !ok {
		t.Fatal("operation signature")
	}
	applyExpression := plan.DefineExpressionRef(applyExpressionID, algebra.NewApply(
		[]algebra.Expression{algebra.NewInput(source)},
		algebra.NewApplyContract(operationSignature.Identity(), []algebra.SlotSource{algebra.NewSlotSource(0, 0)}, algebra.OwnerNamed()),
	))
	projectExpression := plan.DefineExpressionRef(projectExpressionID, algebra.NewProject(
		algebra.NewInput(source),
		algebra.NewProjectContract(target, []algebra.ColumnMapping{algebra.NewColumnMapping(sourceColumn, targetColumn)}, targetKey),
	))
	publishExpression := plan.DefineExpressionRef(publishExpressionID, algebra.NewPublish(
		algebra.NewApply([]algebra.Expression{algebra.NewInput(source)}, algebra.NewApplyContract(
			operationSignature.Identity(), []algebra.SlotSource{algebra.NewSlotSource(0, 0)}, algebra.OwnerNamed())),
		algebra.NewPublishContract(target, targetKey),
	))
	applyDependency := plan.DefineDependency(applyDependencyID, applyExpressionID, []plan.RelationRef{sourceRef}, nil, "state-boundary-apply")
	projectDependency := plan.DefineDependency(projectDependencyID, projectExpressionID, []plan.RelationRef{sourceRef, targetRef}, nil, "state-boundary-project")
	publishDependency := plan.DefineDependency(publishDependencyID, publishExpressionID, []plan.RelationRef{sourceRef}, []plan.RelationRef{targetRef}, "state-boundary-publish")
	applyDependencyRef := plan.DefineDependencyRef(applyDependencyID)
	projectDependencyRef := plan.DefineDependencyRef(projectDependencyID)
	publishDependencyRef := plan.DefineDependencyRef(publishDependencyID)
	applySCC := plan.DefineSCC([]plan.DependencyRef{applyDependencyRef}, nil, plan.DefineRecurrence(plan.Acyclic, nil))
	projectSCC := plan.DefineSCC([]plan.DependencyRef{projectDependencyRef}, nil, plan.DefineRecurrence(plan.Acyclic, nil))
	publishSCC := plan.DefineSCC([]plan.DependencyRef{publishDependencyRef}, nil, plan.DefineRecurrence(plan.Acyclic, nil))
	builder := plan.NewBuilder(schemaID)
	if !builder.AddRelation(model.DefineRelationSchema(source, []model.ColumnID{sourceColumn}, []model.KeyID{sourceKey}, sourceScope)) ||
		!builder.AddRelation(model.DefineRelationSchema(target, []model.ColumnID{targetColumn}, []model.KeyID{targetKey}, targetScope)) ||
		!builder.AddColumn(model.DefineColumnSchema(sourceColumn, typeID)) || !builder.AddColumn(model.DefineColumnSchema(targetColumn, typeID)) ||
		!builder.AddKey(model.DefineKeySchema(sourceKey, []model.ColumnID{sourceColumn})) || !builder.AddKey(model.DefineKeySchema(targetKey, []model.ColumnID{targetColumn})) ||
		!builder.AddScope(model.DefineScopeSchema(sourceScope, nil, region.True())) || !builder.AddScope(model.DefineScopeSchema(targetScope, nil, region.True())) ||
		!builder.AddExpression(applyExpression) || !builder.AddExpression(projectExpression) || !builder.AddExpression(publishExpression) ||
		!builder.AddDependency(applyDependency) || !builder.AddDependency(projectDependency) || !builder.AddDependency(publishDependency) ||
		!builder.AddSCC(applySCC) || !builder.AddSCC(projectSCC) || !builder.AddSCC(publishSCC) || !builder.AddSignature(operationSignature) {
		t.Fatal("state-boundary declarations")
	}
	schema, ok := builder.Build()
	if !ok {
		t.Fatal("state-boundary schema")
	}
	proof, err := recurrence.Check(schema)
	if err != nil || !proof.Available() {
		t.Fatalf("state-boundary recurrence refused: %v", err)
	}
	projections := proof.Projections()
	if len(projections) != 3 {
		t.Fatalf("projection count = %d", len(projections))
	}
	for _, projection := range projections {
		switch projection.Dependency {
		case applyDependencyID:
			if !sameRelationIDs(projection.Reads, []model.RelationID{source}) || len(projection.Writes) != 0 {
				t.Fatalf("Apply projection = %#v, want read source and no writes", projection)
			}
		case projectDependencyID:
			if !sameRelationIDs(projection.Reads, []model.RelationID{source, target}) || len(projection.Writes) != 0 {
				t.Fatalf("Project projection = %#v, want read source+target and no writes", projection)
			}
		case publishDependencyID:
			if !sameRelationIDs(projection.Reads, []model.RelationID{source}) || !sameRelationIDs(projection.Writes, []model.RelationID{target}) {
				t.Fatalf("Publish projection = %#v, want read source and write target", projection)
			}
		default:
			t.Fatalf("unknown projection dependency %v", projection.Dependency)
		}
	}
}

func sameRelationIDs(refs []plan.RelationRef, want []model.RelationID) bool {
	if len(refs) != len(want) {
		return false
	}
	seen := make(map[model.RelationID]struct{}, len(refs))
	for _, ref := range refs {
		seen[ref.ID()] = struct{}{}
	}
	for _, id := range want {
		if _, ok := seen[id]; !ok {
			return false
		}
	}
	return true
}

func validExpressionA(value fixture) plan.ExpressionRef {
	return plan.DefineExpressionRef(value.expressionA, algebra.NewPublish(algebra.NewInput(value.relationB), algebra.NewPublishContract(value.relationA, model.KeyID{})))
}

func validExpressionB(value fixture) plan.ExpressionRef {
	return plan.DefineExpressionRef(value.expressionB, algebra.NewPublish(algebra.NewInput(value.relationA), algebra.NewPublishContract(value.relationB, model.KeyID{})))
}

func issueOwner(t *testing.T, label string) model.OwnerID {
	t.Helper()
	owner, ok := model.IssueOwnerID(token(t, label))
	if !ok {
		t.Fatal("issue owner")
	}
	return owner
}

func issueSchema(t *testing.T, owner model.OwnerID, label string) model.SchemaID {
	t.Helper()
	id, ok := model.IssueSchemaID(owner, token(t, label))
	if !ok {
		t.Fatal("issue schema")
	}
	return id
}

func issueRelation(t *testing.T, owner model.OwnerID, label string) model.RelationID {
	t.Helper()
	id, ok := model.IssueRelationID(owner, token(t, label))
	if !ok {
		t.Fatal("issue relation")
	}
	return id
}

func issueDependency(t *testing.T, owner model.OwnerID, label string) model.DependencyID {
	t.Helper()
	id, ok := model.IssueDependencyID(owner, token(t, label))
	if !ok {
		t.Fatal("issue dependency")
	}
	return id
}

func issueExpression(t *testing.T, owner model.OwnerID, label string) model.ExpressionID {
	t.Helper()
	id, ok := model.IssueExpressionID(owner, token(t, label))
	if !ok {
		t.Fatal("issue expression")
	}
	return id
}

func issueColumn(t *testing.T, relation model.RelationID, label string) model.ColumnID {
	t.Helper()
	id, ok := model.IssueColumnID(relation, token(t, label))
	if !ok {
		t.Fatal("issue column")
	}
	return id
}

func issueKey(t *testing.T, relation model.RelationID, label string) model.KeyID {
	t.Helper()
	id, ok := model.IssueKeyID(relation, token(t, label))
	if !ok {
		t.Fatal("issue key")
	}
	return id
}

func issueScope(t *testing.T, owner model.OwnerID, label string) model.ScopeID {
	t.Helper()
	id, ok := model.IssueScopeID(owner, token(t, label))
	if !ok {
		t.Fatal("issue scope")
	}
	return id
}

func issueType(t *testing.T, owner model.OwnerID, label string) model.TypeID {
	t.Helper()
	id, ok := model.IssueTypeID(owner, token(t, label))
	if !ok {
		t.Fatal("issue type")
	}
	return id
}

func issueOperation(t *testing.T, owner model.OwnerID, label string) model.OperationID {
	t.Helper()
	id, ok := model.IssueOperationID(owner, token(t, label))
	if !ok {
		t.Fatal("issue operation")
	}
	return id
}

func token(t *testing.T, label string) identity.ContentID {
	t.Helper()
	value, ok := identity.DeriveContentID("relation/check/recurrence/test/v1", []byte(label))
	if !ok {
		t.Fatal("derive token")
	}
	return value
}
