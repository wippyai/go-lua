package composite

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
)

// TestPlacementSuspensionBindHotAdmitsAnEmptyMountedCatalog exercises the
// Link binding path for the suspension consumers themselves. A scalar-only
// mounted artifact has no subject-liveness rows, so this isolates schema/owner
// admission from catalog contents while still requiring both the class and
// evidence BindHot implementations to install their selected-read surfaces.
func TestPlacementSuspensionBindHotAdmitsAnEmptyMountedCatalog(t *testing.T) {
	record := mountedRecord(t, "placement-suspension-bind", "return 1")
	compilation, compilationOK := Build()
	if !compilationOK {
		t.Fatal("program schema compilation is unavailable")
	}
	bound, failure := BindProgram(compilation, record)
	if failure.Available() || bound == nil || !bound.Available() {
		t.Fatalf("bind mounted suspension consumers: %v", failure)
	}
	rules := bound.Rules()
	if rules == nil {
		t.Fatal("sealed rule binding is unavailable")
	}
	for _, key := range []schema.Key{"placement-suspension", "placement-suspension-evidence"} {
		if _, programOK := rules.ProgramRuleByKey(key); !programOK {
			t.Fatalf("%q did not publish a sealed Program rule", key)
		}
		catalog, catalogOK := rules.LinkCatalogByKey(key)
		if !catalogOK || catalog == nil || catalog.Count() != 0 {
			count := -1
			if catalog != nil {
				count = catalog.Count()
			}
			t.Fatalf("%q Link catalog = %v/%t count=%d, want an empty sealed catalog", key, catalog, catalogOK, count)
		}
	}
}
