package compiler_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/domain/composite"
)

func compileArtifactForTest(t testing.TB, input *program.Program, compilation composite.Compilation) (*programartifact.Artifact, artifactcompiler.CompileFailure) {
	t.Helper()
	grammar := compilation.ExecutionSchemaID()
	issuance, issuanceOK := composite.ArtifactIssuanceDirectory(compilation)
	grammarOK := grammar.Available()
	if !grammarOK || !issuanceOK {
		t.Fatal("artifact compiler inputs")
	}
	return artifactcompiler.CompileDetailed(input, grammar, issuance)
}
