package certificate

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	checkrecurrence "github.com/wippyai/go-lua/analysis/relation/check/recurrence"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
)

func TestRecurrenceDataCopiesCheckedProofAsIdentityProjection(t *testing.T) {
	owner, ok := model.IssueOwnerID(tokenForTest("owner"))
	if !ok {
		t.Fatal("owner identity unavailable")
	}
	schemaID, ok := model.IssueSchemaID(owner, tokenForTest("schema"))
	if !ok {
		t.Fatal("schema identity unavailable")
	}
	relationA, ok := model.IssueRelationID(owner, tokenForTest("relation-a"))
	if !ok {
		t.Fatal("relation identity unavailable")
	}
	relationB, ok := model.IssueRelationID(owner, tokenForTest("relation-b"))
	if !ok {
		t.Fatal("relation identity unavailable")
	}
	dependencyA, ok := model.IssueDependencyID(owner, tokenForTest("dependency-a"))
	if !ok {
		t.Fatal("dependency identity unavailable")
	}
	dependencyB, ok := model.IssueDependencyID(owner, tokenForTest("dependency-b"))
	if !ok {
		t.Fatal("dependency identity unavailable")
	}
	expressionA, ok := model.IssueExpressionID(owner, tokenForTest("expression-a"))
	if !ok {
		t.Fatal("expression identity unavailable")
	}
	expressionB, ok := model.IssueExpressionID(owner, tokenForTest("expression-b"))
	if !ok {
		t.Fatal("expression identity unavailable")
	}
	refA, _ := plan.NewRelationRef(relationA)
	refB, _ := plan.NewRelationRef(relationB)
	depRefA := plan.DefineDependencyRef(dependencyA)
	depRefB := plan.DefineDependencyRef(dependencyB)
	entryA := plan.DefineExpressionRef(expressionA, algebra.NewPublish(algebra.NewInput(relationB), algebra.NewPublishContract(relationA, model.KeyID{})))
	entryB := plan.DefineExpressionRef(expressionB, algebra.NewPublish(algebra.NewInput(relationA), algebra.NewPublishContract(relationB, model.KeyID{})))
	declA := plan.DefineDependency(dependencyA, expressionA, []plan.RelationRef{refB}, []plan.RelationRef{refA}, "a")
	declB := plan.DefineDependency(dependencyB, expressionB, []plan.RelationRef{refA}, []plan.RelationRef{refB}, "b")
	builder := plan.NewBuilder(schemaID)
	if !builder.AddExpression(entryA) || !builder.AddExpression(entryB) || !builder.AddDependency(declA) || !builder.AddDependency(declB) || !builder.AddSCC(plan.DefineSCC(
		[]plan.DependencyRef{depRefA, depRefB},
		[]plan.DependencyEdge{plan.DefineDependencyEdge(depRefA, depRefB), plan.DefineDependencyEdge(depRefB, depRefA)},
		plan.DefineRecurrence(plan.Positive, []plan.WideningHead{plan.DefineWideningHead(depRefA, refA), plan.DefineWideningHead(depRefB, refB)}),
	)) {
		t.Fatal("declaration rejected")
	}
	schema, ok := builder.Build()
	if !ok {
		t.Fatal("schema build rejected")
	}
	proof, err := checkrecurrence.Check(schema)
	if err != nil {
		t.Fatalf("recurrence proof rejected: %v", err)
	}
	data := recurrenceData(proof, []plan.Dependency{declA, declB})
	if !data.Available() || len(data.Projections()) != 2 || len(data.Edges()) != 2 || len(data.Components()) != 1 || len(data.WideningHeads()) != 2 {
		t.Fatalf("incomplete recurrence projection: available=%v projections=%d edges=%d components=%d heads=%d", data.Available(), len(data.Projections()), len(data.Edges()), len(data.Components()), len(data.WideningHeads()))
	}
	projection := data.Projections()[0]
	if !projection.Dependency().Available() || !projection.Expression().Available() || len(projection.Reads()) != 1 || len(projection.Writes()) != 1 {
		t.Fatal("projection lost dependency, expression, or relation identities")
	}
	component := data.Components()[0]
	if !component.Cyclic() || len(component.Members()) != 2 || len(component.Edges()) != 2 {
		t.Fatal("component lost checked SCC structure")
	}
	projections := data.Projections()
	reads := projections[0].Reads()
	reads[0] = model.RelationID{}
	if !data.Projections()[0].Reads()[0].Available() {
		t.Fatal("nested relation projection was mutable")
	}
	components := data.Components()
	members := components[0].Members()
	members[0] = model.DependencyID{}
	if !data.Components()[0].Members()[0].Available() {
		t.Fatal("nested component projection was mutable")
	}
}

func tokenForTest(label string) identity.ContentID {
	value, _ := identity.DeriveContentID("relation/check/certificate/test/v1", []byte(label))
	return value
}
