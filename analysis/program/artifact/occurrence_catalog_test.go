package artifact_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/domain/composite"
)

func TestArtifactOccurrenceCatalogRetainsPresenceRefinementRows(t *testing.T) {
	lowered, err := lower.Lower(lower.Source{Name: "occurrence-catalog.lua", Text: []byte(`
local function check(value: string?): string
  if value ~= nil then return value end
  return ""
end
return check
`)})
	if err != nil {
		t.Fatal(err)
	}
	compilation, ok := composite.Global()
	if !ok {
		t.Fatal("artifact grammar unavailable")
	}
	artifact, failure := compileArtifactForTest(t, lowered, compilation)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("catalog compilation failed: %s", failure.Error())
	}
	program := artifact.Program()
	count, held := program.OccurrenceKindCount(programschema.OccurrenceBinaryPresenceRefinement)
	if !held || count == 0 {
		t.Fatal("presence refinement occurrence was not catalogued")
	}
}
