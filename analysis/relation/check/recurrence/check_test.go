package recurrence_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/check/recurrence"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
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

func token(t *testing.T, label string) identity.ContentID {
	t.Helper()
	value, ok := identity.DeriveContentID("relation/check/recurrence/test/v1", []byte(label))
	if !ok {
		t.Fatal("derive token")
	}
	return value
}
