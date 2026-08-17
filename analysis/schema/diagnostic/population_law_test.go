package diagnostic

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
)

// TestDiagnosticSurfaceRequiresRows states this surface's population law. The
// declaration root admits a registered surface that declares nothing, and
// leaves the question of how many rows a surface must hold to the surface
// itself. This one is the analyzer's whole published vocabulary: every verdict a
// reader receives resolves its row here, so an empty inventory is an incomplete
// declaration and the table says so at seal.
func TestDiagnosticSurfaceRequiresRows(t *testing.T) {
	failure := sealEntries(t, nil)
	if failure.Law != LawSurfacePopulated || failure.Disposition != schema.DispositionIncomplete {
		t.Fatalf("empty diagnostic surface sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	if failure.Contributor != schema.SurfaceKindDiagnostic {
		t.Fatalf("verdict contributor = %d, want the diagnostic surface", failure.Contributor)
	}
}

// TestDiagnosticTableProjectsEverySealedRow is the other half: a populated
// surface yields the derived read model, and it holds exactly the sealed rows.
// The catalog composes that projection from the sealed view, so the law above is
// what makes the projection total rather than a second check at composition.
func TestDiagnosticTableProjectsEverySealedRow(t *testing.T) {
	entries := []*Entry{mustEntry(t, scratchSpec("advice.always_true_guard", FamilyAdvice))}
	sealed, failure := sealSurfaces(t, entries, []schema.Key{"value", "heap"})
	if failure.Available() || sealed == nil {
		t.Fatalf("populated diagnostic surface rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	view, viewOK := sealed.Surface(schema.SurfaceKindDiagnostic)
	if !viewOK {
		t.Fatal("sealed table holds no diagnostic surface")
	}
	table, tableOK := NewTable(view)
	if !tableOK || table.Count() != len(entries) {
		t.Fatalf("derived table holds %d of %d sealed rows", table.Count(), len(entries))
	}
	row, rowOK := table.ForCode(entries[0].Code())
	if !rowOK || row != entries[0] {
		t.Fatal("the derived table does not resolve a sealed row by its published code")
	}
}
