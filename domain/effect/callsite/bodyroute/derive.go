// Package bodyroute derives the Effect roots one mounted call site dispatches
// its executable bodies to.
//
// It is the declared relation the interprocedural call-site rule selects over:
// the members are the bodies THIS call may reach, in the canonical order the
// Effect algebra numbers its roots by, and the tag each member carries is that
// root's own coordinate. Nothing here reads a Factor cell - which bodies a call
// reaches is Call's cold answer, and what each body's effect IS is the cell the
// selection observes at the member this derivation named.
package bodyroute

import (
	"slices"

	calldomain "github.com/wippyai/go-lua/domain/call"
	effectfactor "github.com/wippyai/go-lua/domain/effect/factor"
)

// Route is one body this call dispatches to: the Effect root its own effect is
// published under, and the tag that root is correlated by. The tag IS the
// root's dense coordinate, so a consumer holding the Effect algebra recovers
// the root from the tag rather than carrying a second copy of it.
type Route struct {
	root effectfactor.Root
	tag  uint64
	set  bool
}

// Coordinate is the Effect coordinate this member's cell is observed at.
func (route Route) Coordinate() (effectfactor.Root, bool) { return route.root, route.set }

// Predicate is the owner-issued tag this member is correlated by.
func (route Route) Predicate() (uint64, bool) { return route.tag, route.set }

// Plan is one call site's whole ordered member set.
type Plan struct{ routes []Route }

// Count is the member census this selection spans.
func Count(plan Plan) int { return len(plan.routes) }

// At projects one member in the canonical order Derive fixed.
func At(plan Plan, index int) (Route, bool) {
	if index < 0 || index >= len(plan.routes) {
		return Route{}, false
	}
	return plan.routes[index], true
}

// Derive answers the bodies this call site reaches.
//
// A Top call value reaches every body there is, so the member set is Call's
// whole body prefix. Otherwise each declared target is authenticated against
// the Call algebra's canonical row for its role: a body target contributes its
// own root, a seed target belongs to the two exact call-site rules and is
// skipped here, and anything else is a target this rule has no reading for.
//
// The order is ascending root coordinate, which is the order the engine
// canonicalizes a selection by, and two targets resolving to one root address
// one member twice - so the set is refused rather than folded twice.
func Derive(effects *effectfactor.Algebra, calls *calldomain.Algebra, mounted effectfactor.MountedCall, value calldomain.Value) (Plan, bool) {
	if effects == nil || !effects.Valid() || calls == nil || !calls.Valid() ||
		!calls.LinkOwner().Matches(effects.LinkOwner()) {
		return Plan{}, false
	}
	_, module, occurrence, identityOK := effects.MountedCallIdentity(mounted)
	_, key, keyOK := calls.MountedCallKeyForOccurrence(module, occurrence)
	if !identityOK || !keyOK || !calls.Admits(key, value) {
		return Plan{}, false
	}
	routes, ok := members(effects, calls, value)
	if !ok {
		return Plan{}, false
	}
	ordered, orderOK := order(routes)
	if !orderOK {
		return Plan{}, false
	}
	return Plan{routes: ordered}, true
}

// order fixes the canonical member order of one call site's route set.
//
// The engine canonicalizes a selection by ascending tag, and every route is
// tagged with the Effect root coordinate its cell is read at, so ascending tag
// IS that canonical order. Call's target order and Effect's root order are
// separate authorities, and a member ordinal is taken from Effect's.
//
// Two routes on one tag address one member twice. The selection has no second
// ordinal for the repeat, so the set is refused rather than observed twice.
func order(routes []Route) ([]Route, bool) {
	slices.SortFunc(routes, func(left, right Route) int {
		switch {
		case left.tag < right.tag:
			return -1
		case left.tag > right.tag:
			return 1
		default:
			return 0
		}
	})
	for index := 1; index < len(routes); index++ {
		if routes[index].tag == routes[index-1].tag {
			return nil, false
		}
	}
	return routes, true
}

func members(effects *effectfactor.Algebra, calls *calldomain.Algebra, value calldomain.Value) ([]Route, bool) {
	if value.IsTop() {
		bodies := calls.Bodies()
		routes := make([]Route, 0, bodies.Count())
		for index := 0; index < bodies.Count(); index++ {
			body, bodyOK := bodies.At(index)
			if !bodyOK {
				return nil, false
			}
			route, routeOK := routeForBody(effects, body)
			if !routeOK {
				return nil, false
			}
			routes = append(routes, route)
		}
		return routes, true
	}
	routes := make([]Route, 0, value.KnownTargetCount())
	for index := 0; index < value.KnownTargetCount(); index++ {
		target, targetOK := value.KnownTargetAt(index)
		role, roleOK := target.RoleID()
		canonical, canonicalOK := calls.TargetForRole(role)
		if !targetOK || !roleOK || !canonicalOK || !canonical.Same(target) {
			return nil, false
		}
		switch role.Kind() {
		case calldomain.TargetRoleSeed:
			continue
		case calldomain.TargetRoleBody:
		default:
			return nil, false
		}
		body, bodyOK := canonical.Body()
		if !bodyOK {
			return nil, false
		}
		route, routeOK := routeForBody(effects, body)
		if !routeOK {
			return nil, false
		}
		routes = append(routes, route)
	}
	return routes, true
}

func routeForBody(effects *effectfactor.Algebra, body calldomain.Body) (Route, bool) {
	moduleKey, moduleOK := body.ModuleKey()
	programID, programOK := body.ProgramID()
	bodyID, bodyIDOK := body.BodyPath()
	if !moduleOK || !programOK || !bodyIDOK {
		return Route{}, false
	}
	root, rootOK := effects.RootForMountedBodyID(moduleKey, programID, bodyID)
	index, indexOK := effects.RootIndex(root)
	if !rootOK || !indexOK || index < 0 {
		return Route{}, false
	}
	return Route{root: root, tag: uint64(index), set: true}, true
}
