package expand_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement/expand"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

func TestFreezeIssuesFencedKeysAndPreservesOwnerOrder(t *testing.T) {
	owner, ok := model.IssueOwnerID(token("owner"))
	if !ok {
		t.Fatal("owner")
	}
	schema, ok := model.IssueSchemaID(owner, token("schema"))
	if !ok {
		t.Fatal("schema")
	}
	candidate, ok := model.IssueRelationID(owner, token("candidate"))
	if !ok {
		t.Fatal("candidate")
	}
	publisher, ok := model.IssueRelationID(owner, token("publisher"))
	if !ok {
		t.Fatal("publisher")
	}
	reader, ok := model.IssueRelationID(owner, token("reader"))
	if !ok {
		t.Fatal("reader")
	}
	column, ok := model.IssueColumnID(reader, token("key"))
	if !ok {
		t.Fatal("column")
	}
	typeID, ok := model.IssueTypeID(owner, token("number"))
	if !ok {
		t.Fatal("type")
	}
	correlation, ok := model.IssueRelationID(owner, token("correlation"))
	if !ok {
		t.Fatal("correlation")
	}
	scope, ok := model.IssueScopeID(owner, token("scope"))
	if !ok {
		t.Fatal("scope")
	}
	contract := model.DefineExpandContract(candidate, publisher, reader, column, correlation).WithScope(scope)
	fence, ok := binding.NewFence(schema, identity.MountID{1}, identity.Generation(1))
	if !ok {
		t.Fatal("fence")
	}
	issuer, ok := binding.NewIssuer(fence)
	if !ok {
		t.Fatal("issuer")
	}
	first := token("first-key")
	second := token("second-key")
	candidateA, _ := model.IssueRowID(candidate, token("candidate-a"))
	candidateB, _ := model.IssueRowID(candidate, token("candidate-b"))
	publisherA, _ := model.IssueRowID(publisher, token("publisher-a"))
	publisherB, _ := model.IssueRowID(publisher, token("publisher-b"))
	rows, ok := []expand.Vector{
		mustVector(t, candidateA, publisherA, []identity.ContentID{first, second}),
		mustVector(t, candidateB, publisherB, []identity.ContentID{token("third-key")}),
	}, true
	if !ok {
		t.Fatal("rows")
	}
	evidence, ok := expand.Freeze(fence, issuer, contract, typeID, rows)
	if !ok || !evidence.Available() {
		t.Fatal("freeze")
	}
	vector, ok := evidence.VectorAt(candidateA)
	if !ok || vector.KeyCount() != 2 {
		t.Fatal("candidate vector")
	}
	left, ok := vector.KeyAt(0)
	if !ok || left.Opaque() != first || !left.ValidFor(fence) || left.Type() != typeID {
		t.Fatal("first key token")
	}
	right, ok := vector.KeyAt(1)
	if !ok || right.Opaque() != second {
		t.Fatal("owner order")
	}
	if !evidence.Digest().Available() {
		t.Fatal("digest")
	}
	firstKey, ok := vector.KeyAt(0)
	if !ok {
		t.Fatal("first key lookup")
	}
	candidates, ok := evidence.CandidatesForKey(firstKey)
	if !ok || len(candidates) != 1 || candidates[0] != candidateA {
		t.Fatalf("key inverse=%v, want candidate A", candidates)
	}
}

func TestFreezeRefusesDuplicateCandidatesAndDuplicateKeys(t *testing.T) {
	fixture := newFixture(t)
	duplicateCandidate := []expand.Vector{
		mustVector(t, fixture.candidate, fixture.publisherA, []identity.ContentID{token("a")}),
		mustVector(t, fixture.candidate, fixture.publisherB, []identity.ContentID{token("b")}),
	}
	if evidence, ok := expand.Freeze(fixture.fence, fixture.issuer, fixture.contract, fixture.typeID, duplicateCandidate); ok || evidence.Available() {
		t.Fatal("duplicate candidates admitted")
	}
	if vector, ok := expand.NewVector(fixture.candidate, fixture.publisherA, []identity.ContentID{token("a"), token("a")}); ok || vector.Available() {
		t.Fatal("duplicate keys admitted into owner vector")
	}
	duplicatePublisher := []expand.Vector{
		mustVector(t, fixture.candidate, fixture.publisherA, []identity.ContentID{token("c")}),
		mustVector(t, mustRow(t, fixture.candidate.Relation(), "other-candidate"), fixture.publisherA, []identity.ContentID{token("d")}),
	}
	if evidence, ok := expand.Freeze(fixture.fence, fixture.issuer, fixture.contract, fixture.typeID, duplicatePublisher); ok || evidence.Available() {
		t.Fatal("duplicate publishers admitted")
	}
}

func TestNewVectorPreservesAuthenticatedEmptyKeys(t *testing.T) {
	fixture := newFixture(t)
	vector, ok := expand.NewVector(fixture.candidate, fixture.publisherA, []identity.ContentID{})
	if !ok || !vector.Available() || vector.KeyCount() != 0 {
		t.Fatal("authenticated empty vector was not preserved")
	}
	if _, ok := vector.KeyAt(0); ok {
		t.Fatal("empty vector exposed a key")
	}
	if vector, ok := expand.NewVector(fixture.candidate, fixture.publisherA, nil); ok || vector.Available() {
		t.Fatal("unavailable nil vector was admitted")
	}
}

func TestFreezeRefusesMissingMountedScope(t *testing.T) {
	fixture := newFixture(t)
	withoutScope := model.DefineExpandContract(fixture.contract.Candidate(), fixture.contract.Publisher(), fixture.contract.Reader(), fixture.contract.Key(), fixture.contract.Correlation())
	if evidence, ok := expand.Freeze(fixture.fence, fixture.issuer, withoutScope, fixture.typeID, []expand.Vector{}); ok || evidence.Available() {
		t.Fatal("unmounted Expand contract was admitted")
	}
}

func TestCatalogSealsExpressionDirectoryAndRejectsDuplicateEntries(t *testing.T) {
	fixture := newFixture(t)
	first, ok := expand.Freeze(fixture.fence, fixture.issuer, fixture.contract, fixture.typeID, []expand.Vector{mustVector(t, fixture.candidate, fixture.publisherA, []identity.ContentID{token("catalog-key")})})
	if !ok {
		t.Fatal("freeze first evidence")
	}
	expression := token("expand-expression")
	catalog, ok := expand.NewCatalog([]expand.Entry{{Expression: expression, Evidence: first}})
	if !ok || !catalog.Available() {
		t.Fatal("catalog")
	}
	resolved, ok := catalog.At(expression)
	if !ok || resolved.Digest() != first.Digest() {
		t.Fatal("catalog lookup")
	}
	if duplicate, ok := expand.NewCatalog([]expand.Entry{{Expression: expression, Evidence: first}, {Expression: expression, Evidence: first}}); ok || duplicate.Available() {
		t.Fatal("duplicate expression entries admitted")
	}
	if !expand.EmptyCatalog().Available() {
		t.Fatal("empty catalog is not a closed set")
	}
}

func TestFreezeRefusesForeignCandidateRowInsteadOfUsingContentEquality(t *testing.T) {
	fixture := newFixture(t)
	foreignRelation, ok := model.IssueRelationID(fixture.candidate.Relation().Owner(), token("foreign-candidate-relation"))
	if !ok {
		t.Fatal("foreign relation")
	}
	foreignCandidate, ok := model.IssueRowID(foreignRelation, fixture.candidate.Content())
	if !ok {
		t.Fatal("foreign candidate")
	}
	value := mustVector(t, foreignCandidate, fixture.publisherA, []identity.ContentID{token("key")})
	if evidence, ok := expand.Freeze(fixture.fence, fixture.issuer, fixture.contract, fixture.typeID, []expand.Vector{value}); ok || evidence.Available() {
		t.Fatal("foreign C relation was admitted through equal content")
	}
}

func TestFreezeRefusesForeignPublisherRowInsteadOfUsingContentEquality(t *testing.T) {
	fixture := newFixture(t)
	foreignRelation, ok := model.IssueRelationID(fixture.publisherA.Relation().Owner(), token("foreign-publisher-relation"))
	if !ok {
		t.Fatal("foreign relation")
	}
	foreignPublisher, ok := model.IssueRowID(foreignRelation, fixture.publisherA.Content())
	if !ok {
		t.Fatal("foreign publisher")
	}
	value := mustVector(t, fixture.candidate, foreignPublisher, []identity.ContentID{token("key")})
	if evidence, ok := expand.Freeze(fixture.fence, fixture.issuer, fixture.contract, fixture.typeID, []expand.Vector{value}); ok || evidence.Available() {
		t.Fatal("foreign P relation was admitted through equal content")
	}
}

func TestEvidenceDigestAndFenceAreExact(t *testing.T) {
	fixture := newFixture(t)
	value := mustVector(t, fixture.candidate, fixture.publisherA, []identity.ContentID{token("fenced-key")})
	evidence, ok := expand.Freeze(fixture.fence, fixture.issuer, fixture.contract, fixture.typeID, []expand.Vector{value})
	if !ok || !evidence.Available() {
		t.Fatal("freeze")
	}
	foreignFence, ok := binding.NewFence(fixture.fence.Schema(), identity.MountID{9}, fixture.fence.Generation())
	if !ok {
		t.Fatal("foreign fence")
	}
	if evidence.ValidFor(foreignFence) {
		t.Fatal("evidence crossed mount fence")
	}
	otherPublisherValue := mustVector(t, fixture.candidate, fixture.publisherB, []identity.ContentID{token("fenced-key")})
	otherPublisher, ok := expand.Freeze(fixture.fence, fixture.issuer, fixture.contract, fixture.typeID, []expand.Vector{otherPublisherValue})
	if !ok || !otherPublisher.Available() {
		t.Fatal("publisher evidence variant freeze")
	}
	if evidence.Digest() == otherPublisher.Digest() {
		t.Fatal("publisher evidence was omitted from the sealed digest")
	}
	otherScope, ok := model.IssueScopeID(fixture.contract.Candidate().Owner(), token("other-scope"))
	if !ok {
		t.Fatal("other scope")
	}
	otherContract := fixture.contract.WithScope(otherScope)
	other, ok := expand.Freeze(fixture.fence, fixture.issuer, otherContract, fixture.typeID, []expand.Vector{value})
	if !ok || !other.Available() {
		t.Fatal("other-scope freeze")
	}
	if evidence.Digest() == other.Digest() {
		t.Fatal("contract scope was omitted from evidence digest")
	}
}

type fixture struct {
	fence      binding.Fence
	issuer     binding.Issuer
	contract   model.ExpandContract
	typeID     model.TypeID
	scope      model.ScopeID
	candidate  model.RowID
	publisherA model.RowID
	publisherB model.RowID
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	owner, _ := model.IssueOwnerID(token("fixture-owner"))
	schema, _ := model.IssueSchemaID(owner, token("fixture-schema"))
	candidateRelation, _ := model.IssueRelationID(owner, token("fixture-candidate"))
	publisher, _ := model.IssueRelationID(owner, token("fixture-publisher"))
	reader, _ := model.IssueRelationID(owner, token("fixture-reader"))
	column, _ := model.IssueColumnID(reader, token("fixture-column"))
	correlation, _ := model.IssueRelationID(owner, token("fixture-correlation"))
	typeID, _ := model.IssueTypeID(owner, token("fixture-type"))
	scope, _ := model.IssueScopeID(owner, token("fixture-scope"))
	candidate, _ := model.IssueRowID(candidateRelation, token("fixture-candidate-row"))
	publisherA, _ := model.IssueRowID(publisher, token("fixture-publisher-a"))
	publisherB, _ := model.IssueRowID(publisher, token("fixture-publisher-b"))
	contract := model.DefineExpandContract(candidateRelation, publisher, reader, column, correlation).WithScope(scope)
	fence, _ := binding.NewFence(schema, identity.MountID{2}, identity.Generation(1))
	issuer, _ := binding.NewIssuer(fence)
	return fixture{fence: fence, issuer: issuer, contract: contract, typeID: typeID, scope: scope, candidate: candidate, publisherA: publisherA, publisherB: publisherB}
}

func mustVector(t *testing.T, candidate, publisher model.RowID, keys []identity.ContentID) expand.Vector {
	t.Helper()
	value, ok := expand.NewVector(candidate, publisher, keys)
	if !ok {
		t.Fatal("vector")
	}
	return value
}

func mustRow(t *testing.T, relation model.RelationID, label string) model.RowID {
	t.Helper()
	value, ok := model.IssueRowID(relation, token(label))
	if !ok {
		t.Fatal("row")
	}
	return value
}

func token(label string) identity.ContentID {
	value, _ := identity.DeriveContentID("analysis/relation/mount/arrangement/expand/law/v1", []byte(label))
	return value
}
