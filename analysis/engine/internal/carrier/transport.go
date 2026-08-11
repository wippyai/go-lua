package carrier

import "github.com/wippyai/go-lua/analysis/engine/internal/facts/support"

// Transport performs one complete directed input boundary:
//
//	post ∧ omega(pre ∧ state)
//
// The precondition is applied in the source scope before reindexing; the
// postcondition is applied only after the result reaches the target scope.
// This is the sole carrier entry point for a complete equation input, so a
// caller cannot reuse a State through a rename, forget, false filter, or
// changed target interface.
func (work *Work) Transport(state State, pre support.Mask, omega ReindexPlan, post support.Mask) (State, bool) {
	if !work.live() || !work.OwnsState(state) || !omega.validFor(work.composition) || !state.scope.same(omega.source()) ||
		!validBoundaryMask(pre, state.scope) || !validBoundaryMask(post, omega.target()) {
		return State{}, false
	}
	// This fast path is intentionally exact. A semantically equivalent but
	// separately issued scope cannot reach it because ReindexPlan.Identity
	// already proves same issued source/target scope.
	if omega.identity() && pre.IsTrue() && post.IsTrue() {
		return state, true
	}
	filtered, ok := work.filter(state, pre)
	if !ok {
		return State{}, false
	}
	reindexed, ok := work.Reindex(filtered, omega)
	if !ok {
		return State{}, false
	}
	return work.filter(reindexed, post)
}

func validBoundaryMask(mask support.Mask, scope Scope) bool {
	if !mask.Valid() || !scope.Valid() || scope.composition == nil || mask.Manager() != scope.composition.guards {
		return false
	}
	root, ok := mask.Guard()
	return ok && scope.guard.Contains(root)
}

// filter retains the exact state roots under a support-only boundary filter.
// No typed root is rebuilt: Transfer with no patches carries the immutable
// vector and changes only outer support. This is the filter-only sharing law.
func (work *Work) filter(state State, within support.Mask) (State, bool) {
	if !work.live() || !work.OwnsState(state) || !validBoundaryMask(within, state.scope) {
		return State{}, false
	}
	if within.SameHandle(state.support) {
		return state, true
	}
	view, ok := state.Restrict(within)
	if !ok {
		return State{}, false
	}
	next, _, ok := work.Transfer(state, view, nil)
	return next, ok
}
