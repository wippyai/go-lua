package formal

import (
	"github.com/wippyai/go-lua/analysis/engine/execution"
	calldomain "github.com/wippyai/go-lua/domain/call"
	"github.com/wippyai/go-lua/domain/heap"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	"github.com/wippyai/go-lua/domain/placement"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// Route is one exact Placement route a mounted call's formal ownership rows
// justify: the allocation root the demand names, and the owner-issued tag it
// is published at. The tag carries the escape the row authored, or the
// unknown code where the formal declaration authenticated a widening, so the
// fold reads the policy out of the same identity the selection is correlated
// by.
type Route struct {
	Key heap.Key
	Tag uint64
}

// Coordinates is the FormalRoutes relation's Key/Destination accessor. A
// formal row displaces the world at the very root it reads, so the key and
// the destination are one coordinate under two declared roles.
func (route Route) Coordinates() (key, destination heap.Key, ok bool) {
	return route.Key, route.Key, route.valid()
}

// Predicate is the FormalRoutes relation's declared selection tag.
func (route Route) Predicate() (uint64, bool) {
	return route.Tag, route.valid()
}

func (route Route) valid() bool {
	return route.Key.Valid() && route.Key.Kind() == heap.RootAllocation && route.Tag != 0
}

// RoutePlan is the FormalRoutes relation's declared Derivation state: a thin
// exported view over the same route algebra the generated family enumerates.
type RoutePlan struct {
	plan routePlan
}

// DeriveFormalRoutes is the sole FormalRoutes derivation: the exact Placement
// routes the formal ownership rows of every known target of one mounted call
// justify.
//
// The call's actuals arrive as the whole vector Value published for it,
// because a demand set reduced over every formal selector range cannot be
// built one actual at a time. A target that denotes no operation carries no
// formal ownership metadata and contributes nothing; an opaque-only dispatch
// is a valid no-route plan rather than a widened one, because the opaque arm
// issues no Target capability. Malformed owner authority is a failed relation.
//
// This signature is derived from the declaration; it is not chosen here.
func DeriveFormalRoutes(
	schema placement.Schema,
	values *valuedomain.Schema,
	calls *calldomain.Algebra,
	packs *packdomain.Schema,
	candidate calldomain.CallCoordinate,
	callFact calldomain.Value,
	actuals execution.SummaryVector[valuedomain.Value],
) (RoutePlan, bool) {
	if calls == nil || !calls.Valid() || !calls.OwnsCallCoordinate(candidate) {
		return RoutePlan{}, false
	}
	targetContract, contractOK := calls.TargetContract()
	mounted, mountedOK := candidate.MountedCall()
	if !contractOK || !mountedOK {
		return RoutePlan{}, false
	}
	plan, planOK := planFor(packs, calls, schema, values, targetContract, mounted, callFact, actuals)
	if !planOK {
		return RoutePlan{}, false
	}
	return RoutePlan{plan: plan}, true
}

// FormalRouteCount is the direct composition accessor for a derived plan.
func FormalRouteCount(plan RoutePlan) int { return plan.plan.routeCount() }

// FormalRouteAt is the direct composition accessor for one derived route, in
// the plan's canonical dense-coordinate order.
func FormalRouteAt(plan RoutePlan, index int) (Route, bool) {
	row, rowOK := plan.plan.routeAt(index)
	if !rowOK {
		return Route{}, false
	}
	return Route{Key: row.key, Tag: uint64(row.tag)}, true
}
