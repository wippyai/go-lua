package factflow

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
