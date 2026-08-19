package snapshottest

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/schema/ingress"
	"github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/domain/composite"
)

// MustLower seals one Program artifact through the composite structure
// vocabulary. Domain tests share this helper instead of copying it.
func MustLower(t testing.TB, artifact *programartifact.Artifact) *ingress.Snapshot {
	t.Helper()
	vocabulary, vocabularyOK := composite.StructureVocabulary()
	snapshot, lowered := ingress.Lower(artifact, vocabulary)
	if !vocabularyOK || !lowered {
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
	frozen, catalog, published := artifact.ColdPublication()
	if !published || !catalog.Available() {
		t.Fatal("artifact publishes no cold value")
	}
	row := programmount.Program{
		ModuleKey: module,
		Program: programschema.Program{
			Frozen: frozen, ArtifactID: artifact.ID(),
			ProgramID: artifact.CompileKey().ProgramID(), SchemaID: artifact.CompileKey().SchemaDigest(),
		},
	}
	if !row.Available() {
		t.Fatal("mount directory row unavailable")
	}
	return row
}
