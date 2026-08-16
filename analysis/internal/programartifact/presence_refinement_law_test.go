package programartifact_test

import (
	"testing"

	programartifact "github.com/wippyai/go-lua/analysis/internal/programartifact"
	"github.com/wippyai/go-lua/analysis/internal/programartifact/schemaadapter"
	"github.com/wippyai/go-lua/analysis/internal/programschema"
	"github.com/wippyai/go-lua/program/lower"
)

// TestProgramArtifactPresenceRefinementUsesExactGuardedPredecessor keeps the
// independently sound production contract: a nil-comparison refinement has
// one exact guarded predecessor and is placed at a normal local cut.  It
// deliberately admits no inferred later continuation or shared-base clone.
func TestProgramArtifactPresenceRefinementUsesExactGuardedPredecessor(t *testing.T) {
	published, err := lower.Lower(lower.Source{Name: "artifact-presence-refinement.lua", Text: []byte(`
local function redundant(value: string?): string
    if value ~= nil then
        if value ~= nil then return value end
    end
    return ""
end
return redundant
`)})
	if err != nil { t.Fatal(err) }
	receipt, receiptOK := programschema.Global()
	if !receiptOK { t.Fatal("Program artifact grammar capability unavailable") }
	artifact, failure := schemaadapter.CompileDetailed(published.TransformerInput(), receipt)
	if failure.Available() || artifact == nil || !artifact.Available() { t.Fatalf("compile: %s", failure.Error()) }
	count := 0
	for index := 0; index < artifact.RuleOccurrenceCount(programartifact.RuleRoleValuePresenceRefinement); index++ {
		rule, ruleOK := artifact.RuleOccurrenceAt(programartifact.RuleRoleValuePresenceRefinement, index)
		occurrence, occurrenceOK := artifact.OccurrenceForID(programartifact.OccurrenceBinaryPresenceRefinement, rule.ID())
		_, _, _, route, _, occurrenceOK2 := occurrence.BinaryPresenceRefinement()
		predecessor, predecessorOK := rule.PredecessorRouteID()
		point, pointOK := rule.PointAt(0)
		input, inputOK := rule.InputPoint()
		if !ruleOK || !occurrenceOK || !occurrenceOK2 || !predecessorOK || !pointOK || !inputOK || predecessor != route ||
			rule.InputKind() != programartifact.RuleInputPredecessor || rule.Stage() != programartifact.RuleStageLocal || point == input {
			t.Fatalf("refinement[%d] lost exact guarded predecessor/local placement", index)
		}
		count++
	}
	if count == 0 { t.Fatal("missing generic presence refinement") }
}
