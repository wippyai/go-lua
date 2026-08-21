package module

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
)

func TestModuleCountRowsMatchSealedGeometry(t *testing.T) {
	component := sealModuleFixture(t)
	rows, ok := component.CountRows()
	if !ok || !rows.Available() || rows.Count() != 4 {
		t.Fatalf("Module CountRows = %d/%t, want 4/true", rows.Count(), ok)
	}
	ids := denominator.GeneratedLinkModuleIDs()
	want := []struct {
		id    schema.EntryID
		count int
	}{
		{ids.LinkModule, len(component.authority.spec.ModuleCacheEntries)},
		{ids.LinkModuleCache, component.Cache().InstanceCount()},
		{ids.LinkModuleRepresentative, component.Cache().InstanceCount()},
		{ids.LinkModuleAnalysisRoot, component.Roots().Count()},
	}
	for _, row := range want {
		if got, ok := rows.Value(row.id); !ok || got != uint64(row.count) {
			t.Fatalf("Module denominator %v = %d/%t, want %d/true", row.id, got, ok, row.count)
		}
	}
}

func TestModuleDenominatorFormsMatchAuthoredOwnership(t *testing.T) {
	primary, ok := denominator.GeneratedRelationByKey(schema.Key("LinkModule@-"))
	if !ok || primary.Form() != denominator.RelationFormAuthored || len(primary.Parents()) != 0 {
		t.Fatal("LinkModule primary is not an authored, parentless relation")
	}
	cache, ok := denominator.GeneratedRelationByKey(schema.Key("LinkModule@LinkModuleCache"))
	if !ok || cache.Form() != denominator.RelationFormAuthored || len(cache.Parents()) != 0 {
		t.Fatal("LinkModule cache is not an authored, parentless relation")
	}
	representative, ok := denominator.GeneratedRelationByKey(schema.Key("LinkModule@LinkModuleRepresentative"))
	if !ok || representative.Form() != denominator.RelationFormSealDerived || len(representative.Parents()) != 1 || representative.Parents()[0] != cache.ID() {
		t.Fatal("LinkModule representative is not derived from the cache relation")
	}
	root, ok := denominator.GeneratedRelationByKey(schema.Key("LinkModule@LinkModuleAnalysisRoot"))
	if !ok || root.Form() != denominator.RelationFormAuthored || len(root.Parents()) != 0 {
		t.Fatal("LinkModule analysis root is not an authored, parentless relation")
	}
}
