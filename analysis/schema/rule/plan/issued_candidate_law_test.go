package plan

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/internal/framing"
)

const issuedCandidateRelation schema.Key = "program-relation/plan-law-issued-row"

// issuedCandidateEntry is the sealed issuance row an issued-row candidate
// resolves against. The plan surface only has to find it; the relation's own
// shape is the issuance surface's law.
type issuedCandidateEntry struct{ key schema.Key }

func (entry issuedCandidateEntry) Key() schema.Key      { return entry.key }
func (entry issuedCandidateEntry) EntryAvailable() bool { return entry.key.Available() }

func (entry issuedCandidateEntry) EntryContent(content *framing.Writer) error {
	return content.String(string(entry.key))
}

// reprovidedCatalog restates one catalog under a different candidate
// authority, leaving every other declaration exactly as authored.
func reprovidedCatalog(t *testing.T, catalog member.Catalog, provider member.CandidateRef) member.Catalog {
	t.Helper()
	relations := make([]member.Relation, len(catalog.Relations))
	for index, relation := range catalog.Relations {
		relation.CandidateProvider = provider
		relations[index] = relation
	}
	projections := make([]member.Projection, len(catalog.Projections))
	for index, projection := range catalog.Projections {
		projection.CandidateProvider = provider
		projections[index] = projection
	}
	restated, ok := member.NewCatalog(catalog.Authorities, catalog.CarrierRefs, relations, projections, catalog.Reducers, catalog.CarryTransforms)
	if !ok {
		t.Fatal("restated member catalog rejected")
	}
	return restated
}

// configureIssuedRouteFixture is the heterogeneous route fixture with its
// candidate taken from a Program row space instead of a Factor axis. Every
// join keeps the same candidate carrier, which is what the compiler holds the
// declaration to when no axis owner states one.
func configureIssuedRouteFixture(t *testing.T) *planFixture {
	t.Helper()
	fixture := configureHeterogeneousRouteFixture(t)
	candidate := member.IssuedRowCandidate(issuedCandidateRelation)
	fixture.declaration.Candidate = candidate
	// The rows a rule joins are addressed by the authority its candidate came
	// from, so moving the candidate to a Program row space moves every join
	// with it. A catalog left half-swapped would describe a rule indexing an
	// axis directory with an ordinal that directory never issued.
	fixture.catalog = reprovidedCatalog(t, fixture.catalog, candidate)
	fixture.otherCatalog = reprovidedCatalog(t, fixture.otherCatalog, candidate)
	fixture.issuance = []schema.Entry{issuedCandidateEntry{key: issuedCandidateRelation}}
	if problem, valid := fixture.declaration.Check(); !valid {
		t.Fatalf("issued-row route declaration rejected: %+v", problem)
	}
	return fixture
}

// TestCompileLowersAnIssuedRowCandidateWithoutAFactorRelation is the plan half
// of the issued-candidate cut. The rule compiles with no candidate relation at
// all: its address stays zero and the plan carries the issuance relation the
// declaration named.
func TestCompileLowersAnIssuedRowCandidateWithoutAFactorRelation(t *testing.T) {
	fixture := configureIssuedRouteFixture(t)
	compiled, failure := Compile(fixture.seal(t))
	if failure.Available() || !compiled.Available() {
		t.Fatalf("issued-row route rejected: failure=%+v", failure)
	}
	planned, ok := compiled.At(0)
	if !ok || !planned.Present() {
		t.Fatal("issued-row plan absent")
	}
	source, issued := planned.IssuedCandidate()
	if !issued || source != issuedCandidateRelation {
		t.Fatalf("compiled issued candidate = %s/%t, want %s", source, issued, issuedCandidateRelation)
	}
	if planned.Candidate() != (RelationAddr{}) {
		t.Fatalf("issued candidate carries a relation address: %+v", planned.Candidate())
	}
	if planned.JoinCount() != 3 {
		t.Fatalf("issued-row plan join count = %d, want 3", planned.JoinCount())
	}
}

// TestAxisCandidateStillCarriesItsRelationAddress keeps the other arm intact
// beside the new one: the same declaration on an axis relation compiles to a
// real address and states no issuance relation.
func TestAxisCandidateStillCarriesItsRelationAddress(t *testing.T) {
	compiled, failure := Compile(configureHeterogeneousRouteFixture(t).seal(t))
	planned, ok := compiled.At(0)
	if failure.Available() || !ok || !planned.Present() {
		t.Fatalf("axis-relation route rejected: failure=%+v", failure)
	}
	if _, issued := planned.IssuedCandidate(); issued {
		t.Fatal("axis-relation plan claims an issued candidate")
	}
	if planned.Candidate() == (RelationAddr{}) {
		t.Fatal("axis-relation plan lost its candidate address")
	}
}

// TestIssuedCandidateRefusesTheExactWriteForm states the law the two arms do
// not share. An exact write is one cell per candidate row addressed through
// the candidate relation's coordinate space, and an issued Program row has
// none, so the pairing cannot be compiled rather than being compiled onto some
// other relation's coordinates.
func TestIssuedCandidateRefusesTheExactWriteForm(t *testing.T) {
	fixture := newPlanFixture(t)
	fixture.declaration.Candidate = member.IssuedRowCandidate(issuedCandidateRelation)
	fixture.issuance = []schema.Entry{issuedCandidateEntry{key: issuedCandidateRelation}}
	if _, failure := Compile(fixture.seal(t)); !failure.Available() {
		t.Fatal("an exact write on an issued-row candidate was admitted")
	}
}

// TestIssuedCandidateResolvesAgainstTheIssuanceSurface proves the reference is
// really resolved: the same declaration refuses when the issuance surface does
// not publish the relation it names.
func TestIssuedCandidateResolvesAgainstTheIssuanceSurface(t *testing.T) {
	fixture := configureIssuedRouteFixture(t)
	fixture.issuance = nil
	if failure := fixture.sealFailure(t); !failure.Available() {
		t.Fatal("an issued-row candidate naming an unpublished relation was admitted")
	}
	// The same declaration seals once the relation is published, so the
	// refusal above is the unresolved reference and nothing else.
	published := configureIssuedRouteFixture(t)
	if failure := published.sealFailure(t); failure.Available() {
		t.Fatalf("a published issued-row candidate was refused: %+v", failure)
	}
}
