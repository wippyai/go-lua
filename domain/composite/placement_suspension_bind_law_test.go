package composite

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
)

// TestPlacementSuspensionBindHotAdmitsAnEmptyMountedCatalog exercises the
// mounted binding path for the suspension consumers themselves. A scalar-only
// mounted artifact has no subject-liveness rows, so this isolates schema/owner
// admission from catalog contents while still requiring both the class and
// evidence BindHot implementations to install their Call gate and
// selected-read surfaces.
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
		cell, cellOK := rules.cellByKey(key)
		capability, capabilityOK := rules.CapabilityByKey(key)
		if !cellOK || !cell.Available() || !capabilityOK || !capability.Mounted() {
			t.Fatalf("%q did not publish its sealed canonical mounted cell and capability", key)
		}
		// A mounted consumer is issued its operands by Program, so it publishes
		// no Link occurrence inventory of its own.
		if catalog, catalogOK := rules.OccurrenceCatalogByKey(key); catalogOK || catalog != nil {
			t.Fatalf("%q published a Link occurrence catalog = %v/%t, want none", key, catalog, catalogOK)
		}
	}
}
