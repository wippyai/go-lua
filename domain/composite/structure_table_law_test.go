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
	compilation, compilationOK := Build()
	if !compilationOK {
		t.Fatal("compilation unavailable")
	}
	sealed, failure := Table(compilation)
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
	specs := structureSpecs()
	if view.Count() != len(specs) {
		t.Fatalf("sealed structural surface holds %d rows for %d declared vocabulary members", view.Count(), len(specs))
	}
	// A member is reachable at the position the aggregation numbers it, and a
	// contributor that authors an ordinal is naming that same position: a
	// category whose ordinals a foreign spelling is pinned to declares them, and
	// a category resolved by name alone leaves them to the aggregation.
	positions := make(map[structure.Category]uint16)
	for _, spec := range specs {
		positions[spec.Category]++
		position := positions[spec.Category]
		if spec.Ordinal != 0 && spec.Ordinal != position {
			t.Fatalf("member %q authors ordinal %d at position %d of its category", spec.Key, spec.Ordinal, position)
		}
		member, memberOK := table.At(spec.Category, position)
		if !memberOK || member.Key() != spec.Key {
			t.Fatalf("member %q is not reachable at its declared ordinal", spec.Key)
		}
		if member.Spelling() != spec.Spelling {
			t.Fatalf("member %q renders as %q, declared %q", spec.Key, member.Spelling(), spec.Spelling)
		}
	}
	for category, count := range positions {
		if table.Count(category) != int(count) {
			t.Fatalf("category %d projected %d of %d declared members", category, table.Count(category), count)
		}
	}
}

// TestStructureSurfaceSealsFirstInTheCatalog states the phase law for this
// surface: it hosts the closed identity vocabularies the rest of the catalog
// names members of, and it names no surface itself, so no surface precedes it
// and every surface above it may resolve against it.
func TestStructureSurfaceSealsFirstInTheCatalog(t *testing.T) {
	for kind := schema.SurfaceKind(1); kind.Available(); kind++ {
		if kind < schema.SurfaceKindStructure {
			t.Fatalf("surface ordinal %d precedes the structural ordinal %d", kind, schema.SurfaceKindStructure)
		}
	}
}

// TestStructureTableIsHostedFromItsContributions states that the analyzer's
// vocabulary is an aggregate rather than one authored list: each contribution
// declares its own rows, the surface numbers the result densely per category,
// and the sealed table holds exactly what the contributions declared.
func TestStructureTableIsHostedFromItsContributions(t *testing.T) {
	contributions := structureContributions()
	if len(contributions) < 2 {
		t.Fatal("the structural vocabulary is hosted from a single contribution")
	}
	declared := 0
	for _, contribution := range contributions {
		if len(contribution) == 0 {
			t.Fatal("a contribution declares no rows")
		}
		declared += len(contribution)
	}
	entries, ok := structureEntries()
	if !ok || len(entries) != declared {
		t.Fatalf("the contributions collected %d rows of %d, ok=%t", len(entries), declared, ok)
	}
	counts := make(map[structure.Category]uint16, len(entries))
	for _, entry := range entries {
		counts[entry.Category()]++
		if entry.Ordinal() != counts[entry.Category()] {
			t.Fatalf("member %q holds ordinal %d at position %d of its category", entry.Key(), entry.Ordinal(), counts[entry.Category()])
		}
		if entry.Spelling() == "" {
			t.Fatalf("member %q was collected without a rendered name", entry.Key())
		}
	}
}
