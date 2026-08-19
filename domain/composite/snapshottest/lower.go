package snapshottest

import (
	"testing"

	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/schema/ingress"
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
