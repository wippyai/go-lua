package engine

import "github.com/wippyai/go-lua/analysis/engine/internal/composition"

// State is one completed immutable Solver result.  It deliberately exposes
// neither carrier state nor scheduler data: those are evaluator-local and
// must not become a continuation route. A State is valid only for the exact
// Solver revision that published it.
type State struct {
	owner        *Solver
	completion   *completionAuthority
	results      []*queryResult
	observations []*observationResult
}

// completionAuthority is an unforgeable, immutable terminal token. The
// revision binds the result to the exact Solver relation that produced it;
// installing a later activation revision invalidates the earlier result.
type completionAuthority struct {
	solver   *Solver
	serial   uint64
	revision uint64
}

// ReceiptQueryResult reads a receipt-native query attached to the common
// solver runtime. The graph identity, owner proof, result slot, and solver
// revision are all revalidated; no family or semantic lookup is performed.
func ReceiptQueryResult[R any](query ReceiptQuery, solver *Solver, state *State) (R, bool) {
	var zero R
	if query.graph == nil || !query.graph.valid() || solver == nil || solver.runtime == nil || state == nil || state.owner != solver || !solver.ownsCompletedState(state) || solver.runtime.graph == nil || !solver.runtime.graph.OwnsQuery(query.identity) {
		return zero, false
	}
	for index, runtimeQuery := range solver.runtime.queries {
		if runtimeQuery == nil || runtimeQuery.query().Key() != query.identity.Key() {
			continue
		}
		owner := runtimeQuery.queryOwner()
		result, ok := state.result(index, owner, query.identity.Key())
		if !ok || result.value == nil {
			return zero, false
		}
		typed, ok := result.value.(*typedFrozenValue[R])
		if !ok || typed == nil {
			return zero, false
		}
		return typed.freeze.Clone(typed.value), true
	}
	return zero, false
}

// ReceiptObservationResult reads one optional solve-local observation. The
// handle, runtime row, completed State, and Solver revision must all share the
// same unforgeable owner; the returned value is an independent frozen clone.
func ReceiptObservationResult[R any](observation ReceiptObservation[R], solver *Solver, state *State) (R, bool) {
	var zero R
	if !observation.Available() || solver == nil || solver.runtime == nil || state == nil || state.owner != solver || !solver.ownsCompletedState(state) || !observation.owner.valid(solver.runtime) || observation.ordinal >= uint64(len(solver.runtime.observations)) || observation.ordinal >= uint64(len(state.observations)) {
		return zero, false
	}
	runtime := solver.runtime.observations[observation.ordinal]
	result := state.observations[observation.ordinal]
	if runtime == nil || runtime.observationOwner() != observation.owner || runtime.observationID() != observation.id || result == nil || result.owner != observation.owner || result.id != observation.id || result.value == nil {
		return zero, false
	}
	typed, ok := result.value.(*typedFrozenValue[R])
	if !ok || typed == nil {
		return zero, false
	}
	return typed.freeze.Clone(typed.value), true
}

func (solver *Solver) ownsCompletedState(state *State) bool {
	if solver == nil || state == nil || state.owner != solver {
		return false
	}
	solver.mu.Lock()
	defer solver.mu.Unlock()
	return state.completion != nil && state.completion.solver == solver && state.completion.serial != 0 && state.completion.serial <= solver.completion && state.completion.revision == solver.revision
}

func (state *State) result(index int, owner queryOwner, key composition.Key) (*queryResult, bool) {
	if state == nil || owner == nil || index < 0 || index >= len(state.results) {
		return nil, false
	}
	result := state.results[index]
	return result, result != nil && result.owner == owner && result.key == key
}
