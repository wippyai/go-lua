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
	if postIDom == nil {
		t.Fatal("ComputePostDominators returned nil idom map")
	}
	if postTree == nil {
		t.Fatal("ComputePostDominators returned nil tree map")
	}
	if !PostDominates(postIDom, g.Exit(), g.Entry()) {
		t.Fatal("exit should post-dominate entry in the connected CFG")
	}

	if got := ComputeImmediatePostDominators(g); got == nil {
		t.Fatal("ComputeImmediatePostDominators returned nil map")
	}

	info := ComputeDomInfo(g)
	if info == nil {
		t.Fatal("ComputeDomInfo returned nil")
	}
	if info.DominanceFrontier == nil {
		t.Fatal("ComputeDomInfo returned nil DominanceFrontier")
	}
}
