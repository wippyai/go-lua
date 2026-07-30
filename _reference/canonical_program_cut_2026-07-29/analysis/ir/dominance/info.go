package dominance

import "github.com/wippyai/go-lua/analysis/ir/cfg"

// DomInfo holds dominator tree and dominance frontier information.
type DomInfo struct {
	ImmediateDominators map[cfg.Point]cfg.Point
	DominatorTree       map[cfg.Point][]cfg.Point
	DominanceFrontier   map[cfg.Point][]cfg.Point
}

// ComputeDomInfo computes immediate dominators, the dominator tree, and frontiers.
func ComputeDomInfo(g cfg.Graph) *DomInfo {
	idom, domTree := ComputeDominators(g)
	df := ComputeDominanceFrontier(g, idom)
	return &DomInfo{
		ImmediateDominators: idom,
		DominatorTree:       domTree,
		DominanceFrontier:   df,
	}
}
