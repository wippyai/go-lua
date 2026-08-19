package causal

import (
	"github.com/wippyai/go-lua/analysis/identity"
)

// FinalRoute is the single opaque Causal projection consumed by Program route
// facades. Endpoint and optional decision/Mu Site handles are issued here;
// callers never reconstruct them from raw successor Terms.
type FinalRoute struct {
	owner      *Result
	successor  Successor
	from       Site
	to         Site
	fromPoint  WTOPoint
	toPoint    WTOPoint
	decision   Site
	mu         Site
	identity   RouteIdentity
	semanticID identity.ContentID
	fromPath   identity.ContentID
	toPath     identity.ContentID
	guard      RouteGuardProof
	recurrence RouteRecurrence
	guarded    bool
	component  bool
}

// FinalAt reissues one sealed union route as a complete final-route proof.
// Every derived fact is resolved once here against the issuing owner; a route
// whose identity, semantic ID, endpoint points, guard, or recurrence proof is
// unavailable has no final projection.
func (v Successors) FinalAt(index int) (FinalRoute, bool) {
	successor, ok := v.TotalAt(index)
	if !ok {
		return FinalRoute{}, false
	}
	return v.result.finalRoute(successor)
}

func (r *Result) finalRoute(successor Successor) (FinalRoute, bool) {
	routeIdentity, identityOK := successor.Identity()
	semanticID, semanticIDOK := successor.SemanticID()
	fromPoint, fromPointOK := successor.FromPoint()
	toPoint, toPointOK := successor.ToPoint()
	if !identityOK || !semanticIDOK || !fromPointOK || !toPointOK {
		return FinalRoute{}, false
	}
	route := FinalRoute{owner: r, successor: successor, fromPoint: fromPoint, toPoint: toPoint,
		identity: routeIdentity, semanticID: semanticID, fromPath: fromPoint.PathID(), toPath: toPoint.PathID(),
		guarded: successor.Decision != 0}
	if from, ok := r.SiteForTerm(successor.From); ok {
		route.from = from
	}
	if to, ok := r.SiteForTerm(successor.To); ok {
		route.to = to
	}
	if successor.Decision != 0 {
		if decision, ok := r.SiteForTerm(successor.Decision); ok {
			route.decision = decision
		}
	}
	if successor.Mu != 0 {
		if mu, ok := r.SiteForTerm(successor.Mu); ok {
			route.mu = mu
		}
	}
	route.guard, _ = successor.GuardProof()
	if route.guarded && !route.guard.Available() {
		return FinalRoute{}, false
	}
	if _, componentOK := successor.Component(); componentOK {
		route.component = true
		route.recurrence, _ = successor.Recurrence()
		if !route.recurrence.Available() {
			return FinalRoute{}, false
		}
	} else if route.recurrence.Available() {
		return FinalRoute{}, false
	}
	return route, route.Available()
}

// OwnsFinalRoute authenticates a final route issued by this exact owner.
func (r *Result) OwnsFinalRoute(route FinalRoute) bool {
	return r != nil && route.owner == r && route.Available()
}

func (route FinalRoute) Available() bool {
	if route.owner == nil || !route.successor.IssuedBy(route.owner) {
		return false
	}
	issuedIdentity, identityOK := route.successor.Identity()
	semanticID, semanticOK := route.successor.SemanticID()
	if !identityOK || !semanticOK || issuedIdentity != route.identity || semanticID != route.semanticID ||
		!route.fromPath.Available() || !route.toPath.Available() || route.guarded != (route.successor.Decision != 0) {
		return false
	}
	return true
}

func (route FinalRoute) Identity() (RouteIdentity, bool) {
	if !route.Available() {
		return RouteIdentity{}, false
	}
	return route.identity, true
}

// ID returns the parent-issued opaque route identity used for semantic joins.
// Its raw endpoint preimage remains private to Causal validation.
func (route FinalRoute) ID() (identity.ContentID, bool) {
	if !route.Available() {
		return identity.ContentID{}, false
	}
	return route.semanticID, route.semanticID.Available()
}

func (route FinalRoute) From() (Site, bool) {
	if !route.Available() || !route.from.Available() {
		return Site{}, false
	}
	return route.from, true
}

func (route FinalRoute) To() (Site, bool) {
	if !route.Available() || !route.to.Available() {
		return Site{}, false
	}
	return route.to, true
}

func (route FinalRoute) FromPoint() (WTOPoint, bool) {
	if !route.Available() {
		return WTOPoint{}, false
	}
	return route.fromPoint, true
}

func (route FinalRoute) ToPoint() (WTOPoint, bool) {
	if !route.Available() {
		return WTOPoint{}, false
	}
	return route.toPoint, true
}

func (route FinalRoute) FromPath() (identity.ContentID, bool) {
	if !route.Available() {
		return identity.ContentID{}, false
	}
	return route.fromPath, true
}

func (route FinalRoute) ToPath() (identity.ContentID, bool) {
	if !route.Available() {
		return identity.ContentID{}, false
	}
	return route.toPath, true
}

func (route FinalRoute) OwnsSite(site Site) bool {
	return route.Available() && route.owner.OwnsSite(site) &&
		(site == route.from || site == route.to || site == route.decision || site == route.mu)
}

func (route FinalRoute) Decision() (Site, bool) {
	if !route.Available() || !route.decision.Available() || !route.owner.OwnsSite(route.decision) {
		return Site{}, false
	}
	return route.decision, true
}

func (route FinalRoute) Mu() (Site, bool) {
	if !route.Available() || !route.mu.Available() || !route.owner.OwnsSite(route.mu) {
		return Site{}, false
	}
	return route.mu, true
}

func (route FinalRoute) Arm() (BoundaryArmKind, bool) {
	if !route.Available() {
		return 0, false
	}
	return route.successor.Arm, true
}

func (route FinalRoute) GuardProof() (RouteGuardProof, bool) {
	if !route.Available() || !route.guarded || !route.guard.Available() || !route.owner.OwnsRouteGuardProof(route.guard) {
		return RouteGuardProof{}, false
	}
	return route.guard, true
}

func (route FinalRoute) Guarded() bool {
	return route.Available() && route.guarded
}

func (route FinalRoute) Recurrence() (RouteRecurrence, bool) {
	if !route.Available() || !route.component || !route.recurrence.Available() || !route.owner.OwnsRouteRecurrence(route.recurrence) {
		return RouteRecurrence{}, false
	}
	return route.recurrence, true
}

func (route FinalRoute) Component() (Site, bool) {
	proof, ok := route.Recurrence()
	if !ok {
		return Site{}, false
	}
	term, ok := proof.Component()
	if !ok {
		return Site{}, false
	}
	site, ok := route.owner.SiteForTerm(term)
	return site, ok && route.owner.OwnsSite(site)
}

func (route FinalRoute) HasMu() bool {
	proof, ok := route.Recurrence()
	return ok && proof.HasMu()
}

func (route FinalRoute) MuPathID() (identity.ContentID, bool) {
	proof, ok := route.Recurrence()
	if !ok {
		return identity.ContentID{}, false
	}
	return proof.MuPathID()
}

func (route FinalRoute) ResetCount() (int, bool) {
	if !route.Available() {
		return 0, false
	}
	return route.successor.ResetCount()
}

// ResetPathAt returns the immutable semantic path for one reset member.
// It does not expose the authored decision term or a Site context identity.
func (route FinalRoute) ResetPathAt(index int) (identity.ContentID, bool) {
	if !route.Available() || index < 0 {
		return identity.ContentID{}, false
	}
	return route.successor.ResetPathAt(index)
}

func (route FinalRoute) HasResetWitness() bool {
	return route.Available() && route.successor.HasResetWitness()
}

func (route FinalRoute) ResetAt(index int) (Site, bool) {
	if !route.Available() || index < 0 {
		return Site{}, false
	}
	term, ok := route.successor.ResetAt(index)
	if !ok {
		return Site{}, false
	}
	site, ok := route.owner.SiteForTerm(term)
	return site, ok && route.owner.OwnsSite(site)
}

func (route FinalRoute) ResetContains(decision Site) bool {
	if !route.Available() || !decision.Available() || !route.owner.OwnsSite(decision) {
		return false
	}
	term, ok := decision.Term()
	return ok && route.successor.ResetContains(term)
}
