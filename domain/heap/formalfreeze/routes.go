package formalfreeze

import (
	"github.com/wippyai/go-lua/analysis/engine/operand"
	calldomain "github.com/wippyai/go-lua/domain/call"
	"github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/heap/internal/recentplan"
	"github.com/wippyai/go-lua/domain/materialization"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// DeriveFreezeRoutes is the sole formal-freeze relation derivation: the exact
// Heap routes one mounted call justifies freezing.
//
// Each known target of the call contributes the Recent allocation roots its
// own exact Freeze rows select, and the relation is their route-tag
// INTERSECTION, so aliases still match when authored actual ordinals differ
// and a mixed target set writes nothing strong. Semantic uncertainty -
// unresolved, open, opaque or ambiguous evidence - is an empty, valid relation:
// the rule then settles its authenticated empty selection rather than
// fabricating Heap state. Malformed owner authority is a failed relation.
//
// The call's actuals arrive as the ordered cells the selected join answered,
// because a route set computed from every actual cannot be built one actual at
// a time. This signature is derived from the declaration; it is not chosen
// here.
func DeriveFreezeRoutes(
	schema heap.Schema,
	values *valuedomain.Schema,
	calls *calldomain.Algebra,
	packs *packdomain.Schema,
	candidate calldomain.CallCoordinate,
	callFact calldomain.Value,
	actuals operand.SummaryVector[valuedomain.Value],
) (recentplan.Plan, bool) {
	if packs == nil || calls == nil || !calls.Valid() || !schema.Valid() || values == nil || !values.Valid() ||
		!calls.OwnsCallCoordinate(candidate) || !values.OwnsHeapSchema(schema) ||
		!values.LinkOwner().Matches(calls.LinkOwner()) || !packs.LinkOwner().Available() ||
		!packs.LinkOwner().Matches(calls.LinkOwner()) {
		return recentplan.Plan{}, false
	}
	targetContract, contractOK := calls.TargetContract()
	if !contractOK {
		return recentplan.Plan{}, false
	}
	mounted, mountedOK := candidate.MountedCall()
	_, callID, module, _, _, identityOK := calls.MountedCallIdentity(mounted)
	actual, actualOK := packs.MountedActualProjection(module, callID)
	key, keyOK := calls.KeyForMountedCall(mounted)
	if !mountedOK || !identityOK || !actualOK || !actual.Valid() || !actual.OwnedBy(packs) ||
		!keyOK || !key.Valid() || !key.IsApplication() || !actuals.Valid() || actuals.Count() != actual.ActualCount() ||
		!calls.Admits(key, callFact) {
		return recentplan.Plan{}, false
	}
	// The vector is this call's own member set in its own ordinal order, and a
	// cell's POSITION is the ordinal its owner declared it at. There is no tag
	// to agree with: a correspondence proved by re-reading a tag beside every
	// cell would be a second addressing authority for rows the owner already
	// addressed by position.
	_, runtimeTail := actual.TailID()
	if !callFact.IsComplete() || callFact.IsEmpty() || callFact.KnownTargetCount() == 0 {
		return recentplan.Plan{}, true
	}
	if callFact.HasOpaqueAlternative() || callFact.IsTop() {
		return recentplan.Plan{}, true
	}
	var intersection recentplan.Plan
	haveRoutes := false
	for index := 0; index < callFact.KnownTargetCount(); index++ {
		target, targetOK := callFact.KnownTargetAt(index)
		if !targetOK || !calls.OwnsTarget(target) {
			return recentplan.Plan{}, true
		}
		params, found, targetOK := freezeParamsForTarget(targetContract, target, actual.ActualCount(), runtimeTail)
		if !targetOK || !found {
			return recentplan.Plan{}, true
		}
		var targetRoutes recentplan.Plan
		for paramIndex := 0; paramIndex < params.count(); paramIndex++ {
			param, paramOK := params.at(paramIndex)
			if !paramOK || param < 0 || param >= actuals.Count() {
				return recentplan.Plan{}, true
			}
			root, rooted, admitted := freezeActualRoot(values, actuals, param)
			if !admitted {
				return recentplan.Plan{}, false
			}
			if !rooted {
				return recentplan.Plan{}, true
			}
			tag, tagOK := schema.RouteTag(root, materialization.Recent)
			if !tagOK {
				return recentplan.Plan{}, false
			}
			if !targetRoutes.Add(recentplan.Route{Key: root, Tag: tag}) {
				return recentplan.Plan{}, false
			}
		}
		if !haveRoutes {
			intersection = targetRoutes
			haveRoutes = true
			continue
		}
		var intersectionOK bool
		intersection, intersectionOK = intersection.Intersection(targetRoutes)
		if !intersectionOK {
			return recentplan.Plan{}, false
		}
		// Once no exact root is common to the known alternatives, no later
		// target can restore a strong write. This is the common mixed-target
		// case and avoids traversing the remaining formal rows.
		if intersection.Count() == 0 {
			return recentplan.Plan{}, true
		}
	}
	if !haveRoutes || intersection.Count() == 0 {
		return recentplan.Plan{}, true
	}
	for index := 0; index < intersection.Count(); index++ {
		route, routeOK := intersection.At(index)
		if !routeOK || !route.Key.Valid() || route.Key.Kind() != heap.RootAllocation || !schema.OwnsKey(route.Key) || route.Tag == 0 {
			return recentplan.Plan{}, false
		}
	}
	return intersection, true
}

// FreezeRouteCount is the direct composition accessor for a derived plan.
func FreezeRouteCount(plan recentplan.Plan) int { return plan.Count() }

// FreezeRouteAt is the direct composition accessor for one derived route, in
// the plan's canonical route-tag order.
func FreezeRouteAt(plan recentplan.Plan, index int) (recentplan.Route, bool) { return plan.At(index) }
