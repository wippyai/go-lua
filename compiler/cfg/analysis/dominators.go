// Package analysis provides pure graph analysis algorithms for CFGs.
package analysis

import (
	"sort"

	basecfg "github.com/wippyai/go-lua/types/cfg"
)

// DomInfo holds dominator tree and dominance frontier information.
type DomInfo struct {
	ImmediateDominators map[basecfg.Point]basecfg.Point   // idom[n] = immediate dominator of n
	DominatorTree       map[basecfg.Point][]basecfg.Point // children in dominator tree
	DominanceFrontier   map[basecfg.Point][]basecfg.Point // DF[n] = dominance frontier of n
}

// ComputeDominators computes immediate dominators and the dominator tree.
// Uses the Cooper-Harvey-Kennedy algorithm with RPO iteration.
func ComputeDominators(g basecfg.Graph) (idom map[basecfg.Point]basecfg.Point, domTree map[basecfg.Point][]basecfg.Point) {
	rpo := g.RPO()
	if len(rpo) == 0 {
		return make(map[basecfg.Point]basecfg.Point), make(map[basecfg.Point][]basecfg.Point)
	}

	// Build RPO number map for intersection algorithm
	rpoNum := make(map[basecfg.Point]int, len(rpo))
	for i, p := range rpo {
		rpoNum[p] = i
	}

	entry := g.Entry()
	idom = make(map[basecfg.Point]basecfg.Point, len(rpo))
	idom[entry] = entry

	// intersect finds the common dominator of two nodes
	intersect := func(b1, b2 basecfg.Point) basecfg.Point {
		finger1, finger2 := b1, b2

		for finger1 != finger2 {
			for rpoNum[finger1] > rpoNum[finger2] {
				finger1 = idom[finger1]
			}

			for rpoNum[finger2] > rpoNum[finger1] {
				finger2 = idom[finger2]
			}
		}

		return finger1
	}

	// Iterate until fixed point
	changed := true
	for changed {
		changed = false
		for _, b := range rpo {
			if b == entry {
				continue
			}

			preds := g.Predecessors(b)
			if len(preds) == 0 {
				continue
			}

			// Find first predecessor with defined idom
			var newIdom basecfg.Point

			found := false

			for _, p := range preds {
				if _, ok := idom[p]; ok {
					newIdom = p
					found = true

					break
				}
			}

			if !found {
				continue
			}

			// Intersect with other defined predecessors
			for _, p := range preds {
				if p == newIdom {
					continue
				}

				if _, ok := idom[p]; ok {
					newIdom = intersect(p, newIdom)
				}
			}

			if old, ok := idom[b]; !ok || old != newIdom {
				idom[b] = newIdom
				changed = true
			}
		}
	}

	// Build dominator tree from idom
	domTree = make(map[basecfg.Point][]basecfg.Point)

	for n, dom := range idom {
		if n != dom {
			domTree[dom] = append(domTree[dom], n)
		}
	}

	// Sort children for deterministic order
	for p := range domTree {
		sort.Slice(domTree[p], func(i, j int) bool {
			return rpoNum[domTree[p][i]] < rpoNum[domTree[p][j]]
		})
	}

	return idom, domTree
}

// minPredecessorsForDF is the minimum number of predecessors needed for dominance frontier computation.
const minPredecessorsForDF = 2

// ComputeDominanceFrontier computes the dominance frontier for each node.
// DF[n] contains all nodes y such that n dominates a predecessor of y
// but does not strictly dominate y.
func ComputeDominanceFrontier(g basecfg.Graph, idom map[basecfg.Point]basecfg.Point) map[basecfg.Point][]basecfg.Point {
	rpo := g.RPO()
	if len(rpo) == 0 {
		return make(map[basecfg.Point][]basecfg.Point)
	}

	// Build RPO number map for stable sorting
	rpoNum := make(map[basecfg.Point]int, len(rpo))
	for i, point := range rpo {
		rpoNum[point] = i
	}

	df := make(map[basecfg.Point][]basecfg.Point)
	dfSet := make(map[basecfg.Point]map[basecfg.Point]bool)

	for _, block := range rpo {
		preds := g.Predecessors(block)
		if len(preds) < minPredecessorsForDF {
			continue
		}

		domBlock, ok := idom[block]
		if !ok {
			continue
		}

		for _, pred := range preds {
			runner := pred

			for runner != domBlock {
				if dfSet[runner] == nil {
					dfSet[runner] = make(map[basecfg.Point]bool)
				}

				if !dfSet[runner][block] {
					dfSet[runner][block] = true

					df[runner] = append(df[runner], block)
				}

				dom, domExists := idom[runner]
				if !domExists || dom == runner {
					break
				}

				runner = dom
			}
		}
	}

	// Sort for deterministic order
	for point := range df {
		sortedDF := df[point]
		sort.Slice(sortedDF, func(i, j int) bool {
			return rpoNum[sortedDF[i]] < rpoNum[sortedDF[j]]
		})
	}

	return df
}

// ComputeDomInfo computes all dominator information in one call.
func ComputeDomInfo(g basecfg.Graph) *DomInfo {
	idom, domTree := ComputeDominators(g)
	df := ComputeDominanceFrontier(g, idom)

	return &DomInfo{
		ImmediateDominators: idom,
		DominatorTree:       domTree,
		DominanceFrontier:   df,
	}
}

// Dominates returns true if a dominates b (a is on every path from entry to b).
func Dominates(idom map[basecfg.Point]basecfg.Point, pointA, pointB basecfg.Point) bool {
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

// StrictlyDominates returns true if a strictly dominates b (a dominates b and a != b).
func StrictlyDominates(idom map[basecfg.Point]basecfg.Point, pointA, pointB basecfg.Point) bool {
	if pointA == pointB {
		return false
	}

	return Dominates(idom, pointA, pointB)
}

// reversedGraph reverses a CFG for post-dominator computation.
type reversedGraph struct {
	g basecfg.Graph
}

func (r *reversedGraph) ID() uint64                                   { return r.g.ID() }
func (r *reversedGraph) Entry() basecfg.Point                         { return r.g.Exit() }
func (r *reversedGraph) Exit() basecfg.Point                          { return r.g.Entry() }
func (r *reversedGraph) Node(p basecfg.Point) *basecfg.Node           { return r.g.Node(p) }
func (r *reversedGraph) Predecessors(p basecfg.Point) []basecfg.Point { return r.g.Successors(p) }
func (r *reversedGraph) Successor(point basecfg.Point) basecfg.Point {
	preds := r.g.Predecessors(point)
	if len(preds) > 0 {
		return preds[0]
	}

	return point
}
func (r *reversedGraph) Successors(p basecfg.Point) []basecfg.Point { return r.g.Predecessors(p) }
func (r *reversedGraph) Edges() []basecfg.Edge {
	edges := r.g.Edges()
	reversed := make([]basecfg.Edge, len(edges))

	for i, edge := range edges {
		reversed[i] = basecfg.Edge{From: edge.To, To: edge.From, Cond: edge.Cond}
	}

	return reversed
}
func (r *reversedGraph) Size() int { return r.g.Size() }
func (r *reversedGraph) EdgeCond(from, to basecfg.Point) (bool, bool) {
	return r.g.EdgeCond(to, from)
}
func (r *reversedGraph) IsJoin(p basecfg.Point) bool   { return r.g.IsBranch(p) }
func (r *reversedGraph) IsBranch(p basecfg.Point) bool { return r.g.IsJoin(p) }

// RPO computes reverse post-order from the exit node following reversed edges.
func (r *reversedGraph) RPO() []basecfg.Point {
	graphSize := r.g.Size()
	visited := make([]bool, graphSize)
	order := make([]basecfg.Point, 0, graphSize)

	var visit func(point basecfg.Point)
	visit = func(point basecfg.Point) {
		if int(point) >= graphSize || visited[point] {
			return
		}

		visited[point] = true

		for _, succ := range r.Successors(point) {
			visit(succ)
		}

		order = append(order, point)
	}

	visit(r.Entry())

	for i, j := 0, len(order)-1; i < j; i, j = i+1, j-1 {
		order[i], order[j] = order[j], order[i]
	}

	return order
}

// ComputePostDominators computes post-dominators by running the dominator
// algorithm on the reversed CFG.
func ComputePostDominators(graph basecfg.Graph) (map[basecfg.Point]basecfg.Point, map[basecfg.Point][]basecfg.Point) {
	return ComputeDominators(&reversedGraph{g: graph})
}

// PostDominates returns true if a post-dominates b (a is on every path from b to exit).
func PostDominates(postIdom map[basecfg.Point]basecfg.Point, pointA, pointB basecfg.Point) bool {
	return Dominates(postIdom, pointA, pointB)
}
