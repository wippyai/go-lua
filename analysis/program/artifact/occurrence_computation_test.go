package artifact_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/domain/composite"
)

func TestArtifactOccurrenceComputationCatalogRetainsBinaryArithmetic(t *testing.T) {
	lowered, err := lower.Lower(lower.Source{Name: "occurrence-computation.lua", Text: []byte(`
local function add(value: number): number return value + 1 end
return add
`)})
	if err != nil {
		t.Fatal(err)
	}
	compilation, ok := composite.Global()
	if !ok {
		t.Fatal("artifact grammar unavailable")
	}
	artifact, failure := composite.CompileArtifactDetailed(lowered, compilation)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("computation compilation failed: %s", failure.Error())
	}
	program := artifact.Program()
	count, held := program.OccurrenceKindCount(programschema.OccurrenceBinaryArithmetic)
	if !held || count == 0 {
		t.Fatal("binary arithmetic occurrence was not catalogued")
	}
}
