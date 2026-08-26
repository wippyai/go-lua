package compiler_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programissuance "github.com/wippyai/go-lua/analysis/schema/program/issuance"
	"github.com/wippyai/go-lua/domain/composite"
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
	if err != nil {
		t.Fatal(err)
	}
	compilation, compilationOK := composite.Build()
	if !compilationOK {
		t.Fatal("Program artifact grammar unavailable")
	}
	artifact, failure := compileArtifactForTest(t, published, compilation)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("compile: %s", failure.Error())
	}
	program := artifact.Program()
	placementCount, placementsPublished := program.RuleOccurrenceCountForKey("value-presence-refinement")
	if !placementsPublished {
		t.Fatal("rule-occurrence family is unpublished")
	}
	count := 0
	for index := 0; index < placementCount; index++ {
		rule, ruleOK := program.RuleOccurrenceForKeyAt("value-presence-refinement", index)
		ordinal, ordinalOK := rule.Occurrence()
		occurrence, occurrenceOK := program.OccurrenceAt(int(ordinal))
		_, bodyOK := occurrence.BodyID()
		route, routeInputOK := program.OccurrenceInputID(int(ordinal), 3)
		predecessor, predecessorOK := rule.PredecessorRoute()
		point := rule.PointID()
		input, inputOK := rule.InputPointAt(0)
		if !ruleOK || !ordinalOK || !occurrenceOK || !bodyOK || occurrence.Kind() != programschema.OccurrenceBinaryPresenceRefinement || !routeInputOK || !predecessorOK || !point.Available() || !inputOK || predecessor.ID != route || !predecessor.Point.Available() ||
			rule.InputSpec() != programissuance.InputRouteArrival || rule.Stage() != programissuance.StageRoutePredecessor || point == input || predecessor.Point == input {
			t.Fatalf("refinement[%d] lost exact guarded route placement", index)
		}
		count++
	}
	if count == 0 {
		t.Fatal("missing generic presence refinement")
	}
}
