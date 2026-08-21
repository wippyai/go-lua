package engine

import (
	"math"

	"github.com/wippyai/go-lua/analysis/engine/internal/lifetime"
	"github.com/wippyai/go-lua/analysis/identity"
)

// State is one completed immutable Solver result.  It deliberately exposes
// neither carrier state nor scheduler data: those are evaluator-local and
// must not become a continuation route. A State is valid only for the exact
// Solver revision that published it.
type State struct {
	completion *completionAuthority
	solved     SolvedSnapshot
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
var solverStores lifetime.Sequence[identity.StoreID]

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

// PublishedSnapshot returns the immutable publication owned by solver for
// state. The solver/state fence is checked once, before the sealed snapshot
// and its family identities cross the package boundary; callers then read the
// returned publication by stable family and row key.
func (solver *Solver) PublishedSnapshot(state *State) (SolvedSnapshot, bool) {
	if solver == nil || !solver.ownsCompletedState(state) {
		return SolvedSnapshot{}, false
	}
	if !state.solved.Available() {
		return SolvedSnapshot{}, false
	}
	return state.solved, true
}

func (solver *Solver) ownsCompletedState(state *State) bool {
	if solver == nil || state == nil || state.completion == nil {
		return false
	}
	solver.mu.Lock()
	defer solver.mu.Unlock()
	return state.completion.store.Available() && state.completion.store == solver.store && atOrBefore(state.completion.serial, solver.completion) && state.completion.relation == solver.relation.Generation()
}
