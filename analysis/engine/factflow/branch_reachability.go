package factflow

import "github.com/wippyai/go-lua/analysis/ir/cfg"

// BranchEdgeReachability records branch edges proven unreachable before the
// state transfer runs. It is intentionally edge-shaped, not syntax-shaped:
// language frontends prove impossibility, the engine only bottoms that edge.
type BranchEdgeReachability struct {
	unreachableTrue  bool
	unreachableFalse bool
}

// NewBranchEdgeReachability constructs branch-edge reachability facts.
func NewBranchEdgeReachability(unreachableTrue, unreachableFalse bool) BranchEdgeReachability {
	return BranchEdgeReachability{
		unreachableTrue:  unreachableTrue,
		unreachableFalse: unreachableFalse,
	}
}

// EdgeUnreachable reports whether the selected branch edge is impossible.
func (r BranchEdgeReachability) EdgeUnreachable(cond bool) bool {
	if cond {
		return r.unreachableTrue
	}
	return r.unreachableFalse
}

func copyBranchEdgeReachabilityMap(in map[cfg.Point]BranchEdgeReachability) map[cfg.Point]BranchEdgeReachability {
	if len(in) == 0 {
		return nil
	}
	out := make(map[cfg.Point]BranchEdgeReachability, len(in))
	for point, reachability := range in {
		out[point] = reachability
	}
	return out
}

func copyBranchConditionSourceMap(in map[cfg.Point]ValueSource) map[cfg.Point]ValueSource {
	if len(in) == 0 {
		return nil
	}
	out := make(map[cfg.Point]ValueSource, len(in))
	for point, source := range in {
		out[point] = source
	}
	return out
}
