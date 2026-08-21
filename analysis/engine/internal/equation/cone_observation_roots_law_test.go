package equation

import "testing"

func TestDemandWithExplicitRootsAdmitsQuerylessGraphAndRejectsForeignRoots(t *testing.T) {
	fixture := newTriggerBindingFixture(t)
	target, endpoint := triggerBindingKey(11), triggerBindingKey(12)
	topology, sealed := fixture.sealWithRows(
		[]ActivationTriggerBinding{{TriggerOrdinal: 0, Family: fixture.family, Application: fixture.application}},
		[]ActivationRowSpec{fixture.row(fixture.application, target, endpoint)},
	)
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

	foreignFixture := newTriggerBindingFixture(t)
	foreignTopology, foreignSealed := foreignFixture.sealWithRows(
		[]ActivationTriggerBinding{{TriggerOrdinal: 0, Family: foreignFixture.family, Application: foreignFixture.application}},
		[]ActivationRowSpec{foreignFixture.row(foreignFixture.application, target, endpoint)},
	)
	if !foreignSealed || foreignTopology == nil {
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
