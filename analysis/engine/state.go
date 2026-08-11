package engine

import "github.com/wippyai/go-lua/analysis/engine/internal/composition"

// State is one completed immutable Solver result.  It deliberately exposes
// neither carrier state nor scheduler data: those are evaluator-local and
// must not become a continuation route. A State is valid only for the exact
// Solver revision that published it.
type State struct {
	owner      *Solver
	completion *completionAuthority
	results    []*queryResult
}

// completionAuthority is an unforgeable, immutable terminal token. The
// revision binds the result to the exact Solver relation that produced it;
// installing a later activation revision invalidates the earlier result.
type completionAuthority struct {
	solver   *Solver
	serial   uint64
	revision uint64
}

// QueryResult returns an independent clone for one exact opaque QueryReceipt.
// The receipt fences the issuing Assembly, Solver, revision, family
// authority, canonical equation instance key, and dense runtime result slot.
// No family-only lookup exists: callers must retain the one receipt issued for
// each concrete QueryInstance.
func QueryResult[R any](receipt QueryReceipt[R], state *State) (R, bool) {
	var zero R
	value := receipt.value
	query := receipt.query
	if query == nil || !receipt.Available() || !validQueryReceipt(value) || state == nil || state.owner == nil || state.owner != value.solver || !state.owner.ownsCompletedState(state) || state.completion == nil || state.completion.revision != state.owner.revision {
		return zero, false
	}
	authority := sealedQueryAuthority(query)
	if authority == nil || value.authority != authority {
		return zero, false
	}
	result, ok := state.result(value.slot, authority, value.key)
	if !ok || result.value == nil {
		return zero, false
	}
	typed, ok := result.value.(*typedFrozenValue[R])
	if !ok || typed == nil {
		return zero, false
	}
	return query.result.Clone(typed.value), true
}

func (solver *Solver) ownsCompletedState(state *State) bool {
	if solver == nil || state == nil || state.owner != solver {
		return false
	}
	solver.mu.Lock()
	defer solver.mu.Unlock()
	return state.completion != nil && state.completion.solver == solver && state.completion.serial != 0 && state.completion.serial <= solver.completion && state.completion.revision == solver.revision
}

func (state *State) result(index int, owner *queryAuthority, key composition.Key) (*queryResult, bool) {
	if state == nil || owner == nil || index < 0 || index >= len(state.results) {
		return nil, false
	}
	result := state.results[index]
	return result, result != nil && result.owner == owner && result.key == key
}
