package equation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/identity"
	queryschema "github.com/wippyai/go-lua/analysis/schema/query"
)

// twoSelectedFamilyQuerySource seals a composition whose graph query
// denominator has two selected-point families, so a row set that covers only
// one of them is short of the declared coverage.
func twoSelectedFamilyQuerySource(t testing.TB) (*composition.Composition, composition.Key, composition.Key, composition.Key) {
	t.Helper()
	factor, first, second := boundaryKey(240), boundaryKey(241), boundaryKey(242)
	source, ok := composition.Seal(composition.Candidate{
		Factors: []composition.Factor{{Key: factor}},
		Queries: []composition.QueryFamily{
			{Key: first, Freezer: boundaryKey(243), Population: queryschema.PopulationKindSelectedPoint,
				Projections: []composition.QueryProjection{{Kind: composition.QueryFactorExact, Factor: factor}}},
			{Key: second, Freezer: boundaryKey(244), Population: queryschema.PopulationKindSelectedPoint,
				Projections: []composition.QueryProjection{{Kind: composition.QueryFactorExact, Factor: factor}}},
		},
	})
	if !ok || source == nil {
		t.Fatal("two-family query composition")
	}
	return source, factor, first, second
}

// One refused query plane names the boundary that refused it. A caller that
// receives a compile refusal can tell an inventory shortfall from an
// undeclared point, an unauthenticated row, a duplicate coordinate, and an
// uncovered selected-point family; without distinct sites all five arrive as
// one opaque digest and no consumer can act on any of them.
func TestQueryPlaneRefusalsNameDistinctBoundaries(t *testing.T) {
	fixture := newQueryContextFixture(t)
	zeroContext := fixture.row
	zeroContext.Context = identity.ContentID{}
	undeclaredPoint := fixture.row
	undeclaredPoint.Point = PointAt(7)

	querylessSource, querylessOK := composition.Seal(composition.Candidate{Factors: []composition.Factor{{Key: boundaryKey(245)}}})
	if !querylessOK || querylessSource == nil {
		t.Fatal("queryless composition")
	}
	twoFamily, twoFactor, firstFamily, _ := twoSelectedFamilyQuerySource(t)
	coverageRow := QueryInstance{
		Context: boundaryContext(246), Family: firstFamily, Point: PointAt(0),
		Surfaces: []Surface{{Factor: twoFactor, Form: SurfaceReadExact, Local: 1}},
	}
	secondCoverageRow := coverageRow
	secondCoverageRow.Context = boundaryContext(247)

	cases := []struct {
		name   string
		source *composition.Composition
		rows   []QueryInstance
	}{
		{"row-without-family", querylessSource, []QueryInstance{fixture.row}},
		{"inventory-short", fixture.source, nil},
		{"undeclared-point", fixture.source, []QueryInstance{undeclaredPoint}},
		{"unauthenticated-row", fixture.source, []QueryInstance{zeroContext}},
		{"duplicate-coordinate", fixture.source, []QueryInstance{fixture.row, fixture.row}},
		{"uncovered-family", twoFamily, []QueryInstance{coverageRow, secondCoverageRow}},
	}
	sites := make(map[identity.ContentID]string, len(cases))
	for _, unit := range cases {
		queries, failure, accepted := buildQueries(unit.source, fixture.declared, unit.rows, topologyCatalog{})
		if accepted || queries != nil {
			t.Fatalf("%s: refused plane was accepted", unit.name)
		}
		if !failure.Available() || failure.Family != SealFailureFamilyCompile {
			t.Fatalf("%s: refusal = %s", unit.name, failure)
		}
		if previous, collided := sites[failure.Site]; collided && previous != unit.name {
			t.Fatalf("%s shares its refusal boundary with %s: %s", unit.name, previous, failure)
		}
		sites[failure.Site] = unit.name
	}
	if len(sites) != 6 {
		t.Fatalf("query refusal boundaries = %d, want 6 distinct sites over %d cases", len(sites), len(cases))
	}
}

// An accepted query plane reports no failure, so a caller never reads a
// boundary out of a plane that compiled.
func TestAcceptedQueryPlaneCarriesNoBoundary(t *testing.T) {
	fixture := newQueryContextFixture(t)
	queries, failure, accepted := buildQueries(fixture.source, fixture.declared, []QueryInstance{fixture.row}, topologyCatalog{})
	if !accepted || len(queries) != 1 {
		t.Fatal("declared query plane refused")
	}
	if failure.Available() {
		t.Fatalf("accepted plane carried a boundary: %s", failure)
	}
}
