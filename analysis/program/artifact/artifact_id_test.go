package artifact_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
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
		artifact, failure := artifactcompiler.CompileDetailed(published, grammar, artifactcompiler.IssuanceDirectory{})
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

func TestCompiledArtifactIDIsStableAcrossIndependentCompiles(t *testing.T) {
	compile := func() *programartifact.Artifact {
		t.Helper()
		published, err := lower.Lower(lower.Source{Name: "artifact-id-stable.lua", Text: []byte("local n = 1\nreturn n + 2")})
		if err != nil {
			t.Fatal(err)
		}
		grammar, ok := programartifact.NewGrammarIdentity(identity.ContentID{1}, programartifact.GrammarABIVersion)
		if !ok {
			t.Fatal("valid grammar identity was rejected")
		}
		artifact, failure := artifactcompiler.CompileDetailed(published, grammar, artifactcompiler.IssuanceDirectory{})
		if failure.Available() || artifact == nil || !artifact.Available() {
			t.Fatalf("artifact compile failed: %s", failure.Error())
		}
		return artifact
	}
	left, right := compile(), compile()
	if left.ID() != right.ID() || left.CompileKey().ID() != right.CompileKey().ID() {
		t.Fatal("independent compiles of one Program issued distinct artifact identities")
	}
}
