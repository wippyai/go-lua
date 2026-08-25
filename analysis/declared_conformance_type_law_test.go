package analysis

import (
	"testing"

	anadiag "github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/runtimekind"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// declaredConformanceCase is one fixture whose single annotated local fixes
// exactly what its conformance site must be measured against.
type declaredConformanceCase struct {
	name     string
	may      runtimekind.Set
	spelling string
}

// TestDeclaredConformanceColumnPublishesWhatEachDeclarationAdmits is the
// declared half of the conformance judgment. A site is judged by containment
// of the families its value may carry in the families its declaration admits,
// so the declaration has to publish those families: a record admits a table, a
// union admits its members' families, and a primitive admits its own.
//
// The whole vocabulary is the abstention this judgment gives an unnarrowed
// declaration. A structural declaration answered with it is not an abstention
// but a lost distinction, and every conformance site behind it stops being
// decidable at all.
func TestDeclaredConformanceColumnPublishesWhatEachDeclarationAdmits(t *testing.T) {
	contract := fixtureContract(t)
	for _, testCase := range []declaredConformanceCase{
		{name: "core/type-is-falsy-excludes", may: runtimekind.Bit(runtimekind.Table), spelling: "record"},
		{name: "types/union-mismatch", may: runtimekind.Bit(runtimekind.String) | runtimekind.Bit(runtimekind.Number), spelling: "StringOrNumber"},
		{name: "errors/type-mismatch", may: runtimekind.Bit(runtimekind.Number) | runtimekind.Bit(runtimekind.String) | runtimekind.Bit(runtimekind.Boolean), spelling: ""},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			linked, err := testfixture.SealCorpusProject(contract, fixtureProject(t, testCase.name))
			if err != nil {
				t.Fatalf("seal fixture: %v", err)
			}
			plan, status, diagnostics := CompileWithDiagnostics(linked)
			if status != CompileComplete || plan == nil || plan.state == nil || plan.state.artifacts == nil {
				t.Fatalf("compile fixture = %v diagnostics=%+v", status, diagnostics)
			}
			t.Cleanup(func() { plan.Close() })
			observations, observationsOK := plan.state.artifacts.observationCensus()
			if !observationsOK {
				t.Fatal("compile diagnostic observations")
			}
			rows := make([]anadiag.Observation, 0, len(observations))
			for _, observation := range observations {
				if observation.Kind == structure.DiagnosticObservationTypeConformance {
					rows = append(rows, observation)
				}
			}
			if len(rows) == 0 {
				t.Fatal("the fixture's annotated assignment issued no conformance site")
			}
			var admitted runtimekind.Set
			for _, row := range rows {
				if !row.Conformance.DeclaredMay.Valid() {
					t.Fatalf("declared may-set %d is outside the closed runtime vocabulary", row.Conformance.DeclaredMay)
				}
				if row.Conformance.DeclaredMay == runtimekind.All {
					t.Fatalf("declaration %s publishes the whole runtime vocabulary, so its site is undecidable", row.Conformance.Declared)
				}
				if row.Conformance.Target == "" {
					t.Fatalf("declaration %s publishes no spelling to name it by", row.Conformance.Declared)
				}
				if testCase.spelling != "" && row.Conformance.Target != testCase.spelling {
					t.Fatalf("declaration spelling = %q, want %q", row.Conformance.Target, testCase.spelling)
				}
				admitted |= row.Conformance.DeclaredMay
			}
			if admitted != testCase.may {
				t.Fatalf("the fixture's declarations admit %d, want %d", admitted, testCase.may)
			}
		})
	}
}

// TestUnresolvedDeclarationRefusesTheDeclaredColumn states what the projection
// owes the conformance judgment when the type authority does not resolve the
// declaration at all. The whole runtime vocabulary is the abstention an
// unnarrowed declaration earns, and a conformance site measured against it
// conforms for every observation. An unresolved declaration has earned nothing:
// the projection knows only that it cannot state the declaration, so it must
// refuse the row rather than publish the one mask that proves every site.
func TestUnresolvedDeclarationRefusesTheDeclaredColumn(t *testing.T) {
	unheld, derived := identity.DeriveContentID("analysis/declared-conformance-law", []byte("declaration no authority holds"))
	if !derived {
		t.Fatal("derive a declaration identity")
	}
	t.Run("no authority", func(t *testing.T) {
		row, projected := declaredTypeProjection(nil, unheld)
		if projected {
			t.Fatalf("projection published %+v for a declaration no authority can resolve", row)
		}
	})
	t.Run("declaration outside the sealed authority", func(t *testing.T) {
		contract := fixtureContract(t)
		linked, err := testfixture.SealCorpusProject(contract, fixtureProject(t, "core/type-is-falsy-excludes"))
		if err != nil {
			t.Fatalf("seal fixture: %v", err)
		}
		plan, status, diagnostics := CompileWithDiagnostics(linked)
		if status != CompileComplete || plan == nil || plan.state == nil || plan.state.artifacts == nil {
			t.Fatalf("compile fixture = %v diagnostics=%+v", status, diagnostics)
		}
		t.Cleanup(func() { plan.Close() })
		types := plan.state.artifacts.types
		if types == nil || types.Count() == 0 {
			t.Fatal("compile sealed type authority")
		}
		if _, held := types.FindByReferenceID(unheld); held {
			t.Fatal("the authority holds the declaration this law drives as unresolved")
		}
		row, projected := declaredTypeProjection(types, unheld)
		if projected {
			t.Fatalf("projection published %+v for a declaration the sealed authority does not hold", row)
		}
	})
}
