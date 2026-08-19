package artifact_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/schema/cold"
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
	artifact, failure := composite.CompileArtifactDetailed(published, compilation)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("call occurrence compilation failed: %s", failure.Error())
	}
	artifactID := artifact.ID()
	module, moduleOK := identity.DeriveContentID("occurrence-calls-module", artifactID[:])
	frozen, catalog, coldPublished := artifact.ColdPublication()
	program := cold.Program{
		Frozen: frozen, ModuleKey: module, ArtifactID: artifact.ID(),
		ProgramID: artifact.CompileKey().ProgramID(), SchemaID: artifact.CompileKey().SchemaDigest(),
	}
	callCount, callsOK := program.CallCount()
	if !moduleOK || !coldPublished || !catalog.Available() || !callsOK || !program.Available() || callCount == 0 {
		t.Fatal("call occurrence compilation published no call family")
	}
}
