package engine

import (
	"github.com/wippyai/go-lua/analysis/identity"
)

// Receipt-side reads over a published State; the solver-side State, address,
// and borrow validation live in state.go.

// ReceiptQueryResult reads a receipt-native query attached to the common
// solver runtime. The returned value is borrowed: it is the published,
// transitively immutable result, validated once by this call and never cloned.
// A caller that needs an owned, mutable copy asks for one through
// DetachReceiptQueryResult.
func ReceiptQueryResult[R any](query ReceiptQuery, solver *Solver, state *State) (R, bool) {
	value, _, ok := borrowQueryResult[R](query, solver, state)
	return value, ok
}

// DetachReceiptQueryResult reads the same borrowed result and returns the
// freezer's independent copy of it. Detachment is explicit and is charged only
// to the caller that asks for an owned value.
func DetachReceiptQueryResult[R any](query ReceiptQuery, solver *Solver, state *State) (R, bool) {
	value, freeze, ok := borrowQueryResult[R](query, solver, state)
	if !ok || freeze.Clone == nil {
		var zero R
		return zero, false
	}
	return freeze.Clone(value), true
}

// ReceiptObservationResult reads one optional solve-local observation. The
// returned value is borrowed under the same discipline as a query result: one
// validation, no clone.
func ReceiptObservationResult[R any](observation ReceiptObservation[R], solver *Solver, state *State) (R, bool) {
	value, _, ok := borrowObservationResult(observation, solver, state)
	return value, ok
}

// DetachReceiptObservationResult returns the freezer's independent copy of the
// borrowed observation result.
func DetachReceiptObservationResult[R any](observation ReceiptObservation[R], solver *Solver, state *State) (R, bool) {
	value, freeze, ok := borrowObservationResult(observation, solver, state)
	if !ok || freeze.Clone == nil {
		var zero R
		return zero, false
	}
	return freeze.Clone(value), true
}

// borrowQueryResult resolves the query to the single result address this State
// publishes for it, validates that address once, and borrows the published
// value. The graph identity and owner proof are query admission; the store,
// generation, lane kind, and slot bound are the address validation.
func borrowQueryResult[R any](query ReceiptQuery, solver *Solver, state *State) (R, FrozenResult[R], bool) {
	var zero R
	if query.graph == nil || !query.graph.valid() || solver == nil || solver.runtime == nil || solver.runtime.graph == nil || !solver.runtime.graph.OwnsQuery(query.identity) {
		return zero, FrozenResult[R]{}, false
	}
	locator, owner, resolved := resolveQueryResult(query, solver, state)
	if !resolved || !solver.validBorrow(state, locator) {
		return zero, FrozenResult[R]{}, false
	}
	result, borrowed := state.queryAt(locator, owner, query.identity.Key())
	if !borrowed {
		return zero, FrozenResult[R]{}, false
	}
	return typedResult[R](result.value)
}

// borrowObservationResult is the observation-column counterpart of
// borrowQueryResult. The handle's ordinal is the observation column's own
// coordinate, so resolution is a direct address rather than a directory scan.
func borrowObservationResult[R any](observation ReceiptObservation[R], solver *Solver, state *State) (R, FrozenResult[R], bool) {
	var zero R
	if !observation.Available() || solver == nil || solver.runtime == nil || !observation.owner.valid(solver.runtime) || observation.ordinal >= uint64(len(solver.runtime.observations)) {
		return zero, FrozenResult[R]{}, false
	}
	runtime := solver.runtime.observations[observation.ordinal]
	if runtime == nil || runtime.observationOwner() != observation.owner || runtime.observationID() != observation.id {
		return zero, FrozenResult[R]{}, false
	}
	locator, resolved := state.resolveResult(resultLaneObservation, int(observation.ordinal))
	if !resolved || !solver.validBorrow(state, locator) {
		return zero, FrozenResult[R]{}, false
	}
	result, borrowed := state.observationAt(locator, observation.owner, observation.id)
	if !borrowed {
		return zero, FrozenResult[R]{}, false
	}
	return typedResult[R](result.value)
}

// resolveQueryResult returns the single address this State publishes for the
// query, together with the runtime owner the published row must name. It mints
// an address and reads no value; a query the runtime does not carry resolves to
// nothing.
func resolveQueryResult(query ReceiptQuery, solver *Solver, state *State) (resultLocator, queryOwner, bool) {
	if solver == nil || solver.runtime == nil {
		return resultLocator{}, nil, false
	}
	for index, runtimeQuery := range solver.runtime.queries {
		if runtimeQuery == nil || runtimeQuery.query().Key() != query.identity.Key() {
			continue
		}
		locator, resolved := state.resolveResult(resultLaneQuery, index)
		return locator, runtimeQuery.queryOwner(), resolved
	}
	return resultLocator{}, nil, false
}

// observationAt is the observation-column counterpart of queryAt.
func (state *State) observationAt(locator resultLocator, owner *receiptObservationOwner, id identity.ContentID) (*observationResult, bool) {
	if state == nil || owner == nil || locator.Slot.lane != resultLaneObservation || uint64(locator.Slot.slot) >= uint64(len(state.observations)) {
		return nil, false
	}
	result := state.observations[locator.Slot.slot]
	return result, result != nil && result.owner == owner && result.id == id && result.value != nil
}
