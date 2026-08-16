package engine

import (
	"math"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/identity"
)

// State is one completed immutable Solver result.  It deliberately exposes
// neither carrier state nor scheduler data: those are evaluator-local and
// must not become a continuation route. A State is valid only for the exact
// Solver revision that published it.
type State struct {
	completion   *completionAuthority
	results      []*queryResult
	observations []*observationResult
}

// completionAuthority is an unforgeable, immutable terminal token. store names
// the Solver store that published this result; serial and relation are
// Generations of that store: serial orders published completions, and relation
// binds the result to the exact activation relation that produced it, so
// installing a later relation invalidates the earlier result.
type completionAuthority struct {
	store    identity.StoreID
	serial   identity.Generation
	relation identity.Generation
}

// solverStores issues the StoreID of every compiled Solver. Assignment is
// append-only and a store number is never reused, so an address minted by one
// Solver is not addressable in another and a saturated sequence fails closed
// rather than aliasing a live store.
var solverStores idSequence[identity.StoreID]

// resultLane names the two published result columns of a State. It is part of
// the address, so an address into one column can never read the other.
type resultLane uint8

const (
	resultLaneNone resultLane = iota
	resultLaneQuery
	resultLaneObservation
)

// resultAddress is the State-relative coordinate a result locator carries. Its
// fields are unexported and no exported function accepts or returns one, so an
// address can be held, copied, and compared inside the engine but never minted
// by a consumer, serialized, or written down as a durable key. That is the
// whole enforcement of the rule that a locator is an address rather than an
// identity.
type resultAddress struct {
	lane resultLane
	slot uint32
}

// resultLocator is one published result slot in one Solver store at one
// completion revision. It is valid only against the completion that issued it,
// and it stops being valid the moment that store publishes another activation
// relation.
type resultLocator = identity.Locator[resultAddress]

// atOrBefore reports whether stamp names the current revision of a store or an
// earlier one. Both stamps must name a revision: an unset fence never passes.
func atOrBefore(stamp, current identity.Generation) bool {
	return stamp.Available() && current.Available() && (stamp == current || stamp.Precedes(current))
}

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

// resolveResult anchors one lane coordinate to the store and completion
// revision that published this State. An unpublished State, an unnamed lane, or
// a coordinate outside the address width mints nothing, and the zero locator
// never validates.
func (state *State) resolveResult(lane resultLane, slot int) (resultLocator, bool) {
	if state == nil || state.completion == nil || lane == resultLaneNone || slot < 0 || slot > math.MaxUint32 {
		return resultLocator{}, false
	}
	locator := identity.NewLocator(state.completion.store, state.completion.serial, resultAddress{lane: lane, slot: uint32(slot)})
	return locator, locator.Available()
}

// validBorrow is the one validation a borrowed read pays. The address must name
// this State's store and completion revision; that store must be this Solver;
// the completion must not name a future publication; and the activation
// relation that published the result must still be the live one. Everything the
// borrow reaches after this is published immutable data, so no later access
// revalidates and none clones.
func (solver *Solver) validBorrow(state *State, locator resultLocator) bool {
	if solver == nil || state == nil || state.completion == nil {
		return false
	}
	completion := state.completion
	if !locator.Valid(completion.store, completion.serial) {
		return false
	}
	solver.mu.Lock()
	defer solver.mu.Unlock()
	return completion.store.Available() && completion.store == solver.store && atOrBefore(completion.serial, solver.completion) && completion.relation == solver.relation.Generation()
}

// queryAt recovers the query row locator addresses: lane kind first, then the
// slot bound, then the owner and semantic key the published row must carry.
// Every check fails closed, so a mismatched address can never borrow whatever
// now occupies the slot.
func (state *State) queryAt(locator resultLocator, owner queryOwner, key composition.Key) (*queryResult, bool) {
	if state == nil || owner == nil || locator.Slot.lane != resultLaneQuery || uint64(locator.Slot.slot) >= uint64(len(state.results)) {
		return nil, false
	}
	result := state.results[locator.Slot.slot]
	return result, result != nil && result.owner == owner && result.key == key && result.value != nil
}

// observationAt is the observation-column counterpart of queryAt.
func (state *State) observationAt(locator resultLocator, owner *receiptObservationOwner, id identity.ContentID) (*observationResult, bool) {
	if state == nil || owner == nil || locator.Slot.lane != resultLaneObservation || uint64(locator.Slot.slot) >= uint64(len(state.observations)) {
		return nil, false
	}
	result := state.observations[locator.Slot.slot]
	return result, result != nil && result.owner == owner && result.id == id && result.value != nil
}

// typedResult recovers the typed frozen value from a published row. The value
// is returned as it is stored, together with its freezer so an explicit
// detachment can be served from the same borrow.
func typedResult[R any](value frozenValue) (R, FrozenResult[R], bool) {
	typed, ok := value.(*typedFrozenValue[R])
	if !ok || typed == nil {
		var zero R
		return zero, FrozenResult[R]{}, false
	}
	return typed.value, typed.freeze, true
}

func (solver *Solver) ownsCompletedState(state *State) bool {
	if solver == nil || state == nil || state.completion == nil {
		return false
	}
	solver.mu.Lock()
	defer solver.mu.Unlock()
	return state.completion.store.Available() && state.completion.store == solver.store && atOrBefore(state.completion.serial, solver.completion) && state.completion.relation == solver.relation.Generation()
}
