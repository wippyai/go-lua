package operationplan

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func TestStructuralExpressionRegionsAreGraphCertifiedCanonicalMetadata(t *testing.T) {
	graph, branch, rhs, join := structuralRegionGraph()
	points := []cfg.Point{rhs}
	region, ok := factflow.NewStructuralExpressionRegion(branch, rhs, join, join, true, points)
	if !ok {
		t.Fatal("valid region rejected")
	}
	plan := NewWithStructuralExpressionRegions(graph, factflow.FactsInput{}, map[factflow.ExprRef]factflow.StructuralExpressionRegion{
		9: region,
		2: region,
	})
	points[0] = 999
	var order []factflow.ExprRef
	plan.ForEachStructuralExpressionRegion(func(ref factflow.ExprRef, got factflow.StructuralExpressionRegion) bool {
		order = append(order, ref)
		owned := got.OwnedRHSPoints()
		owned[0] = 998
		return true
	})
	if !reflect.DeepEqual(order, []factflow.ExprRef{2, 9}) {
		t.Fatalf("order = %v, want [2 9]", order)
	}
	got, ok := plan.StructuralExpressionRegion(2)
	if !ok || !reflect.DeepEqual(got.OwnedRHSPoints(), []cfg.Point{rhs}) {
		t.Fatalf("stored region = %v/%v", got.OwnedRHSPoints(), ok)
	}
	cur := plan.DependencyCursor()
	for kind, ok := cur.Next(); ok; kind, ok = cur.Next() {
		t.Fatalf("structural metadata leaked into DependencyCursor as %s", kind)
	}
}

func TestStructuralExpressionRegionsRejectMalformedGraphMetadata(t *testing.T) {
	graph, branch, rhs, join := structuralRegionGraph()
	point999, _ := factflow.NewStructuralExpressionRegion(branch, 999, join, join, true, []cfg.Point{999})
	disconnected := graph.AddNode(cfg.NodeNoop)
	disconnectedRegion, _ := factflow.NewStructuralExpressionRegion(branch, rhs, join, join, true, []cfg.Point{rhs, disconnected})
	tests := []struct {
		name   string
		region factflow.StructuralExpressionRegion
	}{
		{name: "point999", region: point999},
		{name: "disconnected owned point", region: disconnectedRegion},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan := NewWithStructuralExpressionRegions(graph, factflow.FactsInput{}, map[factflow.ExprRef]factflow.StructuralExpressionRegion{1: test.region})
			if _, ok := plan.StructuralExpressionRegion(1); ok {
				t.Fatal("malformed graph region retained")
			}
		})
	}
	t.Run("external predecessor enters owned RHS", func(t *testing.T) {
		otherGraph, otherBranch, otherRHS, otherJoin := structuralRegionGraph()
		external := otherGraph.AddNode(cfg.NodeNoop)
		otherGraph.AddEdge(otherGraph.Entry(), external, false)
		otherGraph.AddEdge(external, otherRHS, false)
		region, _ := factflow.NewStructuralExpressionRegion(otherBranch, otherRHS, otherJoin, otherJoin, true, []cfg.Point{otherRHS})
		plan := NewWithStructuralExpressionRegions(otherGraph, factflow.FactsInput{}, map[factflow.ExprRef]factflow.StructuralExpressionRegion{1: region})
		if _, ok := plan.StructuralExpressionRegion(1); ok {
			t.Fatal("region with external ingress retained")
		}
	})
}

func structuralRegionGraph() (*cfg.CFG, cfg.Point, cfg.Point, cfg.Point) {
	graph := cfg.New()
	branch := graph.AddBranch()
	rhs := graph.AddNode(cfg.NodeNoop)
	join := graph.AddNode(cfg.NodeJoin)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, rhs, true)
	graph.AddEdge(branch, join, false)
	graph.AddEdge(rhs, join, false)
	graph.AddEdge(join, graph.Exit(), false)
	return graph, branch, rhs, join
}
