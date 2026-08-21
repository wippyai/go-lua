package snapshottest

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/schema/ingress"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/domain/composite"
)

// MustLower seals one Program artifact through the composite structure
// vocabulary. Domain tests share this helper instead of copying it.
func MustLower(t testing.TB, artifact *programartifact.Artifact) *ingress.Snapshot {
	t.Helper()
	compilation, compilationOK := composite.Build()
	vocabulary, vocabularyOK := composite.StructureVocabulary(compilation)
	snapshot, lowered := ingress.Lower(artifact, vocabulary)
	if !compilationOK || !vocabularyOK || !lowered {
		t.Fatal("ingress lower")
	}
	return snapshot
}

// MustMount builds one Link mount directory row for an artifact placed at a
// module key. Tests share this helper so a fixture states the mount the same
// way the composition does, instead of assembling the row's identities by
// hand and drifting from what Available accepts.
func MustMount(t testing.TB, artifact *programartifact.Artifact, module identity.ContentID) programmount.Program {
	t.Helper()
	if artifact == nil || !artifact.Available() {
		t.Fatal("artifact is unavailable")
	}
	compiled := artifact.Program()
	catalog, published := programcatalog.CatalogID(compiled.SchemaID)
	if !compiled.Available() || !published || !catalog.Available() {
		t.Fatal("artifact publishes no cold value")
	}
	row := programmount.Program{
		ModuleKey: module,
		Program:   compiled,
	}
	if !row.Available() {
		t.Fatal("mount directory row unavailable")
	}
	return row
}
