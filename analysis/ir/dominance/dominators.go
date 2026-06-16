package dominance

import "github.com/wippyai/go-lua/analysis/ir/cfg"

// ComputeDominators computes immediate dominators and the dominator tree.
func ComputeDominators(g cfg.Graph) (map[cfg.Point]cfg.Point, map[cfg.Point][]cfg.Point) {
	data := computeImmediateDominatorData(g)
	idom := data.asMap()
	domTree := make(map[cfg.Point][]cfg.Point, len(idom))

	for point, dom := range idom {
		if point != dom {
			domTree[dom] = append(domTree[dom], point)
		}
	}

	for point := range domTree {
		sortByRPO(domTree[point], data.rpoNum)
	}

	return idom, domTree
}

// Dominates returns true if pointA dominates pointB.
func Dominates(idom map[cfg.Point]cfg.Point, pointA, pointB cfg.Point) bool {
	if pointA == pointB {
		return true
	}

	runner := pointB
	for {
		dom, ok := idom[runner]
		if !ok || dom == runner {
			return false
		}
		if dom == pointA {
			return true
		}
		runner = dom
	}
}

// StrictlyDominates returns true if pointA dominates pointB and the points differ.
func StrictlyDominates(idom map[cfg.Point]cfg.Point, pointA, pointB cfg.Point) bool {
	if pointA == pointB {
		return false
	}
	return Dominates(idom, pointA, pointB)
}
