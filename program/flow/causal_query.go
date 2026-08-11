package flow

import (
	"github.com/wippyai/go-lua/program/flow/internal/causal"
	"github.com/wippyai/go-lua/program/flow/internal/continuation"
	"github.com/wippyai/go-lua/program/keyspace"
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
	sourceID    keyspace.ContentID
	flowID      keyspace.ContentID
	staticID    keyspace.ContentID
	moduleID    keyspace.ContentID
	from        keyspace.Term
	to          keyspace.Term
	decision    keyspace.Term
	truth       bool
	mu          keyspace.Term
	arm         BoundaryArmKind
	resetDigest keyspace.ContentID
	resetCount  uint32
	digest      keyspace.ContentID
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
	return identity.causal().Available()
}

func (identity RouteIdentity) Equal(other RouteIdentity) bool {
	return identity == other
}

func (identity RouteIdentity) Digest() keyspace.ContentID { return identity.digest }
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
func (successor Successor) ResetCount() (int, bool) { return successor.route.ResetCount() }
func (successor Successor) ResetAt(offset int) (keyspace.Term, bool) {
	return successor.route.ResetAt(offset)
}
func (successor Successor) ResetContains(decision keyspace.Term) bool {
	return successor.route.ResetContains(decision)
}

type Causal struct{ result *causal.Result }

func (view Causal) Edges() Edges           { return Edges{result: view.result} }
func (view Causal) Boundaries() Boundaries { return Boundaries{result: view.result} }
func (view Causal) Successors() Successors { return Successors{result: view.result} }
func (view Causal) Sites() Sites           { return Sites{result: view.result} }

// Site is an opaque exact-quartet-fenced Flow causal site handle for an
// existing route endpoint or sealed body-terminal Outcome coordinate.
// ContextID is stable across equivalent seal/artifact replay, but is not a
// portable identity across a mutated program.
type Site struct{ site causal.Site }

func publicSite(site causal.Site) Site { return Site{site: site} }

// Available reports whether the handle belongs to its exact sealed quartet.
func (site Site) Available() bool { return site.site.Available() }

// Equal compares two exact-quartet-fenced endpoint handles.
func (site Site) Equal(other Site) bool { return site.site.Equal(other.site) }

// ContextID returns the contextual endpoint identity; it is not mutation portable.
func (site Site) ContextID() keyspace.ContentID { return site.site.ContextID() }

// Term returns the existing causal endpoint Term represented by the site.
func (site Site) Term() (keyspace.Term, bool) { return site.site.Term() }

// Sites is the allocation-free canonical causal endpoint projection.
// Its indexes borrow the one sealed Causal owner and retain no syntax/IR copy.
type Sites struct{ result *causal.Result }

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
	site, ok := view.result.SiteAt(index)
	return publicSite(site), ok
}

// ForTerm resolves only Terms that occur as existing route endpoints or sealed
// body-terminal Outcome coordinates.
func (view Sites) ForTerm(term keyspace.Term) (Site, bool) {
	if view.result == nil {
		return Site{}, false
	}
	site, ok := view.result.SiteForTerm(term)
	return publicSite(site), ok
}

// ResolveContextID performs an exact-quartet-fenced contextual lookup.
func (view Sites) ResolveContextID(id keyspace.ContentID) (Site, bool) {
	if view.result == nil {
		return Site{}, false
	}
	site, ok := view.result.ResolveContextID(id)
	return publicSite(site), ok
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

type Successors struct{ result *causal.Result }

func (view Successors) Count(from keyspace.Term) int {
	if view.result == nil {
		return 0
	}
	return view.result.Successors().Count(from)
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
