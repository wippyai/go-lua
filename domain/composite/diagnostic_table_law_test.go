package composite

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/diagnostic"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	typedomain "github.com/wippyai/go-lua/domain/type"
)

// publishedCodes is the analyzer's published diagnostic identity space, spelled
// by the exported constants a consumer resolves a finding by. It is the
// independent statement of what the sealed surface owes: a row declared under
// no published code, or a published code no row declares, is a disagreement
// between the inventory and the identities the analyzer exports.
//
// The last is spelled by the domain that declares it. A domain-declared row
// owns its code the way it owns its row, so the composition names the domain's
// constant rather than restating the string beside it.
func publishedCodes() []diagnostic.Code {
	return []diagnostic.Code{
		DiagnosticCodeAlwaysTrueGuard,
		DiagnosticCodeAlwaysFalseGuard,
		DiagnosticCodeRedundantClaim,
		DiagnosticCodeUnresolvedTypeReference,
		DiagnosticCodeUnresolvedValueReference,
		DiagnosticCodeUnusedLocal,
		typedomain.Code,
		typedomain.CallArgumentCode,
		typedomain.ChannelSelectExhaustivenessCode,
	}
}

// TestDiagnosticTableSeals states that the authored diagnostic inventory is
// admitted and sealed by the one declaration root, and that the sealed surface
// is exactly the published code space.
func TestDiagnosticTableSeals(t *testing.T) {
	compilation, compilationOK := Build()
	if !compilationOK {
		t.Fatal("compilation unavailable")
	}
	sealed, failure := Table(compilation)
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
	compilation, compilationOK := Build()
	if !compilationOK {
		t.Fatal("compilation unavailable")
	}
	table, tableOK := Diagnostics(compilation)
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

// TestEveryStaticObservationPopulationIsClaimed states that each canonical
// static observation population an analyzer collector can meet is claimed by
// exactly one declared row, so a mounted row can never be silently dropped.
func TestEveryStaticObservationPopulationIsClaimed(t *testing.T) {
	compilation, compilationOK := Build()
	if !compilationOK {
		t.Fatal("compilation unavailable")
	}
	table, tableOK := Diagnostics(compilation)
	vocabulary, vocabularyOK := StructureVocabulary(compilation)
	if !tableOK || !vocabularyOK {
		t.Fatal("sealed diagnostic table unavailable")
	}
	for _, kind := range []structure.DiagnosticObservationKind{
		structure.DiagnosticObservationTypeReferenceUnresolved,
		structure.DiagnosticObservationValueReferenceUnresolved,
	} {
		population, populationOK := structure.DiagnosticObservationEntry(vocabulary, kind)
		if !populationOK {
			t.Fatalf("artifact observation kind %d names no declared population", kind)
		}
		entry, known := table.ForStaticObservation(population.Key())
		if !known || !entry.Collectable() {
			t.Fatalf("static observation population %q is claimed by no collectable row", population.Key())
		}
	}
	// The branch populations are collected on their own lane, so they are
	// deliberately not resolvable as static rows.
	for _, kind := range []structure.DiagnosticObservationKind{
		structure.DiagnosticObservationBranchCondition,
		structure.DiagnosticObservationTypeConformance,
	} {
		branch, branchOK := structure.DiagnosticObservationEntry(vocabulary, kind)
		if !branchOK {
			t.Fatalf("artifact observation kind %d names no declared population", kind)
		}
		if _, static := table.ForStaticObservation(branch.Key()); static {
			t.Fatalf("branch population %q resolved as a static row", branch.Key())
		}
	}
}

// TestSolverObservedRowsResolveTheirFactDeclaration states that every row
// decided by solver facts names the declaration that produces them, and that
// the name resolves in the same sealed table.
func TestSolverObservedRowsResolveTheirFactDeclaration(t *testing.T) {
	compilation, compilationOK := Build()
	if !compilationOK {
		t.Fatal("compilation unavailable")
	}
	sealed, failure := Table(compilation)
	table, tableOK := Diagnostics(compilation)
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

// TestDiagnosticCollectionDirectoryIsTotalOverBranchRows states that every
// solver-observed diagnostic names a collection family and that the post-seal
// directory joins that family to the issued query or observation inventory.
func TestDiagnosticCollectionDirectoryIsTotalOverBranchRows(t *testing.T) {
	compilation, compilationOK := Build()
	table, tableOK := Diagnostics(compilation)
	directory, directoryOK := compilation.DiagnosticCollections()
	if !tableOK || !compilationOK || !directoryOK {
		t.Fatal("diagnostic collection directory unavailable")
	}
	want := 0
	for position := 0; position < table.Count(); position++ {
		entry, entryOK := table.At(position)
		if entryOK && entry.Lane() == diagnostic.LaneBranch {
			want++
		}
	}
	if directory.Count() != want || want == 0 {
		t.Fatalf("directory holds %d rows for %d branch diagnostics", directory.Count(), want)
	}
	seen := make(map[diagnostic.Code]bool, directory.Count())
	for position := 0; position < directory.Count(); position++ {
		row, rowOK := directory.At(position)
		if !rowOK {
			t.Fatalf("directory row %d unavailable", position)
		}
		if seen[row.Code] {
			t.Fatalf("collection row %q issued twice", row.Code)
		}
		seen[row.Code] = true
		if !row.Collection.Available() || !row.Producer.Available() {
			t.Fatalf("collection row %q is incomplete", row.Code)
		}
		switch row.Collection.Surface {
		case schema.SurfaceKindObservation:
			if !row.Geometry.Available() || !row.Anchor.Available() || !row.Codec.Available() {
				t.Fatalf("observation collection %q lost geometry, anchor, or codec", row.Code)
			}
		case schema.SurfaceKindQuery:
			if !row.Projection.Available() {
				t.Fatalf("query collection %q lost projection", row.Code)
			}
		default:
			t.Fatalf("collection row %q names surface %d", row.Code, row.Collection.Surface)
		}
	}
	assign, assignOK := table.ForBranchObservation(typedomain.ObservationKey, diagnostic.SiteAssignment)
	call, callOK := table.ForBranchObservation(typedomain.ObservationKey, diagnostic.SiteCallArgument)
	if !assignOK || !callOK || assign.Code() != typedomain.Code || call.Code() != typedomain.CallArgumentCode {
		t.Fatal("branch observation index lost the type-conformance sites")
	}
	for position := 0; position < directory.Count(); position++ {
		row, rowOK := directory.At(position)
		if !rowOK {
			t.Fatalf("directory row %d unavailable", position)
		}
		if row.Code == typedomain.Code && (!collectionHasSite(row, diagnostic.SiteAssignment) || !row.Population.Available()) {
			t.Fatal("assignment collection lost its site or population")
		}
		if row.Code == typedomain.CallArgumentCode && !collectionHasSite(row, diagnostic.SiteCallArgument) {
			t.Fatal("call-argument collection lost its site")
		}
	}
}

func collectionHasSite(row DiagnosticCollection, want diagnostic.Site) bool {
	for position := 0; position < row.SiteCount(); position++ {
		site, ok := row.SiteAt(position)
		if ok && site == want {
			return true
		}
	}
	return false
}

// TestBranchDiagnosticsCollectFromTheirOwnPopulationObservation states where a
// solver-observed diagnostic may read its measured value from. The value a
// branch-lane row judges is produced by a rule occurrence, and the only column
// published at that occurrence is the observation family issued for the row's
// own population. A point-keyed query column is published at selected points
// only, so a row that named one would read an absent coordinate at every site
// and abstain silently instead of judging.
func TestBranchDiagnosticsCollectFromTheirOwnPopulationObservation(t *testing.T) {
	compilation, compilationOK := Build()
	directory, directoryOK := compilation.DiagnosticCollections()
	if !compilationOK || !directoryOK || !directory.Available() {
		t.Fatal("diagnostic collection directory unavailable")
	}
	issued := make(map[schema.Key]IssuedObservation, len(ObservationIssuance(compilation)))
	for _, observation := range ObservationIssuance(compilation) {
		issued[observation.Key] = observation
	}
	for position := 0; position < directory.Count(); position++ {
		row, rowOK := directory.At(position)
		if !rowOK {
			t.Fatalf("directory row %d unavailable", position)
		}
		if row.Collection.Surface != schema.SurfaceKindObservation {
			t.Fatalf("branch diagnostic %q collects from surface %d; a produced value is only published on its observation family",
				row.Code, row.Collection.Surface)
		}
		observation, found := issued[row.Collection.Key]
		if !found {
			t.Fatalf("branch diagnostic %q names unissued observation %q", row.Code, row.Collection.Key)
		}
		if !row.Population.Available() {
			t.Fatalf("branch diagnostic %q names no population", row.Code)
		}
		if observation.Population.Kind.Key != row.Population {
			t.Fatalf("branch diagnostic %q measures population %q but collects observation %q of population %q",
				row.Code, row.Population, observation.Key, observation.Population.Kind.Key)
		}
	}
}
