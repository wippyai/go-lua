package recurrence_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/relation/check/recurrence"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
)

// A Complete owner and the writer of its child relation in one SCC are a
// hostile recurrence: no fixed point can use a closed denominator whose
// population is still being written by the same component.
func TestCompleteRejectsSameComponentWriter(t *testing.T) {
	value := newCompleteFixture(t, "same-component")
	childRef, _ := plan.NewRelationRef(value.child)
	otherRef, _ := plan.NewRelationRef(value.other)
	denominator, ok := model.NewDenominatorRef(value.child, value.key)
	if !ok {
		t.Fatal("denominator")
	}
	writerExpression := plan.DefineExpressionRef(value.writerExpression, algebra.NewPublish(
		algebra.NewInput(value.other), algebra.NewPublishContract(value.child, model.KeyID{})))
	ownerExpression := plan.DefineExpressionRef(value.ownerExpression, algebra.NewPublish(
		algebra.NewComplete(algebra.NewInput(value.child), denominator), algebra.NewPublishContract(value.other, model.KeyID{})))
	writer := plan.DefineDependency(value.writer, value.writerExpression, []plan.RelationRef{otherRef}, []plan.RelationRef{childRef}, "writer")
	owner := plan.DefineDependency(value.owner, value.ownerExpression, []plan.RelationRef{childRef}, []plan.RelationRef{otherRef}, "owner")
	writerRef := plan.DefineDependencyRef(value.writer)
	ownerRef := plan.DefineDependencyRef(value.owner)
	component := plan.DefineSCC([]plan.DependencyRef{writerRef, ownerRef}, []plan.DependencyEdge{
		plan.DefineDependencyEdge(writerRef, ownerRef), plan.DefineDependencyEdge(ownerRef, writerRef),
	}, plan.DefineRecurrence(plan.Positive, nil))
	builder := plan.NewBuilder(value.schema)
	if !builder.AddExpression(writerExpression) || !builder.AddExpression(ownerExpression) || !builder.AddDependency(writer) || !builder.AddDependency(owner) || !builder.AddSCC(component) {
		t.Fatal("add hostile declarations")
	}
	schema, ok := builder.Build()
	if !ok {
		t.Fatal("build hostile schema")
	}
	_, err := recurrence.Check(schema)
	if err == nil {
		t.Fatal("same-component Complete writer was accepted")
	}
	refusal, ok := err.(*recurrence.Error)
	if !ok || refusal.Code != recurrence.CodeCompleteStratification || refusal.Dependency != value.owner || refusal.Other != value.writer || refusal.Relation != value.child || !refusal.Occurrence.Available() {
		t.Fatalf("wrong Complete refusal: %T %#v", err, err)
	}
}

func TestCompleteAcceptsColdAndEarlierComponentWriters(t *testing.T) {
	value := newCompleteFixture(t, "earlier")
	childRef, _ := plan.NewRelationRef(value.child)
	otherRef, _ := plan.NewRelationRef(value.other)
	denominator, ok := model.NewDenominatorRef(value.child, value.key)
	if !ok {
		t.Fatal("denominator")
	}
	writerExpression := plan.DefineExpressionRef(value.writerExpression, algebra.NewPublish(
		algebra.NewInput(value.other), algebra.NewPublishContract(value.child, model.KeyID{})))
	ownerExpression := plan.DefineExpressionRef(value.ownerExpression, algebra.NewComplete(algebra.NewInput(value.child), denominator))
	writer := plan.DefineDependency(value.writer, value.writerExpression, []plan.RelationRef{otherRef}, []plan.RelationRef{childRef}, "writer")
	owner := plan.DefineDependency(value.owner, value.ownerExpression, []plan.RelationRef{childRef}, nil, "owner")
	writerRef := plan.DefineDependencyRef(value.writer)
	ownerRef := plan.DefineDependencyRef(value.owner)
	writerSCC := plan.DefineSCC([]plan.DependencyRef{writerRef}, nil, plan.DefineRecurrence(plan.Acyclic, nil))
	ownerSCC := plan.DefineSCC([]plan.DependencyRef{ownerRef}, []plan.DependencyEdge{}, plan.DefineRecurrence(plan.Acyclic, nil))
	builder := plan.NewBuilder(value.schema)
	if !builder.AddExpression(writerExpression) || !builder.AddExpression(ownerExpression) || !builder.AddDependency(writer) || !builder.AddDependency(owner) || !builder.AddSCC(writerSCC) || !builder.AddSCC(ownerSCC) {
		t.Fatal("add earlier declarations")
	}
	schema, ok := builder.Build()
	if !ok {
		t.Fatal("build earlier schema")
	}
	proof, err := recurrence.Check(schema)
	if err != nil || !proof.Available() {
		t.Fatalf("earlier Complete writer refused: %v", err)
	}
	uses := proof.CompleteUses()
	if len(uses) != 1 || !uses[0].Available() || uses[0].Cold || len(uses[0].Writers) != 1 || uses[0].Writers[0].Dependency != value.writer || !uses[0].Writers[0].Earlier {
		t.Fatalf("earlier Complete proof = %#v", uses)
	}

	// Rebuild a genuinely cold owner over the same denominator. Its proof is
	// distinct from the writer-backed occurrence and explicitly records the
	// empty solve writer set.
	coldExpressionID := issueExpression(t, value.ownerOwner, "cold-expression")
	coldDependencyID := issueDependency(t, value.ownerOwner, "cold-dependency")
	coldExpression := plan.DefineExpressionRef(coldExpressionID, algebra.NewComplete(algebra.NewInput(value.child), denominator))
	coldDependency := plan.DefineDependency(coldDependencyID, coldExpressionID, []plan.RelationRef{childRef}, nil, "cold")
	coldSCC := plan.DefineSCC([]plan.DependencyRef{plan.DefineDependencyRef(coldDependencyID)}, nil, plan.DefineRecurrence(plan.Acyclic, nil))
	coldBuilder := plan.NewBuilder(value.schema)
	if !coldBuilder.AddExpression(coldExpression) || !coldBuilder.AddDependency(coldDependency) || !coldBuilder.AddSCC(coldSCC) {
		t.Fatal("add cold declarations")
	}
	coldSchema, ok := coldBuilder.Build()
	if !ok {
		t.Fatal("build cold schema")
	}
	coldProof, err := recurrence.Check(coldSchema)
	if err != nil || len(coldProof.CompleteUses()) != 1 || !coldProof.CompleteUses()[0].Cold {
		t.Fatalf("cold Complete proof = %v / %#v", err, coldProof.CompleteUses())
	}
}

type completeFixture struct {
	schema           model.SchemaID
	ownerOwner       model.OwnerID
	child            model.RelationID
	other            model.RelationID
	key              model.KeyID
	writer           model.DependencyID
	owner            model.DependencyID
	writerExpression model.ExpressionID
	ownerExpression  model.ExpressionID
}

func newCompleteFixture(t *testing.T, label string) completeFixture {
	t.Helper()
	owner := issueOwner(t, "complete/"+label+"/owner")
	child := issueRelation(t, owner, "complete/"+label+"/child")
	return completeFixture{
		schema:           issueSchema(t, owner, "complete/"+label+"/schema"),
		ownerOwner:       owner,
		child:            child,
		other:            issueRelation(t, owner, "complete/"+label+"/other"),
		key:              issueKey(t, child, "complete/"+label+"/child-key"),
		writer:           issueDependency(t, owner, "complete/"+label+"/writer"),
		owner:            issueDependency(t, owner, "complete/"+label+"/owner-dependency"),
		writerExpression: issueExpression(t, owner, "complete/"+label+"/writer-expression"),
		ownerExpression:  issueExpression(t, owner, "complete/"+label+"/owner-expression"),
	}
}
