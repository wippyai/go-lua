package dominance

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// These surfaces are intentionally kept public even before a higher layer wires
// them into SSA construction.
func TestPlannedDominanceSurfacesRemainAvailable(t *testing.T) {
	g := cfg.New()
	p := g.AddNode(cfg.NodeAssign)
	g.AddEdge(g.Entry(), p, false)
	g.AddEdge(p, g.Exit(), false)

	postIDom, postTree := ComputePostDominators(g)
	assertImmediateDominators(t, postIDom, map[cfg.Point]cfg.Point{
		g.Exit():  g.Exit(),
		p:         g.Exit(),
		g.Entry(): p,
	})
	assertPointSlice(t, postTree[g.Exit()], []cfg.Point{p}, "post-dominator tree[exit]")
	assertPointSlice(t, postTree[p], []cfg.Point{g.Entry()}, "post-dominator tree[assignment]")
	if !PostDominates(postIDom, g.Exit(), g.Entry()) {
		t.Fatal("exit should post-dominate entry in the connected CFG")
	}

	assertImmediateDominators(t, ComputeImmediatePostDominators(g), postIDom)

	info := ComputeDomInfo(g)
	if info == nil {
		t.Fatal("ComputeDomInfo returned nil")
	}
	assertImmediateDominators(t, info.ImmediateDominators, map[cfg.Point]cfg.Point{
		g.Entry(): g.Entry(),
		p:         g.Entry(),
		g.Exit():  p,
	})
	assertPointSlice(t, info.DominatorTree[g.Entry()], []cfg.Point{p}, "dominator tree[entry]")
	assertPointSlice(t, info.DominatorTree[p], []cfg.Point{g.Exit()}, "dominator tree[assignment]")
	if len(info.DominanceFrontier) != 0 {
		t.Fatalf("dominance frontier = %v, want empty for a straight-line CFG", info.DominanceFrontier)
	}
}
