package store

import (
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/placement"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// Coordinates exposes the two owner-issued Heap identities carried by one
// sealed Store route.  A route's key and destination are the same Placement
// target, but both roles remain separately declared in the member vocabulary.
// The generated composition path calls this direct accessor after the route
// relation has authenticated the row; it does not rebuild a route or consult
// a second directory.
func (route Route) Coordinates() (key, destination heap.Key, ok bool) {
	return route.Key, route.Key, route.valid()
}

// Predicate exposes the owner-issued route tag as the selected-read
// predicate.  It returns the projected value and the direct-call validity bit;
// no duplicate tag or derived discriminator is introduced.
func (route Route) Predicate() (tag uint64, ok bool) {
	return route.Tag, route.valid()
}

// StorageFold is the irreducible Store semantic reducer.  The Value source is
// already owner-authenticated by the exact read seam; it remains in this
// signature because it is a declared fold input and because the sealed route
// relation, not this reducer, owns source-to-route planning.  The reducer
// authenticates the selected Placement cell, consumes the candidate's sealed
// lifetime, and delegates the transition to Apply. Invalid evidence refuses;
// it never fabricates Unknown or an empty result.
func StorageFold(candidate valuedomain.StorageTransfer, source valuedomain.Value, routeTag uint64, selected placement.Fact) (placement.Fact, structure.ReductionOutcome) {
	_ = source
	// A zero tag is not an empty selection row. The routed execution form
	// settles an empty selection before invoking a member reducer; once this
	// function is called, the tag must be an owner-issued route member.
	if routeTag == 0 {
		return placement.BottomFact(), structure.Refuse
	}
	current, currentOK := placement.AuthenticateFactCell(selected, true, true)
	if !currentOK {
		return placement.BottomFact(), structure.Refuse
	}
	lifetime, lifetimeOK := candidate.Lifetime()
	if !lifetimeOK {
		return placement.BottomFact(), structure.Refuse
	}
	result, resultOK := Apply(current, FromProgram(lifetime))
	if !resultOK {
		return placement.BottomFact(), structure.Refuse
	}
	return result, structure.Concrete
}

func (route Route) valid() bool {
	return route.Key.Valid() && route.Key.Kind() == heap.RootAllocation && route.Tag != 0
}
