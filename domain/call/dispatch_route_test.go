package call

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func TestDispatchRouteKeepsTargetIdentityInsideCall(t *testing.T) {
	_, _, algebra := targetOperationLawAlgebra(t)
	application := identity.ContentID{70}
	algebra.keys = []keyRow{{kind: keyApplication, id: application}}
	algebra.keyIndex = map[identity.ContentID]uint32{application: 1}
	algebra.top = Value{owner: algebra, top: true}
	key, keyOK := algebra.KeyForApplicationID(application)
	target, targetOK := algebra.TargetForSeedID(identity.ContentID{1})
	if !keyOK || !targetOK {
		t.Fatal("dispatch route fixture")
	}

	route, routeOK := algebra.DispatchTargetRoute(key, target)
	selected, destination, coordinatesOK := route.Coordinates()
	predicate, predicateOK := route.Predicate()
	fact, factOK := algebra.dispatchValueForPredicate(key, predicate)
	if !routeOK || !coordinatesOK || !predicateOK || !factOK || selected != key || destination != key || !fact.IsComplete() || !fact.HasTarget(target) || fact.HasOpaqueAlternative() {
		t.Fatalf("target route = route:%t coordinates:%t predicate:%t fact:%t complete:%t target:%t opaque:%t", routeOK, coordinatesOK, predicateOK, factOK, fact.IsComplete(), fact.HasTarget(target), fact.HasOpaqueAlternative())
	}

	opaque, opaqueOK := algebra.DispatchOpaqueRoute(key)
	opaquePredicate, opaquePredicateOK := opaque.Predicate()
	opaqueFact, opaqueFactOK := algebra.dispatchValueForPredicate(key, opaquePredicate)
	if !opaqueOK || !opaquePredicateOK || !opaqueFactOK || !opaqueFact.IsOpen() || !opaqueFact.HasOpaqueAlternative() || opaqueFact.KnownTargetCount() != 0 {
		t.Fatalf("opaque route = route:%t predicate:%t fact:%t open:%t known:%d", opaqueOK, opaquePredicateOK, opaqueFactOK, opaqueFact.IsOpen(), opaqueFact.KnownTargetCount())
	}

	top, topOK := algebra.DispatchTopRoute(key)
	topPredicate, topPredicateOK := top.Predicate()
	topFact, topFactOK := algebra.dispatchValueForPredicate(key, topPredicate)
	if !topOK || !topPredicateOK || !topFactOK || !topFact.IsTop() {
		t.Fatalf("top route = route:%t predicate:%t fact:%t top:%t", topOK, topPredicateOK, topFactOK, topFact.IsTop())
	}
}

func TestDispatchRouteRefusesForeignAndForgedPredicates(t *testing.T) {
	_, _, algebra := targetOperationLawAlgebra(t)
	application := identity.ContentID{80}
	algebra.keys = []keyRow{{kind: keyApplication, id: application}}
	algebra.keyIndex = map[identity.ContentID]uint32{application: 1}
	key, keyOK := algebra.KeyForApplicationID(application)
	if !keyOK {
		t.Fatal("dispatch key")
	}
	for _, predicate := range []uint64{0, uint64(dispatchDispositionInvalid), uint64(999)<<dispatchPredicateDispositionBits | uint64(dispatchDispositionTarget), uint64(1)<<dispatchPredicateDispositionBits | uint64(dispatchDispositionOpaque)} {
		if _, ok := algebra.dispatchValueForPredicate(key, predicate); ok {
			t.Fatalf("forged predicate %d was admitted", predicate)
		}
	}
}
