package equation

import "testing"

func TestDemandWithExplicitRootsAdmitsQuerylessGraphAndRejectsForeignRoots(t *testing.T) {
	fixture := newTemplateMaterializationFixture(t)
	materialized, materializedOK := MaterializeTemplateBoundary(fixture.source, fixture.binding,
		[]Site{fixture.input.Site(), fixture.local, fixture.output.Site()}, fixture.inputs)
	if !materializedOK {
		t.Fatal("queryless root materialization")
	}
	topology, sealed := SealTopology(fixture.source, TopologySpec{
		Batch:            fixture.actuals,
		Materializations: []TemplateMaterialization{materialized},
		Points:           []PointSpec{{Site: fixture.actualInput}, {Site: fixture.actualOutput}},
	})
	if !sealed || topology == nil {
		t.Fatal("queryless root topology")
	}
	graph, graphOK := initialGraph(topology)
	if !graphOK || graph == nil {
		t.Fatal("queryless root graph")
	}
	root, rootOK := graph.PointAt(0)
	if !rootOK {
		t.Fatal("queryless root graph")
	}
	if _, demanded := graph.Demand(); demanded {
		t.Fatal("queryless graph accepted demand without an explicit root")
	}
	demand, demanded := graph.DemandWithPoints([]Point{root, root})
	if !demanded || demand == nil || demand.PointCount() == 0 {
		t.Fatal("explicit root did not demand queryless graph")
	}

	foreignFixture := newTemplateMaterializationFixture(t)
	foreignMaterialized, foreignMaterializedOK := MaterializeTemplateBoundary(foreignFixture.source, foreignFixture.binding,
		[]Site{foreignFixture.input.Site(), foreignFixture.local, foreignFixture.output.Site()}, foreignFixture.inputs)
	foreignTopology, foreignSealed := SealTopology(foreignFixture.source, TopologySpec{
		Batch:            foreignFixture.actuals,
		Materializations: []TemplateMaterialization{foreignMaterialized},
		Points:           []PointSpec{{Site: foreignFixture.actualInput}, {Site: foreignFixture.actualOutput}},
	})
	if !foreignMaterializedOK || !foreignSealed || foreignTopology == nil {
		t.Fatal("foreign root graph")
	}
	foreignGraph, foreignGraphOK := initialGraph(foreignTopology)
	if !foreignGraphOK || foreignGraph == nil {
		t.Fatal("foreign root graph")
	}
	foreignRoot, foreignRootOK := foreignGraph.PointAt(0)
	if !foreignRootOK {
		t.Fatal("foreign root graph")
	}
	if _, demanded := graph.DemandWithPoints([]Point{foreignRoot}); demanded {
		t.Fatal("queryless graph accepted a foreign explicit root")
	}
}
