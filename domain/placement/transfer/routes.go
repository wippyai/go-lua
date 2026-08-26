package transfer

import (
	"github.com/wippyai/go-lua/analysis/engine/operand"
	calldomain "github.com/wippyai/go-lua/domain/call"
	"github.com/wippyai/go-lua/domain/heap"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	"github.com/wippyai/go-lua/domain/placement"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// Route is one exact Placement route a mounted transfer justifies: the
// allocation root the demand names, and the owner-issued tag that root is
// published at. Both identities are issued by Placement's schema; this row
// reinterprets neither.
type Route struct {
	Key heap.Key
	Tag uint64
}

// Coordinates is the TransferRoutes relation's Key/Destination accessor. A
// transfer displaces the world at the very root it reads, so the key and the
// destination are one coordinate - but which role a projection plays is the
// declaration's statement, so both stay separately declared.
func (route Route) Coordinates() (key, destination heap.Key, ok bool) {
	return route.Key, route.Key, route.valid()
}

// Predicate is the TransferRoutes relation's declared selection tag: the route
// coordinate this member is published at, paired with the destination
// Coordinates answers. It carries the Send policy code the demand was
// authenticated under, which is what the fold reads it for.
func (route Route) Predicate() (uint64, bool) {
	return route.Tag, route.valid()
}

// valid is the row fence both projections answer under: an exact, live
// allocation root carrying a nonzero owner-issued route tag.
func (route Route) valid() bool {
	return route.Key.Valid() && route.Key.Kind() == heap.RootAllocation && route.Tag != 0
}

// RoutePlan is the TransferRoutes relation's declared Derivation state. It is
// a thin exported view over the same route algebra the generated family's
// worker enumerates; TransferRouteCount and TransferRouteAt never re-derive a
// route.
type RoutePlan struct {
	plan routePlan
}

// DeriveTransferRoutes is the sole TransferRoutes derivation: the exact
// Placement routes one mounted call's authenticated Target transfer rows
// justify displacing to Send.
//
// The call's actuals arrive as the whole vector Value published for the call,
// because a demand set computed from every actual cannot be built one actual
// at a time. An authenticated opaque payload widens the demand to every Send
// root rather than naming one, which is the conservative answer the Target
// declaration authorizes. Semantic uncertainty is an empty, valid relation:
// the rule then settles its authenticated empty selection rather than
// fabricating Placement state. Malformed owner authority is a failed relation.
//
// This signature is derived from the declaration; it is not chosen here.
func DeriveTransferRoutes(
	schema placement.Schema,
	values *valuedomain.Schema,
	calls *calldomain.Algebra,
	packs *packdomain.Schema,
	candidate calldomain.CallCoordinate,
	callFact calldomain.Value,
	actuals operand.SummaryVector[valuedomain.Value],
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

// TransferRouteCount is the direct composition accessor for a derived plan.
func TransferRouteCount(plan RoutePlan) int { return plan.plan.routeCount() }

// TransferRouteAt is the direct composition accessor for one derived route, in
// the plan's canonical dense-coordinate order.
func TransferRouteAt(plan RoutePlan, index int) (Route, bool) {
	row, rowOK := plan.plan.routeAt(index)
	if !rowOK {
		return Route{}, false
	}
	return Route{Key: row.key, Tag: uint64(row.tag)}, true
}
