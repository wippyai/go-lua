package analysis

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/query"
	"github.com/wippyai/go-lua/domain/composite"
)

// Query construction enumerates the sealed query table. Family names stay on
// their owning registrations.

func TestQueryConstructionUsesSealedQueryFamilies(t *testing.T) {
	issued := composite.QueryIssuance()
	if len(issued) == 0 {
		t.Fatal("sealed table declares no query families")
	}
	seen := make(map[string]bool, len(issued))
	for _, family := range issued {
		if !family.Family.Available() || !family.Authority.Available() || !family.Population.Available() || !family.Projection.Available() {
			t.Fatalf("issued family %q is not a complete sealed query identity", family.Family)
		}
		if family.Authority != family.Family {
			t.Fatalf("family %q publishes authority %q, not its declaration key", family.Family, family.Authority)
		}
		if family.Population != query.PopulationSelectedPoint {
			t.Fatalf("family %q is asked at %q, not the selected-point population", family.Family, family.Population)
		}
		if family.Projection != query.ProjectionSummary && family.Projection != query.ProjectionExact {
			t.Fatalf("family %q declares projection %q", family.Family, family.Projection)
		}
		if seen[string(family.Family)] {
			t.Fatalf("family %q is issued twice", family.Family)
		}
		seen[string(family.Family)] = true
	}
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("analysis source location")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "artifact_query_plan.go"))
	if err != nil {
		t.Fatalf("read artifact_query_plan.go: %v", err)
	}
	for _, literal := range []string{`"value-summary"`, `"effect-exact"`} {
		if strings.Contains(string(src), literal) {
			t.Fatalf("artifact_query_plan.go restates query family %s; construction walks composite.QueryIssuance", literal)
		}
	}
	for _, call := range []string{"ValueQuery()", "EffectQuery()"} {
		if strings.Contains(string(src), call) {
			t.Fatalf("artifact_query_plan.go calls binding.%s; construction attaches through ProgramBinding.Query", call)
		}
	}
}
