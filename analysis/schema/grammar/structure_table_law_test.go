package grammar

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// TestStructureTableSeals states that the authored structural vocabulary is
// admitted and sealed by the one declaration root, and that it projects back at
// the ordinals it was declared under: the projection a consumer switches on is
// the authored table itself, never a restatement of it.
func TestStructureTableSeals(t *testing.T) {
	sealed, failure := Table()
	if failure.Available() || sealed == nil {
		t.Fatalf("declaration table rejected: contributor=%d law=%d disposition=%s", failure.Contributor, failure.Law, failure.Disposition)
	}
	view, viewOK := sealed.Surface(schema.SurfaceKindStructure)
	if !viewOK || view.Count() != len(structureSpecs()) {
		t.Fatalf("sealed structural surface holds %d of %d authored rows", view.Count(), len(structureSpecs()))
	}
	table, tableOK := structure.NewTable(view)
	if !tableOK {
		t.Fatal("sealed structural vocabulary did not project")
	}
	for category, size := range map[structure.Category]int{
		structure.CategoryArm:     8,
		structure.CategoryEvent:   3,
		structure.CategoryOutcome: 7,
	} {
		if table.Count(category) != size {
			t.Fatalf("category %d projected %d of %d members", category, table.Count(category), size)
		}
	}
	for _, spec := range structureSpecs() {
		member, memberOK := table.At(spec.Category, spec.Ordinal)
		if !memberOK || member.Key() != spec.Key {
			t.Fatalf("member %q is not reachable at its declared ordinal", spec.Key)
		}
	}
}
