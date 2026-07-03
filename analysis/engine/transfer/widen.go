package transfer

import "github.com/wippyai/go-lua/analysis/ir/cfg"

// DefaultWidenAt returns the canonical CFG widening policy for forward
// transfer: mark targets of edges that go backward in RPO. For any total order,
// every directed cycle has at least one edge whose target is not after its
// source, so these targets form a deterministic feedback set. Acyclic graphs
// yield a predicate that is always false.
func DefaultWidenAt(g cfg.Graph) func(cfg.Point) bool {
	points := feedbackTargetsByRPO(g, graphRPO(g))
	if len(points) == 0 {
		return func(cfg.Point) bool { return false }
	}
	return func(point cfg.Point) bool {
		_, ok := points[point]
		return ok
	}
}

func defaultWidenAtForRPO(g cfg.Graph, rpo []cfg.Point) func(cfg.Point) bool {
	points := feedbackTargetsByRPO(g, rpo)
	if len(points) == 0 {
		return func(cfg.Point) bool { return false }
	}
	return func(point cfg.Point) bool {
		_, ok := points[point]
		return ok
	}
}

func graphRPO(g cfg.Graph) []cfg.Point {
	if g == nil {
		return nil
	}
	return g.RPO()
}

func feedbackTargetsByRPO(g cfg.Graph, rpo []cfg.Point) map[cfg.Point]struct{} {
	if g == nil {
		return nil
	}
	if len(rpo) == 0 {
		return nil
	}
	order := make(map[cfg.Point]int, len(rpo))
	for i, point := range rpo {
		order[point] = i
	}
	var out map[cfg.Point]struct{}
	for _, edge := range g.Edges() {
		fromOrder, fromOK := order[edge.From]
		toOrder, toOK := order[edge.To]
		if !fromOK || !toOK || toOrder > fromOrder {
			continue
		}
		if out == nil {
			out = make(map[cfg.Point]struct{})
		}
		out[edge.To] = struct{}{}
	}
	return out
}
