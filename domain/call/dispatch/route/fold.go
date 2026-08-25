package route

import (
	"github.com/wippyai/go-lua/analysis/schema/structure"
	calldomain "github.com/wippyai/go-lua/domain/call"
)

// Fold consumes only the Call-owned candidate, destination coordinate, route
// predicate, and selected fact. The destination is the projection already
// paired with this member by the schema emitter; it is checked against the
// candidate rather than reconstructed from a dense index or predicate. No
// Value, Heap, or target selector crosses this boundary.

func Fold(candidate calldomain.CallCoordinate, destination calldomain.Key, predicate uint64, _ calldomain.Value) (calldomain.Value, structure.ReductionOutcome) {
	// The generated routed form has paired this destination and predicate with
	// the selected cell before calling the reducer. Candidate ownership is the
	// remaining fence needed to decode the Call-owned predicate.
	key, candidateOK := candidate.Key()
	if !candidateOK || key != destination {
		return calldomain.Value{}, structure.Refuse
	}
	fact, ok := candidate.DispatchValueForPredicate(predicate)
	if !ok {
		return calldomain.Value{}, structure.Refuse
	}
	return fact, structure.Concrete
}
