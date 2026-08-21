package composite

import "testing"

// TestBuildSealsIndependentCompilationsWithOneSemanticIdentity states the
// Workspace environment boundary: equal declarations seal equal identities,
// but no mutable engine schema instance is shared between environments.
func TestBuildSealsIndependentCompilationsWithOneSemanticIdentity(t *testing.T) {
	first, firstOK := Build()
	second, secondOK := Build()
	if !firstOK || !secondOK || !first.Available() || !second.Available() {
		t.Fatal("independent compilation unavailable")
	}
	if first.Schema() == second.Schema() {
		t.Fatal("independent compilations shared one engine schema instance")
	}
	if first.Digest() != second.Digest() || first.ExecutionSchemaID() != second.ExecutionSchemaID() {
		t.Fatal("equal declarations produced different semantic identities")
	}
	firstPublication, firstPublicationOK := first.Publication()
	secondPublication, secondPublicationOK := second.Publication()
	firstSchemaID, firstSchemaIDOK := firstPublication.SchemaID()
	secondSchemaID, secondSchemaIDOK := secondPublication.SchemaID()
	if !firstPublicationOK || !secondPublicationOK || !firstSchemaIDOK || !secondSchemaIDOK || firstSchemaID != secondSchemaID {
		t.Fatal("equal declarations produced different publication layouts")
	}
}
