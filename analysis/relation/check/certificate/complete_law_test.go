package certificate_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/relation/check/certificate"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
)

func issueCompleteDependency(t *testing.T, owner model.OwnerID, label string) model.DependencyID {
	t.Helper()
	id, ok := model.IssueDependencyID(owner, token(t, label))
	if !ok {
		t.Fatal("dependency identity")
	}
	return id
}

func TestCertificateCompleteEvidenceIsImmutableAndDigestRelevant(t *testing.T) {
	value := newFixture(t)
	ref, ok := plan.NewRelationRef(value.relation)
	if !ok {
		t.Fatal("relation reference")
	}
	denominator, ok := model.NewDenominatorRef(value.relation, value.key)
	if !ok {
		t.Fatal("denominator")
	}
	expressionID := issueExpression(t, value.owner, "complete-evidence-expression")
	dependencyID := issueCompleteDependency(t, value.owner, "complete-evidence-dependency")
	expression := plan.DefineExpressionRef(expressionID, algebra.NewComplete(algebra.NewInput(value.relation), denominator))
	dependency := plan.DefineDependency(dependencyID, expressionID, []plan.RelationRef{ref}, nil, "complete-evidence")
	dependencyRef := plan.DefineDependencyRef(dependencyID)
	scc := plan.DefineSCC([]plan.DependencyRef{dependencyRef}, nil, plan.DefineRecurrence(plan.Acyclic, nil))
	schema := buildSchema(t, value, false, func(builder *plan.Builder) {
		builder.AddExpression(expression)
		builder.AddDependency(dependency)
		builder.AddSCC(scc)
	})
	cert, refusal := certificate.Check(schema)
	if refusal != nil || !cert.Available() {
		t.Fatalf("Complete certificate refused: %v", refusal)
	}
	uses := cert.Recurrence().CompleteUses()
	if len(uses) != 1 || !uses[0].Available() || !uses[0].Cold() || uses[0].ChildRelation() != value.relation || uses[0].Denominator() != denominator || uses[0].Path() == "" || !uses[0].Occurrence().Available() {
		t.Fatalf("unexpected Complete evidence: %#v", uses)
	}
	evidenceDigest := uses[0].Digest()
	uses[0] = certificate.CompleteUse{}
	if cert.Recurrence().CompleteUses()[0].Digest() != evidenceDigest {
		t.Fatal("certificate exposed mutable Complete evidence")
	}
	if cert.Digest() == schema.Digest() {
		t.Fatal("certificate digest unexpectedly equals schema digest")
	}
}
