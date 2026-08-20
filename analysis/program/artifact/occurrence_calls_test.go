package artifact_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/domain/composite"
)

func TestArtifactCallOccurrenceCompilationRetainsCallRows(t *testing.T) {
	published, err := lower.Lower(lower.Source{Name: "occurrence-calls.lua", Text: []byte(`
local function identity(value) return value end
return identity(1)
`)})
	if err != nil {
		t.Fatal(err)
	}
	compilation, ok := composite.Global()
	if !ok {
		t.Fatal("artifact grammar unavailable")
	}
	artifact, failure := compileArtifactForTest(t, published, compilation)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("call occurrence compilation failed: %s", failure.Error())
	}
	program := artifact.Program()
	catalog, coldPublished := programschema.CatalogID(program.SchemaID)
	callCount, callsOK := program.CallCount()
	if !coldPublished || !catalog.Available() || !callsOK || !program.Available() || callCount == 0 {
		t.Fatal("call occurrence compilation published no call family")
	}
}
