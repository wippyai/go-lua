package flow

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/causal"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/continuation"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// Edge, CallBoundary, and Successor are public value copies. Their owner
// Results remain private and immutable; no internal package type appears in a
// public method signature.
type Edge struct {
	From     keyspace.Term
	To       keyspace.Term
	Decision keyspace.Term
	Truth    bool
	Mu       keyspace.Term
}

type CallBoundary struct {
	Call       keyspace.Term
	Normal     keyspace.Term
	Other      keyspace.Term
	TailReturn keyspace.Term
	Throw      keyspace.Term
	Yield      keyspace.Term
	Cancel     keyspace.Term
}

type BoundaryArmKind uint8

const (
	BoundaryLocal BoundaryArmKind = iota + 1
	BoundaryResume
	BoundarySelectTrue
	BoundarySelectFalse
	BoundaryTail
	BoundaryThrow
	BoundaryYield
	BoundaryCancel
)

type Successor struct {
	From     keyspace.Term
	To       keyspace.Term
	Decision keyspace.Term
	Truth    bool
	Mu       keyspace.Term
	Arm      BoundaryArmKind
	route    causal.Successor
}

// RouteIdentity is a stable semantic route value. It contains only canonical
// endpoints, guard/Mu semantics, the closed arm kind, a full reset-set digest,
// and the exact four-owner provenance fence. Physical row indexes and
// successor ordinals are deliberately absent.
type RouteIdentity struct {
	sourceID    identity.ContentID
	flowID      identity.ContentID
	staticID    identity.ContentID
	moduleID    identity.ContentID
	from        keyspace.Term
	to          keyspace.Term
	decision    keyspace.Term
	truth       bool
	mu          keyspace.Term
	arm         BoundaryArmKind
	resetDigest identity.ContentID
	resetCount  uint32
	digest      identity.ContentID
}

func publicRouteIdentity(identity causal.RouteIdentity) RouteIdentity {
	return RouteIdentity{
		sourceID: identity.SourceID, flowID: identity.FlowID, staticID: identity.StaticID, moduleID: identity.ModuleID,
		from: identity.From, to: identity.To, decision: identity.Decision, truth: identity.Truth, mu: identity.Mu,
		arm: BoundaryArmKind(identity.Arm), resetDigest: identity.ResetDigest, resetCount: identity.ResetCount,
		digest: identity.Digest,
	}
}

func (identity RouteIdentity) causal() causal.RouteIdentity {
	return causal.RouteIdentity{
		SourceID: identity.sourceID, FlowID: identity.flowID, StaticID: identity.staticID, ModuleID: identity.moduleID,
		From: identity.from, To: identity.to, Decision: identity.decision, Truth: identity.truth, Mu: identity.mu,
		Arm: causal.BoundaryArmKind(identity.arm), ResetDigest: identity.resetDigest, ResetCount: identity.resetCount,
		Digest: identity.digest,
	}
}

// Available reports whether identity came from a sealed Causal route.
func (identity RouteIdentity) Available() bool {
	// Route identities are issued by Causal's sealed route directory.  The
	// public value has no setters or physical row capability, so consuming it
	// only needs the owner's issued-shape fence; re-hashing the same preimage
	// on every Artifact query would rederive an identity Flow already issued.
	return identity.causal().Issued()
}

func (identity RouteIdentity) Equal(other RouteIdentity) bool {
	return identity == other
}

func (identity RouteIdentity) Digest() identity.ContentID      { return identity.digest }
func (identity RouteIdentity) ResetDigest() identity.ContentID { return identity.resetDigest }
func (identity RouteIdentity) Provenance() Provenance {
	return Provenance{Source: identity.sourceID, Flow: identity.flowID, Static: identity.staticID, Module: identity.moduleID}
}
func (identity RouteIdentity) From() keyspace.Term     { return identity.from }
func (identity RouteIdentity) To() keyspace.Term       { return identity.to }
func (identity RouteIdentity) Decision() keyspace.Term { return identity.decision }
func (identity RouteIdentity) Truth() bool             { return identity.truth }
func (identity RouteIdentity) Mu() keyspace.Term       { return identity.mu }
func (identity RouteIdentity) Arm() BoundaryArmKind    { return identity.arm }
func (identity RouteIdentity) ResetCount() int         { return int(identity.resetCount) }

func publicEdge(edge causal.Edge) Edge {
	return Edge{From: edge.From, To: edge.To, Decision: edge.Decision, Truth: edge.Truth, Mu: edge.Mu}
}

func publicBoundary(boundary causal.CallBoundary) CallBoundary {
	return CallBoundary{Call: boundary.Call, Normal: boundary.Normal, Other: boundary.Other,
		TailReturn: boundary.TailReturn, Throw: boundary.Throw, Yield: boundary.Yield, Cancel: boundary.Cancel}
}

func publicSuccessor(successor causal.Successor) Successor {
	return Successor{From: successor.From, To: successor.To, Decision: successor.Decision,
		Truth: successor.Truth, Mu: successor.Mu, Arm: BoundaryArmKind(successor.Arm), route: successor}
}

func (successor Successor) Identity() (RouteIdentity, bool) {
	identity, ok := successor.route.Identity()
	return publicRouteIdentity(identity), ok
}
func (successor Successor) SemanticID() (identity.ContentID, bool) {
	return successor.route.SemanticID()
}
func (successor Successor) FromPoint() (WTOPoint, bool) {
	return successor.route.FromPoint()
}
func (successor Successor) ToPoint() (WTOPoint, bool) {
	return successor.route.ToPoint()
}

// WTORegionID returns the sole parent-issued local schedule membership for
// this final route. A zero ID denotes a cross-region/acyclic route.
func (successor Successor) WTORegionID() identity.ContentID { return successor.route.WTORegionID() }
func (successor Successor) ResetCount() (int, bool)         { return successor.route.ResetCount() }
func (successor Successor) ResetAt(offset int) (keyspace.Term, bool) {
	return successor.route.ResetAt(offset)
}
func (successor Successor) ResetPathAt(offset int) (identity.ContentID, bool) {
	return successor.route.ResetPathAt(offset)
}
func (successor Successor) ResetContains(decision keyspace.Term) bool {
	return successor.route.ResetContains(decision)
}
func (successor Successor) HasResetWitness() bool { return successor.route.HasResetWitness() }

// FinalRoute is the single opaque Flow projection consumed by Program route
// facades. Endpoint and optional decision/Mu Site handles are issued here;
// callers never reconstruct them from raw successor Terms.
type FinalRoute struct {
	owner      *causal.Result
	successor  Successor
	from       Site // optional legacy attachment
	to         Site // optional legacy attachment
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

func publicFinalRoute(result *causal.Result, successor causal.Successor) (FinalRoute, bool) {
	public := publicSuccessor(successor)
	identity, identityOK := public.Identity()
	semanticID, semanticIDOK := public.SemanticID()
	fromPoint, fromPointOK := public.FromPoint()
	toPoint, toPointOK := public.ToPoint()
	if !identityOK || !semanticIDOK || !fromPointOK || !toPointOK {
		return FinalRoute{}, false
	}
	route := FinalRoute{owner: result, successor: public, fromPoint: fromPoint, toPoint: toPoint, identity: identity, semanticID: semanticID, fromPath: fromPoint.PathID(), toPath: toPoint.PathID(), guarded: successor.Decision != 0}
	if from, ok := result.SiteForTerm(successor.From); ok {
		route.from = from
	}
	if to, ok := result.SiteForTerm(successor.To); ok {
		route.to = to
	}
	if successor.Decision != 0 {
		if decision, ok := result.SiteForTerm(successor.Decision); ok {
			route.decision = decision
		}
	}
	if successor.Mu != 0 {
		if mu, ok := result.SiteForTerm(successor.Mu); ok {
			route.mu = mu
		}
	}
	route.guard, _ = public.GuardProof()
	if route.guarded && !route.guard.Available() {
		return FinalRoute{}, false
	}
	if _, componentOK := public.Component(); componentOK {
		route.component = true
		route.recurrence, _ = public.Recurrence()
		if !route.recurrence.Available() {
			return FinalRoute{}, false
		}
	} else if route.recurrence.Available() {
		return FinalRoute{}, false
	}
	return route, route.Available()
}

func (route FinalRoute) Available() bool {
	if route.owner == nil || !route.successor.route.IssuedBy(route.owner) {
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
	if !route.Available() || !route.guarded || !route.guard.Available() || !route.owner.OwnsRouteGuardProof(route.guard.proof) {
		return RouteGuardProof{}, false
	}
	return route.guard, true
}

func (route FinalRoute) Guarded() bool {
	return route.Available() && route.guarded
}

func (route FinalRoute) Recurrence() (RouteRecurrence, bool) {
	if !route.Available() || !route.component || !route.recurrence.Available() || !route.owner.OwnsRouteRecurrence(route.recurrence.proof) {
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

func (view Successors) FinalAt(index int) (FinalRoute, bool) {
	if view.result == nil {
		return FinalRoute{}, false
	}
	successor, ok := view.TotalAt(index)
	if !ok {
		return FinalRoute{}, false
	}
	return publicFinalRoute(view.result, successor.route)
}

func (view Causal) OwnsFinalRoute(route FinalRoute) bool {
	return view.result != nil && route.owner == view.result && route.Available()
}

// Component reports the recurrence-issued Program-local cyclic head for this
// exact final route. A false result denotes an acyclic or cross-component
// route; callers never infer membership from Mu.
func (successor Successor) Component() (keyspace.Term, bool) {
	return successor.route.Component()
}

// RouteRecurrence is the exact recurrence/component proof issued by the
// sealed Causal owner for a cyclic final route. It is opaque to Program: the
// component identity is authored here, not reconstructed from route fields.
type RouteRecurrence struct {
	proof causal.RouteRecurrence
	route RouteIdentity
}

func publicRouteRecurrence(proof causal.RouteRecurrence) RouteRecurrence {
	route, _ := proof.RouteIdentity()
	return RouteRecurrence{proof: proof, route: publicRouteIdentity(route)}
}

func (successor Successor) Recurrence() (RouteRecurrence, bool) {
	proof, ok := successor.route.Recurrence()
	return publicRouteRecurrence(proof), ok
}

// RouteGuardProof is the exact parent-issued guarded disposition of one
// route. Its decision Site projection is optional; Truth and ownership are
// authenticated by the proof itself.
type RouteGuardProof struct{ proof causal.RouteGuardProof }

func (successor Successor) GuardProof() (RouteGuardProof, bool) {
	proof, ok := successor.route.GuardProof()
	return RouteGuardProof{proof: proof}, ok
}

func (proof RouteGuardProof) Available() bool               { return proof.proof.Available() }
func (proof RouteGuardProof) ContextID() identity.ContentID { return proof.proof.ContextID() }
func (proof RouteGuardProof) Truth() (bool, bool)           { return proof.proof.Truth() }
func (proof RouteGuardProof) RouteID() (identity.ContentID, bool) {
	return proof.proof.RouteID()
}
func (proof RouteGuardProof) DecisionPathID() (identity.ContentID, bool) {
	return proof.proof.DecisionPathID()
}
func (proof RouteGuardProof) RouteIdentity() (RouteIdentity, bool) {
	identity, ok := proof.proof.RouteIdentity()
	return publicRouteIdentity(identity), ok
}

func (proof RouteRecurrence) Available() bool { return proof.proof.Available() }

func (proof RouteRecurrence) Equal(other RouteRecurrence) bool {
	if !proof.Available() || !other.Available() || proof.ComponentID() != other.ComponentID() {
		return false
	}
	leftRouteID, leftRouteOK := proof.RouteID()
	rightRouteID, rightRouteOK := other.RouteID()
	leftCount, leftCountOK := proof.ResetCount()
	rightCount, rightCountOK := other.ResetCount()
	leftDigest, leftDigestOK := proof.ResetDigest()
	rightDigest, rightDigestOK := other.ResetDigest()
	leftMuPath, leftMuPathOK := proof.MuPathID()
	rightMuPath, rightMuPathOK := other.MuPathID()
	return leftRouteOK && rightRouteOK && leftRouteID == rightRouteID && proof.HasMu() == other.HasMu() &&
		leftMuPath == rightMuPath && leftMuPathOK == rightMuPathOK && leftCount == rightCount && leftCountOK == rightCountOK &&
		leftDigest == rightDigest && leftDigestOK == rightDigestOK
}

func (proof RouteRecurrence) RouteIdentity() (RouteIdentity, bool) {
	if !proof.Available() {
		return RouteIdentity{}, false
	}
	return proof.route, true
}

func (proof RouteRecurrence) ComponentID() identity.ContentID {
	return proof.proof.ComponentID()
}

func (proof RouteRecurrence) RouteID() (identity.ContentID, bool) {
	return proof.proof.RouteID()
}
func (proof RouteRecurrence) HasMu() bool                          { return proof.proof.HasMu() }
func (proof RouteRecurrence) MuPathID() (identity.ContentID, bool) { return proof.proof.MuPathID() }

func (proof RouteRecurrence) Component() (keyspace.Term, bool) {
	return proof.proof.Component()
}

func (proof RouteRecurrence) Mu() (keyspace.Term, bool) { return proof.proof.Mu() }

func (proof RouteRecurrence) ResetCount() (int, bool) { return proof.proof.ResetCount() }

func (proof RouteRecurrence) ResetDigest() (identity.ContentID, bool) {
	return proof.proof.ResetDigest()
}

type Causal struct{ result *causal.Result }

func (view Causal) Edges() Edges           { return Edges(view) }
func (view Causal) Boundaries() Boundaries { return Boundaries(view) }
func (view Causal) Successors() Successors { return Successors(view) }
func (view Causal) Sites() Sites           { return Sites(view) }

func (view Causal) OwnsRouteRecurrence(proof RouteRecurrence) bool {
	return view.result != nil && view.result.OwnsRouteRecurrence(proof.proof)
}

func (view Causal) OwnsRouteGuardProof(proof RouteGuardProof) bool {
	return view.result != nil && view.result.OwnsRouteGuardProof(proof.proof)
}

// Site is an opaque exact-quartet-fenced Flow causal site handle for an
// existing route endpoint or sealed body-terminal Outcome coordinate.
// ContextID is stable across equivalent seal/artifact replay, but is not a
// portable identity across a mutated program.
type Site = causal.Site

// Sites is the allocation-free canonical causal endpoint projection.
// Its indexes borrow the one sealed Causal owner and retain no syntax/IR copy.
type Sites struct{ result *causal.Result }

// Owns accepts only a Site issued by this exact hot Causal owner. Unlike
// Equal, it deliberately rejects equivalent artifact replay handles.
func (view Sites) Owns(site Site) bool {
	return view.result != nil && view.result.OwnsSite(site)
}

// Count reports the deduped route-endpoint denominator.
func (view Sites) Count() int {
	if view.result == nil {
		return 0
	}
	return view.result.SiteCount()
}

// At returns one endpoint in canonical Term order.
func (view Sites) At(index int) (Site, bool) {
	if view.result == nil {
		return Site{}, false
	}
	return view.result.SiteAt(index)
}

// ForTerm resolves only Terms that occur as existing route endpoints or sealed
// body-terminal Outcome coordinates.
func (view Sites) ForTerm(term keyspace.Term) (Site, bool) {
	if view.result == nil {
		return Site{}, false
	}
	return view.result.SiteForTerm(term)
}

// ResolveContextID performs an exact-quartet-fenced contextual lookup.
func (view Sites) ResolveContextID(id identity.ContentID) (Site, bool) {
	if view.result == nil {
		return Site{}, false
	}
	return view.result.ResolveContextID(id)
}

type Edges struct{ result *causal.Result }

func (view Edges) Count() int {
	if view.result == nil {
		return 0
	}
	return view.result.Edges().Count()
}
func (view Edges) At(index int) (Edge, bool) {
	if view.result == nil {
		return Edge{}, false
	}
	edge, ok := view.result.Edges().At(index)
	return publicEdge(edge), ok
}
func (view Edges) Decision(index int) (keyspace.Term, bool, bool) {
	if view.result == nil {
		return 0, false, false
	}
	return view.result.Edges().Decision(index)
}
func (view Edges) Mu(index int) (keyspace.Term, bool) {
	if view.result == nil {
		return 0, false
	}
	return view.result.Edges().Mu(index)
}
func (view Edges) ResetCount(index int) (int, bool) {
	if view.result == nil {
		return 0, false
	}
	return view.result.Edges().ResetCount(index)
}
func (view Edges) ResetAt(index, offset int) (keyspace.Term, bool) {
	if view.result == nil {
		return 0, false
	}
	return view.result.Edges().ResetAt(index, offset)
}
func (view Edges) ResetContains(index int, decision keyspace.Term) bool {
	return view.result != nil && view.result.Edges().ResetContains(index, decision)
}
func (view Edges) BodyCount(body keyspace.Term) (int, bool) {
	if view.result == nil {
		return 0, false
	}
	return view.result.Edges().BodyCount(body)
}
func (view Edges) BodyAt(body keyspace.Term, index int) (Edge, bool) {
	if view.result == nil {
		return Edge{}, false
	}
	edge, ok := view.result.Edges().BodyAt(body, index)
	return publicEdge(edge), ok
}
func (view Edges) ActivationCount(body keyspace.Term) (int, bool) {
	if view.result == nil {
		return 0, false
	}
	return view.result.Edges().ActivationCount(body)
}
func (view Edges) ActivationAt(body keyspace.Term, index int) (Edge, bool) {
	if view.result == nil {
		return Edge{}, false
	}
	edge, ok := view.result.Edges().ActivationAt(body, index)
	return publicEdge(edge), ok
}

type Boundaries struct{ result *causal.Result }

func (view Boundaries) Count() int {
	if view.result == nil {
		return 0
	}
	return view.result.Boundaries().Count()
}
func (view Boundaries) At(index int) (CallBoundary, bool) {
	if view.result == nil {
		return CallBoundary{}, false
	}
	boundary, ok := view.result.Boundaries().At(index)
	return publicBoundary(boundary), ok
}
func (view Boundaries) For(call keyspace.Term) (CallBoundary, bool) {
	if view.result == nil {
		return CallBoundary{}, false
	}
	boundary, ok := view.result.Boundaries().For(call)
	return publicBoundary(boundary), ok
}

// Arm returns one exact sealed CallBoundary arm without traversing the union
// successor ranges. The arm vocabulary is closed and the returned route is
// still owned by the same Causal authority.
func (view Boundaries) Arm(call keyspace.Term, arm BoundaryArmKind) (Successor, bool) {
	if view.result == nil {
		return Successor{}, false
	}
	successor, ok := view.result.Boundaries().Arm(call, causal.BoundaryArmKind(arm))
	return publicSuccessor(successor), ok
}

type Successors struct{ result *causal.Result }

func (view Successors) Count(from keyspace.Term) int {
	if view.result == nil {
		return 0
	}
	return view.result.Successors().Count(from)
}

// TotalCount returns the one sealed local-plus-call route denominator in
// canonical publication order. It borrows the causal index and retains no
// second catalog.
func (view Successors) TotalCount() int {
	if view.result == nil {
		return 0
	}
	return view.result.Successors().TotalCount()
}

// TotalAt reissues one route from the sealed union index without rescanning
// endpoint ranges or reconstructing a downstream route table.
func (view Successors) TotalAt(index int) (Successor, bool) {
	if view.result == nil {
		return Successor{}, false
	}
	successor, ok := view.result.Successors().TotalAt(index)
	return publicSuccessor(successor), ok
}
func (view Successors) At(from keyspace.Term, index int) (Successor, bool) {
	if view.result == nil {
		return Successor{}, false
	}
	successor, ok := view.result.Successors().At(from, index)
	return publicSuccessor(successor), ok
}

// AssignmentPredecessor returns the existing owner-issued local Successor
// for an assignment's reverse commit edge. A Write may also have an
// evaluation predecessor; this query selects only the sealed commit route and
// never reconstructs one from authored storage order.
func (view Successors) AssignmentPredecessor(write keyspace.Term) (Successor, bool) {
	if view.result == nil {
		return Successor{}, false
	}
	successor, ok := view.result.Successors().AssignmentPredecessor(write)
	return publicSuccessor(successor), ok
}

// Resolve returns the route represented by identity in this exact sealed
// Causal authority. Equivalent reseals resolve the same identity; foreign,
// malformed, duplicate, or unavailable identities fail closed.
func (view Successors) Resolve(identity RouteIdentity) (Successor, bool) {
	if view.result == nil {
		return Successor{}, false
	}
	successor, ok := view.result.Successors().Resolve(identity.causal())
	if !ok {
		return Successor{}, false
	}
	return publicSuccessor(successor), true
}

type Continuation struct{ result *continuation.Result }

func (view Continuation) CellCount(subject keyspace.Term) (int, bool) {
	if view.result == nil {
		return 0, false
	}
	return view.result.CellCount(subject)
}
func (view Continuation) CellAt(subject keyspace.Term, index int) (keyspace.Term, bool) {
	if view.result == nil {
		return 0, false
	}
	return view.result.CellAt(subject, index)
}
func (view Continuation) GuardCount(subject keyspace.Term) (int, bool) {
	if view.result == nil {
		return 0, false
	}
	return view.result.GuardCount(subject)
}
func (view Continuation) GuardAt(subject keyspace.Term, index int) (keyspace.Term, bool) {
	if view.result == nil {
		return 0, false
	}
	return view.result.GuardAt(subject, index)
}
