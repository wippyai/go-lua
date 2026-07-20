package body

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	"github.com/wippyai/go-lua/analysis/domain/effect/iteration"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func TestValidateGraphTransformerEligibilityCensusIsInstanceExact(t *testing.T) {
	prepared := validateGraphPreparedFixture(t)
	entries := transformer.NewPlanCompiler().EligibilityCensus(prepared.registry, prepared.cfg.Graph, prepared.operationPlan, transformer.Shape{
		Params:   uint32(len(prepared.operationPlan.BoundaryParams())),
		Captures: uint32(len(prepared.operationPlan.BoundaryCaptures())),
		Globals:  uint32(len(prepared.operationPlan.BoundaryGlobals())),
	})
	if len(entries) != 342 {
		t.Fatalf("eligibility entries = %d, want 342 active plan instances/family markers", len(entries))
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
		"RootAssignments":               {total: 56, exact: 55},
		"Returns":                       {total: 5, exact: 5},
		"PathDescendantInvalidations":   {total: 9, exact: 9},
		"PathValuePresenceImplications": {total: 3, exact: 3},
		"DynamicIndexWrites":            {total: 9, exact: 9},
		"CallSites":                     {total: 49, exact: 49},
		// Five source-authored #sequence > 0 guards are normalized to the exact
		// Boolean DAG `#path >= 1`; each distinct CFG branch therefore owns a
		// condition source even though edge exactness is certified only by the
		// whole symbolic CFG compiler below.
		"BranchConditionSources": {total: 58},
		"BranchRefinements":      {total: 57},
		"BranchPathEvidence":     {total: 43},
		"extension:1":            {total: 38, exact: 38},
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
	t.Log("validate_graph exact now uses the canonical State-backed structural environment: CallSite 49/49, RootAssignment 55/56, PathDescendantInvalidation 9/9, DynamicIndexWrite 9/9, PathValuePresenceImplication 3/3, generic-for 38/38, Return 5/5; five exact normalized length guards bring the branch-condition census to 58")
}
