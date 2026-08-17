package module

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
)

func TestModuleCountRowsMatchSealedGeometry(t *testing.T) {
	component := sealModuleFixture(t)
	rows, ok := component.CountRows()
	if !ok || !rows.Available() || rows.Count() != 8 {
		t.Fatalf("Module CountRows = %d/%t, want 8/true", rows.Count(), ok)
	}
	ids := denominator.GeneratedLinkModuleIDs()
	want := []struct {
		id    schema.EntryID
		count int
	}{
		{ids.LinkModule, component.Cache().EntryCount()},
		{ids.LinkModuleCache, component.Cache().InstanceCount()},
		{ids.LinkModuleRepresentative, component.Cache().InstanceCount()},
		{ids.LinkModuleTransport, component.Coordinates().Count()},
		{ids.LinkModuleAnalysisRoot, component.Roots().Count()},
		{ids.LinkModuleInitGeneration, component.Generations().Count()},
		{ids.LinkModuleInitOutcome, moduleOutcomeCount(component)},
		{ids.LinkModuleInitTerminal, component.Terminals().Count()},
	}
	for _, row := range want {
		if got, ok := rows.Value(row.id); !ok || got != uint64(row.count) {
			t.Fatalf("Module denominator %v = %d/%t, want %d/true", row.id, got, ok, row.count)
		}
	}
}

func moduleOutcomeCount(component *Component) int {
	total := 0
	for index := 0; index < component.Generations().Count(); index++ {
		generation, ok := component.Generations().At(index)
		if !ok {
			continue
		}
		total += component.Outcomes().Count(generation)
	}
	return total
}
