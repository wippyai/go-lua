package factquery

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestDominatingOrdinaryRootWriteQueryUsesProvidedCarrier(t *testing.T) {
	graph := cfg.New()
	write := graph.AddNode(cfg.NodeAssign)
	descendant := graph.AddNode(cfg.NodeCall)
	other := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), write, false)
	graph.AddEdge(write, descendant, false)
	graph.AddEdge(descendant, graph.Exit(), false)
	graph.AddEdge(graph.Entry(), other, false)
	target := symbol.ID(42)
	otherTarget := symbol.ID(99)

	query := NewDominatingOrdinaryRootWriteQuery(graph, func(point cfg.Point, id symbol.ID) bool {
		return point == write && id == target
	})

	got, ok := query.DominatingOrdinaryRootWrite(descendant, target)
	if !ok || got != write {
		t.Fatalf("DominatingOrdinaryRootWrite = %d/%v, want %d/true", got, ok, write)
	}
	if got, ok := query.DominatingOrdinaryRootWrite(descendant, otherTarget); ok || got != 0 {
		t.Fatalf("DominatingOrdinaryRootWrite(other) = %d/%v, want 0/false", got, ok)
	}
	if got, ok := query.DominatingOrdinaryRootWrite(other, target); ok || got != 0 {
		t.Fatalf("DominatingOrdinaryRootWrite(non-dominating) = %d/%v, want 0/false", got, ok)
	}
}

func TestDominatingOrdinaryRootWriteQueryStopsMalformedIDomCycle(t *testing.T) {
	target := symbol.ID(42)
	query := newDominatingOrdinaryRootWriteQueryWithDominators(
		map[cfg.Point]cfg.Point{1: 2, 2: 1},
		2,
		func(point cfg.Point, id symbol.ID) bool { return false },
	)

	if got, ok := query.DominatingOrdinaryRootWrite(1, target); ok || got != 0 {
		t.Fatalf("DominatingOrdinaryRootWrite(cycle) = %d/%v, want 0/false", got, ok)
	}
}
