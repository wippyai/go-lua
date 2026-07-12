package body

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	"github.com/wippyai/go-lua/analysis/domain/effect/iteration"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func TestValidateGraphTransformerEligibilityCensusIsInstanceExact(t *testing.T) {
	prepared, _ := validateGraphSemanticProgramFixture(t)
	entries := transformer.NewPlanCompiler().EligibilityCensus(prepared.registry, prepared.cfg.Graph, prepared.operationPlan, transformer.Shape{Params: uint32(len(prepared.operationPlan.BoundaryParams()))})
	if len(entries) != 320 {
		t.Fatalf("eligibility entries = %d, want 320 active plan instances/family markers", len(entries))
	}
	type counts struct{ total, exact, unboundPath, iteratorCall int }
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
		if entry.Extension && strings.HasPrefix(entry.Reason, "generic-for:") {
			count.iteratorCall++
		}
		byFamily[entry.Family] = count
	}
	want := map[string]counts{
		"RootAssignments":               {total: 56, exact: 25, unboundPath: 2},
		"Returns":                       {total: 5, exact: 2},
		"PathDescendantInvalidations":   {total: 9},
		"PathValuePresenceImplications": {total: 3},
		"DynamicIndexWrites":            {total: 9},
		"CallSites":                     {total: 49, exact: 33},
		"BranchConditionSources":        {total: 36},
		"BranchRefinements":             {total: 57},
		"BranchPathEvidence":            {total: 43},
		"extension:1":                   {total: 38, exact: 22, iteratorCall: 16},
	}
	for family, expected := range want {
		if got := byFamily[family]; got != expected {
			t.Fatalf("%s census = %+v, want %+v", family, got, expected)
		}
	}
	indexed, keyed := 0, 0
	for point := cfg.Point(0); int(point) < prepared.operationPlan.PointCount(); point++ {
		op, ok := prepared.operationPlan.GenericForOperation(point)
		if !ok {
			continue
		}
		iterator, ok := op.Iterator()
		if !ok {
			t.Fatalf("generic-for point %d lost canonical signature iterator", point)
		}
		switch iterator.Kind {
		case iteration.IterateIndexed:
			indexed++
		case iteration.IterateKeyed:
			keyed++
		default:
			t.Fatalf("generic-for point %d iterator=%v", point, iterator)
		}
	}
	if indexed != 28 || keyed != 10 {
		t.Fatalf("signature iterator bindings indexed/keyed = %d/%d, want 28/10", indexed, keyed)
	}
	t.Log("validate_graph exact now: CallSite 33/49, RootAssignment 25/56 (+11 boundary descendants), generic-for 22/38 (+18 reusable descendant bindings), Return 2/5")
}
