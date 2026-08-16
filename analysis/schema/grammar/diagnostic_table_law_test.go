package grammar

import (
	"testing"

	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/diagnostic"
)

// TestDiagnosticTableSeals states that the authored diagnostic inventory is
// admitted and sealed by the one declaration root, in catalog order.
func TestDiagnosticTableSeals(t *testing.T) {
	sealed, failure := Table()
	if failure.Available() || sealed == nil {
		t.Fatalf("diagnostic table rejected: contributor=%d law=%d disposition=%s", failure.Contributor, failure.Law, failure.Disposition)
	}
	view, viewOK := sealed.Surface(schema.SurfaceKindDiagnostic)
	if !viewOK || view.Count() != len(diagnosticSpecs()) {
		t.Fatalf("sealed diagnostic surface holds %d of %d authored rows", view.Count(), len(diagnosticSpecs()))
	}
}

// TestDiagnosticSurfaceIsSealedAfterTheRuleSurface states the phase law for
// this surface: a diagnostic references the rules and axes its subjects are
// decided by, so it is sealed after both.
func TestDiagnosticSurfaceIsSealedAfterTheRuleSurface(t *testing.T) {
	if schema.SurfaceKindDiagnostic <= schema.SurfaceKindRule || schema.SurfaceKindDiagnostic <= schema.SurfaceKindAxis {
		t.Fatalf("diagnostic catalog ordinal %d does not follow the rule ordinal %d", schema.SurfaceKindDiagnostic, schema.SurfaceKindRule)
	}
}

// TestDiagnosticTableDrivesEveryDerivedView is the drift law at the
// composition: every authored row is reachable through each derived lookup, so
// no consumer holds a second per-code table.
func TestDiagnosticTableDrivesEveryDerivedView(t *testing.T) {
	table, tableOK := Diagnostics()
	if !tableOK {
		t.Fatal("sealed diagnostic table unavailable")
	}
	specs := diagnosticSpecs()
	if table.Count() != len(specs) {
		t.Fatalf("derived table holds %d of %d authored rows", table.Count(), len(specs))
	}
	for position, spec := range specs {
		ordered, orderedOK := table.At(position)
		byCode, byCodeOK := table.ForCode(spec.Code)
		if !orderedOK || !byCodeOK || ordered != byCode || ordered.Code() != spec.Code {
			t.Fatalf("row %q is not reachable through every derived lookup", spec.Code)
		}
		if ordered.Family().String() == "" || ordered.Tier() == diagnostic.TierInvalid {
			t.Fatalf("row %q published no family or tier", spec.Code)
		}
		if spec.Lane != diagnostic.LaneStatic {
			continue
		}
		byObservation, byObservationOK := table.ForStaticObservation(spec.Observation)
		if !byObservationOK || byObservation != ordered {
			t.Fatalf("row %q is not reachable by its static observation population", spec.Code)
		}
	}
}

// TestEveryStaticObservationPopulationIsClaimed states that each artifact
// observation kind an analyzer collector can meet is claimed by exactly one
// declared row, so a mounted row can never be silently dropped.
func TestEveryStaticObservationPopulationIsClaimed(t *testing.T) {
	table, tableOK := Diagnostics()
	if !tableOK {
		t.Fatal("sealed diagnostic table unavailable")
	}
	for _, kind := range []programartifact.DiagnosticObservationKind{
		programartifact.DiagnosticObservationTypeReferenceUnresolved,
		programartifact.DiagnosticObservationValueReferenceUnresolved,
	} {
		entry, known := table.ForStaticObservation(kind)
		if !known || !entry.Collectable() {
			t.Fatalf("static observation population %d is claimed by no collectable row", kind)
		}
	}
	// The branch population is collected on its own lane, so it is deliberately
	// not resolvable as a static row.
	if _, static := table.ForStaticObservation(programartifact.DiagnosticObservationBranchCondition); static {
		t.Fatal("branch condition population resolved as a static row")
	}
}

// TestSolverObservedRowsResolveTheirFactDeclaration states that every row
// decided by solver facts names the declaration that produces them, and that
// the name resolves in the same sealed table.
func TestSolverObservedRowsResolveTheirFactDeclaration(t *testing.T) {
	sealed, failure := Table()
	table, tableOK := Diagnostics()
	if failure.Available() || !tableOK {
		t.Fatal("sealed diagnostic table unavailable")
	}
	for position := 0; position < table.Count(); position++ {
		entry, entryOK := table.At(position)
		if !entryOK {
			t.Fatalf("row %d absent", position)
		}
		reference := entry.Fact()
		if entry.Lane() != diagnostic.LaneBranch {
			if reference.Declared() {
				t.Fatalf("row %q names a fact declaration it never reads", entry.Code())
			}
			continue
		}
		producer, producerOK := sealed.Surface(reference.Surface)
		if !producerOK {
			t.Fatalf("row %q references unsealed surface %d", entry.Code(), reference.Surface)
		}
		if _, resolved := producer.ByID(schema.NewEntryID(reference.Surface, reference.Key)); !resolved {
			t.Fatalf("row %q references undeclared entry %q", entry.Code(), reference.Key)
		}
	}
}
