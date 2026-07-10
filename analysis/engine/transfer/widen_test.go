package transfer

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func TestDefaultWidenAtAcyclicGraphMarksNoPoints(t *testing.T) {
	g := cfg.New()
	a := g.AddNode(cfg.NodeAssign)
	b := g.AddNode(cfg.NodeAssign)
	g.AddEdge(g.Entry(), a, false)
	g.AddEdge(a, b, false)
	g.AddEdge(b, g.Exit(), false)

	widenAt := DefaultWidenAt(g)
	for _, point := range g.RPO() {
		if widenAt(point) {
			t.Fatalf("DefaultWidenAt(%d) = true in acyclic graph", point)
		}
	}
}

func TestDefaultWidenAtMarksLoopHeader(t *testing.T) {
	g := cfg.New()
	header := g.AddNode(cfg.NodeBranch)
	body := g.AddNode(cfg.NodeAssign)
	g.AddEdge(g.Entry(), header, false)
	g.AddEdge(header, body, true)
	g.AddEdge(header, g.Exit(), false)
	g.AddEdge(body, header, false)

	widenAt := DefaultWidenAt(g)
	if !widenAt(header) {
		t.Fatalf("DefaultWidenAt(header) = false, want true")
	}
	if widenAt(body) {
		t.Fatalf("DefaultWidenAt(body) = true, want false")
	}
}

func TestDefaultWidenAtCutsIrreducibleCycleByRPOBackEdge(t *testing.T) {
	g := cfg.New()
	a := g.AddNode(cfg.NodeBranch)
	b := g.AddNode(cfg.NodeAssign)
	g.AddEdge(g.Entry(), a, false)
	g.AddEdge(g.Entry(), b, false)
	g.AddEdge(a, b, true)
	g.AddEdge(b, a, false)
	g.AddEdge(a, g.Exit(), false)

	widenAt := DefaultWidenAt(g)
	marked := 0
	for _, point := range []cfg.Point{a, b} {
		if widenAt(point) {
			marked++
		}
	}
	if marked == 0 {
		t.Fatalf("DefaultWidenAt did not mark either point in irreducible cycle")
	}
}
