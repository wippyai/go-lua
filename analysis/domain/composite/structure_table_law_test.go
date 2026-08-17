package composite

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
	if !viewOK {
		t.Fatal("sealed table holds no structural surface")
	}
	table, tableOK := structure.NewTable(view)
	if !tableOK {
		t.Fatal("sealed structural vocabulary did not project")
	}
	// The sizes are the vocabularies the analyzer consolidates here: the eight
	// structural arms, the three bracket events, the seven body outcomes, the
	// eight Lua runtime families, and the ten symbolic expression forms. They are
	// stated independently of the authored inventory, so a member added or
	// dropped on either side is a verdict rather than a table that agrees with
	// itself.
	sizes := map[structure.Category]int{
		structure.CategoryArm:            8,
		structure.CategoryEvent:          3,
		structure.CategoryOutcome:        7,
		structure.CategoryRuntimeKind:    8,
		structure.CategoryConstraintForm: 10,
	}
	declared := 0
	for category, size := range sizes {
		if table.Count(category) != size {
			t.Fatalf("category %d projected %d of %d members", category, table.Count(category), size)
		}
		declared += size
	}
	if view.Count() != declared {
		t.Fatalf("sealed structural surface holds %d rows for %d vocabulary members", view.Count(), declared)
	}
	for _, spec := range structureSpecs() {
		member, memberOK := table.At(spec.Category, spec.Ordinal)
		if !memberOK || member.Key() != spec.Key {
			t.Fatalf("member %q is not reachable at its declared ordinal", spec.Key)
		}
	}
}
