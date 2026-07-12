package body

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
)

func TestValidateGraphTransformerEligibilityCensusIsInstanceExact(t *testing.T) {
	prepared, _ := validateGraphSemanticProgramFixture(t)
	entries := transformer.NewPlanCompiler().EligibilityCensus(prepared.registry, prepared.cfg.Graph, prepared.operationPlan, transformer.Shape{})
	if len(entries) != 320 {
		t.Fatalf("eligibility entries = %d, want 320 active plan instances/family markers", len(entries))
	}
	type counts struct{ total, exact, unboundPath int }
	byFamily := map[string]counts{}
	for _, entry := range entries {
		count := byFamily[entry.Family]
		count.total++
		if entry.Exact {
			count.exact++
		}
		if strings.HasPrefix(entry.Reason, "source path symbol ") {
			count.unboundPath++
		}
		byFamily[entry.Family] = count
	}
	want := map[string]counts{
		"RootAssignments":               {total: 56, exact: 4, unboundPath: 14},
		"Returns":                       {total: 5, exact: 2},
		"PathDescendantInvalidations":   {total: 9},
		"PathValuePresenceImplications": {total: 3},
		"DynamicIndexWrites":            {total: 9},
		"CallSites":                     {total: 49},
		"BranchConditionSources":        {total: 36},
		"BranchRefinements":             {total: 57},
		"BranchPathEvidence":            {total: 43},
		"extension:1":                   {total: 38},
	}
	for family, expected := range want {
		if got := byFamily[family]; got != expected {
			t.Fatalf("%s census = %+v, want %+v", family, got, expected)
		}
	}
	t.Log("validate_graph exact now: RootAssignment 4/56, Return 2/5; 14 root-path copies await exact producer cells")
}
