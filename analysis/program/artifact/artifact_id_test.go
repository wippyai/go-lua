package artifact_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
)

func TestArtifactIDCommitsCompileKeyIdentity(t *testing.T) {
	compile := func(schema identity.ContentID) *programartifact.Artifact {
		t.Helper()
		published, err := lower.Lower(lower.Source{
			Name: "artifact-id.lua",
			Text: []byte("return 1"),
		})
		if err != nil {
			t.Fatal(err)
		}
		grammar, ok := programartifact.NewGrammarIdentity(schema, programartifact.GrammarABIVersion)
		if !ok {
			t.Fatal("valid grammar identity was rejected")
		}
		artifact, failure := programartifact.CompileDetailed(published, grammar, nil)
		if failure.Available() || artifact == nil || !artifact.Available() {
			t.Fatalf("artifact compile failed: %s", failure.Error())
		}
		return artifact
	}

	left, right := compile(identity.ContentID{1}), compile(identity.ContentID{2})
	if left.CompileKey().ID() == right.CompileKey().ID() {
		t.Fatal("compile key ignored grammar identity")
	}
	if left.ID() == right.ID() {
		t.Fatal("artifact identity ignored its compile key")
	}
}
