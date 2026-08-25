// Package bodyroute answers which Effect root one body a call may reach
// publishes its effect under.
//
// It is the judgment behind a DECLARED relation: the member set itself - what
// to enumerate, how to union it, what to widen to when the call names no
// alternatives, and the order the members come back in - is stated by the
// relation and written by the emitter. What is left here is the one question
// only this domain can answer, which is what a single call target means to
// Effect.
package bodyroute

import (
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

// ResolveRoute answers what one call target contributes to a mounted site's
// member set.
//
// A target is authenticated against the Call algebra's canonical row for its
// role before it is read, so a value naming a target its owner does not is
// refused rather than resolved. A SEED target belongs to the two exact
// call-site rules and contributes no member here - that is an absence, not a
// refusal, which is how a judgment declines an item without failing the set.
// A BODY target contributes the Effect root its own effect is published under.
func ResolveRoute(effects *effectfactor.Algebra, calls *calldomain.Algebra, mounted effectfactor.MountedCall, target calldomain.Target) (Route, bool, bool) {
	if effects == nil || !effects.Valid() || calls == nil || !calls.Valid() ||
		!calls.LinkOwner().Matches(effects.LinkOwner()) || !mounted.Valid() {
		return Route{}, false, false
	}
	role, roleOK := target.RoleID()
	canonical, canonicalOK := calls.TargetForRole(role)
	if !roleOK || !canonicalOK || !canonical.Same(target) {
		return Route{}, false, false
	}
	switch role.Kind() {
	case calldomain.TargetRoleSeed:
		return Route{}, false, true
	case calldomain.TargetRoleBody:
	default:
		return Route{}, false, false
	}
	body, bodyOK := canonical.Body()
	if !bodyOK {
		return Route{}, false, false
	}
	moduleKey, moduleOK := body.ModuleKey()
	programID, programOK := body.ProgramID()
	bodyID, bodyIDOK := body.BodyPath()
	if !moduleOK || !programOK || !bodyIDOK {
		return Route{}, false, false
	}
	root, rootOK := effects.RootForMountedBodyID(moduleKey, programID, bodyID)
	index, indexOK := effects.RootIndex(root)
	if !rootOK || !indexOK || index < 0 {
		return Route{}, false, false
	}
	return Route{root: root, tag: uint64(index), set: true}, true, true
}

// BeyondTargets answers whether a call site's member set is beyond
// enumeration: the call named no closed list of alternatives, so the bodies it
// reaches are every body there is rather than the ones written down. It
// answers its own validity beside that, because it is the one judgment of this
// derivation that runs whether or not the call names a single target - a site
// whose call value is empty reaches nothing else that could authenticate it.
//
// It is asked of the same things ResolveRoute is asked of, minus the target
// there is not one of yet, because whether a set has a closed list can depend
// on what the set is OF.
func BeyondTargets(effects *effectfactor.Algebra, calls *calldomain.Algebra, mounted effectfactor.MountedCall, fact calldomain.Value) (bool, bool) {
	if effects == nil || !effects.Valid() || calls == nil || !calls.Valid() ||
		!calls.LinkOwner().Matches(effects.LinkOwner()) || !mounted.Valid() {
		return false, false
	}
	return fact.IsTop(), true
}
