package equation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
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
	zeroQueries, accepted := buildQueries(zeroSource, nil, nil, topologyCatalog{}, false)
	if !accepted || len(zeroQueries) != 0 {
		t.Fatal("0 families / 0 instances was rejected")
	}
	if _, accepted = buildQueries(zeroSource, nil, []QueryInstance{{}}, topologyCatalog{}, false); accepted {
		t.Fatal("0 families / 1 instance was accepted")
	}

	factor := boundaryKey(202)
	familyA, familyB := boundaryKey(203), boundaryKey(204)
	withOneFamily, oneFamilyOK := composition.Seal(composition.Candidate{
		Factors: []composition.Factor{{Key: factor}},
		Queries: []composition.QueryFamily{{
			Key: familyA, Freezer: boundaryKey(205),
			Projections: []composition.QueryProjection{{Kind: composition.QueryFactorExact, Factor: factor}},
		}},
	})
	if !oneFamilyOK || withOneFamily == nil {
		t.Fatal("one-family source")
	}
	if _, accepted = buildQueries(withOneFamily, nil, nil, topologyCatalog{}, false); accepted {
		t.Fatal("1 family / 0 instances was accepted")
	}

	withTwoFamilies, twoFamiliesOK := composition.Seal(composition.Candidate{
		Factors: []composition.Factor{{Key: factor}},
		Queries: []composition.QueryFamily{
			{Key: familyA, Freezer: boundaryKey(206), Projections: []composition.QueryProjection{{Kind: composition.QueryFactorExact, Factor: factor}}},
			{Key: familyB, Freezer: boundaryKey(207), Projections: []composition.QueryProjection{{Kind: composition.QueryFactorExact, Factor: factor}}},
		},
	})
	if !twoFamiliesOK || withTwoFamilies == nil {
		t.Fatal("two-family source")
	}
	if _, accepted = buildQueries(withTwoFamilies, nil, []QueryInstance{{}}, topologyCatalog{}, false); accepted {
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

func TestObservationTopologyDefersOnlyAllQueryFamilies(t *testing.T) {
	factor := boundaryKey(241)
	first, second := boundaryKey(242), boundaryKey(243)
	source, sourceOK := composition.Seal(composition.Candidate{
		Factors: []composition.Factor{{Key: factor}},
		Queries: []composition.QueryFamily{
			{Key: first, Freezer: boundaryKey(244), Projections: []composition.QueryProjection{{Kind: composition.QueryFactorExact, Factor: factor}}},
			{Key: second, Freezer: boundaryKey(245), Projections: []composition.QueryProjection{{Kind: composition.QueryFactorExact, Factor: factor}}},
		},
	})
	if !sourceOK || source == nil {
		t.Fatal("deferred-query source")
	}
	batch := NewBatch()
	site, siteOK := batch.AdmitSite(boundaryKey(246), EmptyScope(), TrueExpr(), InitPresent)
	if !siteOK || !batch.Seal() {
		t.Fatal("deferred-query batch")
	}
	zero := TopologySpec{Batch: batch, Points: []PointSpec{{Site: site}}}
	noFamilySource, noFamilyOK := composition.Seal(composition.Candidate{Factors: []composition.Factor{{Key: factor}}})
	if !noFamilyOK || noFamilySource == nil {
		t.Fatal("no-family source")
	}
	if topology, _, deferred := SealObservationTopologyWithFailure(noFamilySource, zero); deferred || topology != nil {
		t.Fatal("observation topology accepted a source without query families")
	}
	if topology, strict := SealTopology(source, zero); strict || topology != nil {
		t.Fatal("ordinary topology accepted missing declared query families")
	}
	partial := zero
	partial.Queries = []QueryInstance{{Family: first, Point: PointAt(0), Surfaces: []Surface{{Factor: factor, Form: SurfaceReadExact, Local: 1}}}}
	if topology, _, deferred := SealObservationTopologyWithFailure(source, partial); deferred || topology != nil {
		t.Fatal("observation topology accepted a partial ordinary query inventory")
	}
	full := zero
	full.Queries = []QueryInstance{
		{Family: first, Point: PointAt(0), Surfaces: []Surface{{Factor: factor, Form: SurfaceReadExact, Local: 1}}},
		{Family: second, Point: PointAt(0), Surfaces: []Surface{{Factor: factor, Form: SurfaceReadExact, Local: 1}}},
	}
	strict, strictOK := SealTopology(source, full)
	if !strictOK || strict == nil {
		t.Fatal("ordinary topology rejected complete query inventory")
	}
	if topology, _, deferred := SealObservationTopologyWithFailure(source, full); deferred || topology != nil {
		t.Fatal("observation topology accepted a complete ordinary query inventory")
	}
	topology, _, deferred := SealObservationTopologyWithFailure(source, zero)
	if !deferred || topology == nil {
		t.Fatal("observation topology rejected complete query deferral")
	}
	graph, graphOK := topology.Graph(nil)
	if !graphOK || graph == nil || graph.QueryCount() != 0 {
		t.Fatal("deferred topology exposed ordinary query rows")
	}
	if _, demanded := graph.Demand(); demanded {
		t.Fatal("deferred topology acquired unowned demand")
	}
	strictGraph, strictGraphOK := strict.Graph(nil)
	strictRevision, strictRevisionOK := strict.Revision(nil)
	deferredRevision, deferredRevisionOK := topology.Revision(nil)
	if !strictGraphOK || strictGraph == nil || !strictRevisionOK || !deferredRevisionOK || strict.Key() == topology.Key() || strictRevision == deferredRevision {
		t.Fatal("strict and deferred topologies shared an identity")
	}
	if strict.OwnsGraph(graph) || topology.OwnsGraph(strictGraph) {
		t.Fatal("strict and deferred topology owners exchanged graphs")
	}
}
