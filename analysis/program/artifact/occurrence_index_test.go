package artifact_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/domain/composite"
)

func TestArtifactOccurrenceIndexPublishesPointAttachments(t *testing.T) {
	lowered, err := lower.Lower(lower.Source{Name: "occurrence-index.lua", Text: []byte(`
local function run(value: number): number return value + 1 end
return run
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
		t.Fatalf("point indexing failed: %s", failure.Error())
	}
	program := artifact.Program()
	count, held := program.OccurrenceKindCount(programschema.OccurrencePointAttachment)
	if !held || count == 0 {
		t.Fatal("point index published no Site-to-WTO occurrence")
	}
}
