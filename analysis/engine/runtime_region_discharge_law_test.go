package engine

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

func regionLawKey(value byte) composition.Key {
	var id composition.ID
	id[0] = value
	return composition.Key{ID: id, Version: 1}
}

func newRegionDischargeGraph(t testing.TB) (*equation.Graph, equation.Decision, equation.Decision) {
	t.Helper()
	compositionSource, sealed := composition.Seal(composition.Candidate{Factors: []composition.Factor{{Key: regionLawKey(1)}}})
	if !sealed {
		t.Fatal("region-law composition")
	}
	cycle, cycleOK := equation.NewDecision(regionLawKey(2))
	outer, outerOK := equation.NewDecision(regionLawKey(3))
	cycleScope, cycleScopeOK := equation.NewScope(cycle, outer)
	outerScope, outerScopeOK := equation.NewScope(outer)
	if !cycleOK || !outerOK || !cycleScopeOK || !outerScopeOK {
		t.Fatal("region-law scopes")
	}
	batch := equation.NewBatch()
	outside, outsideOK := batch.AdmitSite(regionLawKey(4), outerScope, equation.TrueExpr(), equation.InitPresent)
	head, headOK := batch.AdmitSite(regionLawKey(5), cycleScope, equation.TrueExpr(), equation.InitPresent)
	if !outsideOK || !headOK || !batch.Seal() {
		t.Fatalf("region-law source batch outside=%t head=%t", outsideOK, headOK)
	}
	backReindex := equation.IdentityReindex(cycleScope)
	back := equation.BoundaryInput(head, head, regionLawKey(6), equation.TrueExpr(), backReindex, equation.TrueExpr())
	ingressReindex, ingressOK := equation.NewReindex(outerScope, cycleScope, []equation.DecisionMap{equation.Identity(outer)})
	ingress := equation.BoundaryInput(outside, head, regionLawKey(7), equation.TrueExpr(), ingressReindex, equation.TrueExpr())
	if !outsideOK || !headOK || !back.Available() || !ingressOK || !ingress.Available() {
		t.Fatalf("region-law boundaries admitted outside=%t head=%t back=%t back-reindex=%t ingress-reindex=%t ingress=%t scopes=%t/%t", outsideOK, headOK, back.Available(), backReindex.Available(), ingressOK, ingress.Available(), head.Scope().Key() == cycleScope.Key(), outside.Scope().Key() == outerScope.Key())
	}
	topology, topologyOK := equation.SealTopology(compositionSource, equation.TopologySpec{
		Batch:  batch,
		Points: []equation.PointSpec{{Site: outside}, {Site: head}},
		EnvironmentEdges: []equation.EnvironmentEdge{
			{Target: equation.PointAt(1), Input: back},
			{Target: equation.PointAt(1), Input: ingress},
		},
	})
	if !topologyOK || topology == nil {
		t.Fatal("region-law topology")
	}
	relation, relationOK := topology.InitialRelation()
	graph, graphOK := topology.Graph(relation)
	if !relationOK || !graphOK || graph == nil || graph.RegionCount() == 0 {
		t.Fatal("region-law graph has no cycle")
	}
	return graph, cycle, outer
}

func TestRegionDischargeForgetsOnlyTheCycleOwnCoordinates(t *testing.T) {
	graph, cycle, outer := newRegionDischargeGraph(t)
	for index := 0; index < graph.RegionCount(); index++ {
		region, ok := graph.RegionAt(index)
		head, headOK := region.Head()
		if !ok || !headOK {
			t.Fatal("region-law region")
		}
		local, localOK := regionLocalDecisions(graph, region, head)
		if !localOK {
			t.Fatal("region-law local decision derivation")
		}
		if head.Scope().Count() == 2 && (len(local) != 1 || local[0] != cycle) {
			t.Fatalf("cycle head local coordinates=%v, want only cycle decision", local)
		}
		for _, decision := range local {
			if decision == outer {
				t.Fatal("outside coordinate was discharged")
			}
		}
	}
}

func TestRegionDischargeRetainsCoordinatesTheOutsideEstablishes(t *testing.T) {
	graph, cycle, outer := newRegionDischargeGraph(t)
	region, ok := graph.RegionAt(0)
	head, headOK := region.Head()
	local, localOK := regionLocalDecisions(graph, region, head)
	if !ok || !headOK || !localOK {
		t.Fatal("region-law ingress")
	}
	seenCycle, seenOuter := false, false
	for _, decision := range local {
		seenCycle = seenCycle || decision == cycle
		seenOuter = seenOuter || decision == outer
	}
	if !seenCycle || seenOuter {
		t.Fatalf("local decisions cycle=%t outer=%t values=%v", seenCycle, seenOuter, local)
	}
}

func TestCycleCoordinateSolveCompletes(t *testing.T) {
	fixture := newReceiptQueryMatrixFixture(t, 2, nil, nil)
	state, status := fixture.solver.Solve(context.Background())
	if state == nil || status != SolveComplete {
		t.Fatalf("sealed coordinate solve state=%t status=%v", state != nil, status)
	}
}

func TestGuardedCycleSolveCompletes(t *testing.T) {
	fixture := newReceiptQueryMatrixFixture(t, 4, nil, nil)
	state, status := fixture.solver.Solve(context.Background())
	if state == nil || status != SolveComplete || fixture.solver.runtime.graph.Schedule() == nil {
		t.Fatalf("sealed guarded solve state=%t status=%v", state != nil, status)
	}
}

func TestRegionDischargeJoinsTheCycleOwnGuardCellsAtTheHead(t *testing.T) {
	graph, cycle, _ := newRegionDischargeGraph(t)
	region, ok := graph.RegionAt(0)
	head, headOK := region.Head()
	local, localOK := regionLocalDecisions(graph, region, head)
	if !ok || !headOK || !localOK || len(local) != 1 || local[0] != cycle {
		t.Fatalf("cycle head discharge candidates=%v", local)
	}
}
