package composite

import (
	"testing"

	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/diagnostic"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// publishedCodes is the analyzer's published diagnostic identity space, spelled
// by the exported constants a consumer resolves a finding by. It is the
// independent statement of what the sealed surface owes: a row declared under
// no published code, or a published code no row declares, is a disagreement
// between the inventory and the identities the analyzer exports.
func publishedCodes() []diagnostic.Code {
	return []diagnostic.Code{
		DiagnosticCodeAlwaysTrueGuard,
		DiagnosticCodeAlwaysFalseGuard,
		DiagnosticCodeRedundantClaim,
		DiagnosticCodeUnresolvedTypeReference,
		DiagnosticCodeUnresolvedValueReference,
		DiagnosticCodeUnusedLocal,
	}
}

// TestDiagnosticTableSeals states that the authored diagnostic inventory is
// admitted and sealed by the one declaration root, and that the sealed surface
// is exactly the published code space.
func TestDiagnosticTableSeals(t *testing.T) {
	sealed, failure := Table()
	if failure.Available() || sealed == nil {
		t.Fatalf("diagnostic table rejected: contributor=%d law=%d disposition=%s", failure.Contributor, failure.Law, failure.Disposition)
	}
	view, viewOK := sealed.Surface(schema.SurfaceKindDiagnostic)
	if !viewOK {
		t.Fatal("sealed table holds no diagnostic surface")
	}
	codes := publishedCodes()
	if view.Count() != len(codes) {
		t.Fatalf("sealed diagnostic surface holds %d rows for %d published codes", view.Count(), len(codes))
	}
	for _, code := range codes {
		if _, declared := view.ByID(schema.NewEntryID(schema.SurfaceKindDiagnostic, schema.Key(code))); !declared {
			t.Fatalf("published code %q is declared by no sealed row", code)
		}
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
	if table.Count() != len(publishedCodes()) {
		t.Fatalf("derived table holds %d rows for %d published codes", table.Count(), len(publishedCodes()))
	}
	for position, spec := range specs {
		ordered, orderedOK := table.At(position)
		byCode, byCodeOK := table.ForCode(spec.Code)
		if !orderedOK || !byCodeOK || ordered != byCode || ordered.Code() != spec.Code {
			t.Fatalf("row %q is not reachable through every derived lookup", spec.Code)
		}
		if !ordered.Family().Declared() || ordered.Tier() == diagnostic.TierInvalid {
			t.Fatalf("row %q published no family or tier", spec.Code)
		}
		if spec.Lane != diagnostic.LaneStatic {
			continue
		}
		byObservation, byObservationOK := table.ForStaticObservation(spec.Observation.Key)
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
	vocabulary, vocabularyOK := StructureVocabulary()
	if !tableOK || !vocabularyOK {
		t.Fatal("sealed diagnostic table unavailable")
	}
	for _, kind := range []programartifact.DiagnosticObservationKind{
		programartifact.DiagnosticObservationTypeReferenceUnresolved,
		programartifact.DiagnosticObservationValueReferenceUnresolved,
	} {
		population, populationOK := vocabulary.At(structure.CategoryDiagnosticObservation, uint16(kind))
		if !populationOK {
			t.Fatalf("artifact observation kind %d names no declared population", kind)
		}
		entry, known := table.ForStaticObservation(population.Key())
		if !known || !entry.Collectable() {
			t.Fatalf("static observation population %q is claimed by no collectable row", population.Key())
		}
	}
	// The branch population is collected on its own lane, so it is deliberately
	// not resolvable as a static row.
	branch, branchOK := vocabulary.At(structure.CategoryDiagnosticObservation, uint16(programartifact.DiagnosticObservationBranchCondition))
	if !branchOK {
		t.Fatal("artifact branch observation kind names no declared population")
	}
	if _, static := table.ForStaticObservation(branch.Key()); static {
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
