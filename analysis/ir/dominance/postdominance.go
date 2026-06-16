package dominance

import (
	"slices"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

type reversedGraph struct {
	g cfg.Graph
}

func (r *reversedGraph) ID() uint64 {
	if r == nil || r.g == nil {
		return 0
	}
	return r.g.ID()
}

func (r *reversedGraph) Entry() cfg.Point {
	if r == nil || r.g == nil {
		return 0
	}
	return r.g.Exit()
}

func (r *reversedGraph) Exit() cfg.Point {
	if r == nil || r.g == nil {
		return 0
	}
	return r.g.Entry()
}

func (r *reversedGraph) Node(point cfg.Point) *cfg.Node {
	if r == nil || r.g == nil {
		return nil
	}
	return r.g.Node(point)
}

func (r *reversedGraph) RPO() []cfg.Point {
	if r == nil || r.g == nil {
		return nil
	}

	graphSize := r.g.Size()
	visited := make([]bool, graphSize)
	order := make([]cfg.Point, 0, graphSize)

	var visit func(cfg.Point)
	visit = func(point cfg.Point) {
		if !validPoint(point, graphSize) || visited[int(point)] {
			return
		}
		visited[int(point)] = true

		for _, succ := range predecessorsOf(r.g, point) {
			visit(succ)
		}
		order = append(order, point)
	}

	visit(r.Entry())
	slices.Reverse(order)
	return order
}

func (r *reversedGraph) Predecessors(point cfg.Point) []cfg.Point {
	if r == nil || r.g == nil {
		return nil
	}
	return successorsOf(r.g, point)
}

func (r *reversedGraph) Successors(point cfg.Point) []cfg.Point {
	if r == nil || r.g == nil {
		return nil
	}
	return predecessorsOf(r.g, point)
}

func (r *reversedGraph) Edges() []cfg.Edge {
	if r == nil || r.g == nil {
		return nil
	}

	edges := r.g.Edges()
	reversed := make([]cfg.Edge, len(edges))
	for i, edge := range edges {
		reversed[i] = cfg.Edge{From: edge.To, To: edge.From, Cond: edge.Cond}
	}
	return reversed
}

func (r *reversedGraph) Size() int {
	if r == nil || r.g == nil {
		return 0
	}
	return r.g.Size()
}

func (r *reversedGraph) EdgeCond(from, to cfg.Point) (bool, bool) {
	if r == nil || r.g == nil {
		return false, false
	}
	return r.g.EdgeCond(to, from)
}

func (r *reversedGraph) IsJoin(point cfg.Point) bool {
	return r != nil && r.g != nil && r.g.IsBranch(point)
}

func (r *reversedGraph) IsBranch(point cfg.Point) bool {
	return r != nil && r.g != nil && r.g.IsJoin(point)
}

// ComputePostDominators computes post-dominators by running dominance on the
// reversed CFG.
func ComputePostDominators(g cfg.Graph) (map[cfg.Point]cfg.Point, map[cfg.Point][]cfg.Point) {
	if g == nil {
		return make(map[cfg.Point]cfg.Point), make(map[cfg.Point][]cfg.Point)
	}
	return ComputeDominators(&reversedGraph{g: g})
}

// ComputeImmediatePostDominators computes only the immediate post-dominator map.
func ComputeImmediatePostDominators(g cfg.Graph) map[cfg.Point]cfg.Point {
	if g == nil {
		return make(map[cfg.Point]cfg.Point)
	}
	return ComputeImmediateDominatorInfo(&reversedGraph{g: g}).Map()
}

// PostDominates returns true if pointA post-dominates pointB.
func PostDominates(postIDom map[cfg.Point]cfg.Point, pointA, pointB cfg.Point) bool {
	return Dominates(postIDom, pointA, pointB)
}
