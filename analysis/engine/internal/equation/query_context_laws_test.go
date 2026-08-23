package equation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/identity"
	queryschema "github.com/wippyai/go-lua/analysis/schema/query"
)

type queryContextFixture struct {
	source   *composition.Composition
	declared map[PointRef]Point
	points   []Point
	row      QueryInstance
}

func newQueryContextFixture(t testing.TB) queryContextFixture {
	t.Helper()
	factor, family := boundaryKey(230), boundaryKey(231)
	source, sourceOK := composition.Seal(composition.Candidate{
		Factors: []composition.Factor{{Key: factor}},
		Queries: []composition.QueryFamily{{
			Key: family, Freezer: boundaryKey(232), Population: queryschema.PopulationKindSelectedPoint,
			Projections: []composition.QueryProjection{{Kind: composition.QueryFactorExact, Factor: factor}},
		}},
	})
	if !sourceOK || source == nil {
		t.Fatal("query context composition")
	}
	batch := NewBatch()
	site, siteOK := batch.AdmitSite(boundaryKey(233), EmptyScope(), TrueExpr(), InitPresent)
	if !siteOK || !batch.Seal() {
		t.Fatal("query context batch")
	}
	declared, _, points, pointsOK := buildPoints([]PointSpec{{Site: site}})
	if !pointsOK || len(points) != 1 {
		t.Fatal("query context point")
	}
	return queryContextFixture{
		source: source, declared: declared, points: points,
		row: QueryInstance{
			Context: boundaryContext(234), Family: family, Point: PointAt(0),
			Surfaces: []Surface{{Factor: factor, Form: SurfaceReadExact, Local: 1}},
		},
	}
}

func TestQueryCoordinatesInDistinctContextsCoexist(t *testing.T) {
	fixture := newQueryContextFixture(t)
	other := fixture.row
	other.Context = boundaryContext(235)
	queries, _, accepted := buildQueries(fixture.source, fixture.declared, []QueryInstance{fixture.row, other}, topologyCatalog{})
	if !accepted || len(queries) != 2 {
		t.Fatal("equal query coordinates in distinct contexts did not coexist")
	}
	if queries[0].Key() == queries[1].Key() {
		t.Fatal("query context did not reach canonical identity")
	}
	graph, graphOK := assembleGraph(fixture.source, fixture.points, nil, nil, nil, queries, nil, nil, topologyCatalog{})
	if !graphOK || graph == nil || graph.QueryCount() != 2 {
		t.Fatal("distinct-context query rows did not install together")
	}
	seen := map[identity.ContentID]bool{}
	for index := 0; index < graph.QueryCount(); index++ {
		query, queryOK := graph.QueryAt(index)
		if !queryOK || !query.ContextID().Available() || seen[query.ContextID()] {
			t.Fatal("retained query lost or duplicated its context identity")
		}
		seen[query.ContextID()] = true
	}
	if !seen[fixture.row.Context] || !seen[other.Context] {
		t.Fatal("retained query contexts did not match their declarations")
	}
}

func TestQueryDuplicateSameContextRefuses(t *testing.T) {
	fixture := newQueryContextFixture(t)
	if queries, _, accepted := buildQueries(fixture.source, fixture.declared, []QueryInstance{fixture.row, fixture.row}, topologyCatalog{}); accepted || queries != nil {
		t.Fatal("duplicate query in one context was accepted")
	}
}

func TestQueryZeroContextRefuses(t *testing.T) {
	fixture := newQueryContextFixture(t)
	zero := fixture.row
	zero.Context = identity.ContentID{}
	if queries, _, accepted := buildQueries(fixture.source, fixture.declared, []QueryInstance{zero}, topologyCatalog{}); accepted || queries != nil {
		t.Fatal("query with zero context was accepted")
	}
}

func TestQueryInstanceClonePreservesContext(t *testing.T) {
	fixture := newQueryContextFixture(t)
	rows := []QueryInstance{fixture.row}
	cloned := cloneQueryInstances(rows)
	originalContext := rows[0].Context
	originalLocal := rows[0].Surfaces[0].Local
	if len(cloned) != 1 || cloned[0].Context != originalContext {
		t.Fatal("query instance clone lost context identity")
	}
	rows[0].Context = boundaryContext(236)
	rows[0].Surfaces[0].Local = 2
	if cloned[0].Context != originalContext || cloned[0].Surfaces[0].Local != originalLocal {
		t.Fatal("query instance clone retained caller mutation")
	}
}
