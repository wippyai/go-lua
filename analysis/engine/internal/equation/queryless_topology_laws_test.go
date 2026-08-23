package equation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	queryschema "github.com/wippyai/go-lua/analysis/schema/query"
)

// TestQuerylessTopologyCoverageAndEmptyDemandLaw pins the transformer-local
// base case: no cold query families admit no query instances, while every
// nonempty family inventory still requires complete instance coverage. The
// resulting empty Graph is structurally valid but has no Demand cone.
func TestQuerylessTopologyCoverageAndEmptyDemandLaw(t *testing.T) {
	zeroSource, zeroOK := composition.Seal(composition.Candidate{
		Factors: []composition.Factor{{Key: boundaryKey(201)}},
	})
	if !zeroOK || zeroSource == nil {
		t.Fatal("queryless source")
	}
	zeroQueries, _, accepted := buildQueries(zeroSource, nil, nil, topologyCatalog{})
	if !accepted || len(zeroQueries) != 0 {
		t.Fatal("0 families / 0 instances was rejected")
	}
	if _, _, accepted = buildQueries(zeroSource, nil, []QueryInstance{{}}, topologyCatalog{}); accepted {
		t.Fatal("0 families / 1 instance was accepted")
	}

	factor := boundaryKey(202)
	familyA, familyB := boundaryKey(203), boundaryKey(204)
	withOneFamily, oneFamilyOK := composition.Seal(composition.Candidate{
		Factors: []composition.Factor{{Key: factor}},
		Queries: []composition.QueryFamily{{
			Key: familyA, Freezer: boundaryKey(205),
			Population:  queryschema.PopulationKindSelectedPoint,
			Projections: []composition.QueryProjection{{Kind: composition.QueryFactorExact, Factor: factor}},
		}},
	})
	if !oneFamilyOK || withOneFamily == nil {
		t.Fatal("one-family source")
	}
	if _, _, accepted = buildQueries(withOneFamily, nil, nil, topologyCatalog{}); accepted {
		t.Fatal("1 family / 0 instances was accepted")
	}

	withTwoFamilies, twoFamiliesOK := composition.Seal(composition.Candidate{
		Factors: []composition.Factor{{Key: factor}},
		Queries: []composition.QueryFamily{
			{Key: familyA, Freezer: boundaryKey(206), Population: queryschema.PopulationKindSelectedPoint, Projections: []composition.QueryProjection{{Kind: composition.QueryFactorExact, Factor: factor}}},
			{Key: familyB, Freezer: boundaryKey(207), Population: queryschema.PopulationKindSelectedPoint, Projections: []composition.QueryProjection{{Kind: composition.QueryFactorExact, Factor: factor}}},
		},
	})
	if !twoFamiliesOK || withTwoFamilies == nil {
		t.Fatal("two-family source")
	}
	if _, _, accepted = buildQueries(withTwoFamilies, nil, []QueryInstance{{}}, topologyCatalog{}); accepted {
		t.Fatal("nonempty family inventory with missing coverage was accepted")
	}

	graph, graphOK := assembleGraph(zeroSource, nil, nil, nil, nil, zeroQueries, nil, nil, topologyCatalog{})
	if !graphOK || graph == nil || !graph.valid() {
		t.Fatal("empty queryless graph was not structurally accepted")
	}
	if _, demandOK := graph.Demand(); demandOK {
		t.Fatal("Graph.Demand accepted a graph with no query roots")
	}
}

func TestMixedQueryPopulationUsesOnlySelectedPointRows(t *testing.T) {
	factor := boundaryKey(161)
	selectedFamily, observationFamily := boundaryKey(162), boundaryKey(163)
	source, sourceOK := composition.Seal(composition.Candidate{
		Factors: []composition.Factor{{Key: factor}},
		Queries: []composition.QueryFamily{
			{Key: selectedFamily, Freezer: boundaryKey(164), Population: queryschema.PopulationKindSelectedPoint, Projections: []composition.QueryProjection{{Kind: composition.QueryFactorExact, Factor: factor}}},
			{Key: observationFamily, Freezer: boundaryKey(165), Population: queryschema.PopulationKindObservation, Projections: []composition.QueryProjection{{Kind: composition.QueryFactorExact, Factor: factor}}},
		},
	})
	if !sourceOK || source == nil {
		t.Fatal("mixed query population source")
	}
	batch := NewBatch()
	site, siteOK := batch.AdmitSite(boundaryKey(166), EmptyScope(), TrueExpr(), InitPresent)
	if !siteOK || !batch.Seal() {
		t.Fatal("mixed query population batch")
	}
	declared, _, points, pointsOK := buildPoints([]PointSpec{{Site: site}})
	if !pointsOK || len(points) != 1 {
		t.Fatal("mixed query population point")
	}
	row := QueryInstance{
		Context: boundaryContext(167), Family: selectedFamily, Point: PointAt(0),
		Surfaces: []Surface{{Factor: factor, Form: SurfaceReadExact, Local: 1}},
	}
	queries, _, accepted := buildQueries(source, declared, []QueryInstance{row}, topologyCatalog{})
	if !accepted || len(queries) != 1 {
		t.Fatal("selected-point row did not cover mixed query families")
	}
	observationRow := row
	observationRow.Family = observationFamily
	if queries, _, accepted = buildQueries(source, declared, []QueryInstance{observationRow}, topologyCatalog{}); accepted || queries != nil {
		t.Fatal("observation family was smuggled into graph query rows")
	}
	if queries, _, accepted = buildQueries(source, declared, nil, topologyCatalog{}); accepted || queries != nil {
		t.Fatal("missing selected-point family was accepted")
	}
}

func TestObservationOnlyQueryPopulationAllowsEmptyGraphQueryPlane(t *testing.T) {
	factor, family := boundaryKey(171), boundaryKey(172)
	source, sourceOK := composition.Seal(composition.Candidate{
		Factors: []composition.Factor{{Key: factor}},
		Queries: []composition.QueryFamily{{
			Key: family, Freezer: boundaryKey(173), Population: queryschema.PopulationKindObservation,
			Projections: []composition.QueryProjection{{Kind: composition.QueryFactorExact, Factor: factor}},
		}},
	})
	if !sourceOK || source == nil {
		t.Fatal("observation-only query population source")
	}
	queries, _, accepted := buildQueries(source, nil, nil, topologyCatalog{})
	if !accepted || len(queries) != 0 {
		t.Fatal("observation-only source did not retain an empty graph query plane")
	}
}
