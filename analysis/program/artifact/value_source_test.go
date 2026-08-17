package artifact_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/domain/composite"
)

func TestValueSourceCompilationRetainsLiteralOccurrences(t *testing.T) {
	published, err := lower.Lower(lower.Source{Name: "value-source.lua", Text: []byte("return 7, true, 'x'")})
	if err != nil {
		t.Fatal(err)
	}
	compilation, ok := composite.Global()
	if !ok {
		t.Fatal("artifact grammar unavailable")
	}
	artifact, failure := composite.CompileArtifactDetailed(published, compilation)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("value source compilation failed: %s", failure.Error())
	}
	if artifact.OccurrenceKindCount(programartifact.OccurrenceValueSource) < 3 {
		t.Fatalf("literal value-source count = %d", artifact.OccurrenceKindCount(programartifact.OccurrenceValueSource))
	}
}
