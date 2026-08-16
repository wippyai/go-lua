package program

import (
	"github.com/wippyai/go-lua/program/flow"
	"github.com/wippyai/go-lua/program/internal/canonical"
	"github.com/wippyai/go-lua/program/keyspace"
)

// StructuralRoutes is a zero-copy visitor over Flow's sealed union route
// index. It contains local edges and all closed call-boundary outcomes in the
// owner-issued order; it does not retain a route table or endpoint terms.
type StructuralRoutes struct{ input TransformerInput }

// Routes returns the exact Program structural route visitor.
func (input TransformerInput) Routes() StructuralRoutes { return StructuralRoutes{input: input} }

// StructuralRoutes returns the same visitor under its explicit semantic name.
func (input TransformerInput) StructuralRoutes() StructuralRoutes {
	return StructuralRoutes{input: input}
}

func (routes StructuralRoutes) Count() int {
	if !routes.input.Available() {
		return 0
	}
	return routes.input.owner.Flow().Causal().Successors().TotalCount()
}

func (routes StructuralRoutes) At(index int) (StructuralRoute, bool) {
	if !routes.input.Available() || index < 0 {
		return StructuralRoute{}, false
	}
	finalRoute, ok := routes.input.owner.Flow().Causal().Successors().FinalAt(index)
	if !ok {
		return StructuralRoute{}, false
	}
	return newStructuralRoute(routes.input, finalRoute)
}

// StructuralRoute is one exact owner-fenced local or call-boundary route.
// Endpoints and optional guard/Mu decisions are opaque Sites; no authored
// Term or physical route index crosses this Program facade.
type StructuralRoute struct {
	input      TransformerInput
	final      flow.FinalRoute
	from       flow.Site
	to         flow.Site
	decision   flow.Site
	mu         flow.Site
	identity   flow.RouteIdentity
	recurrence flow.RouteRecurrence
	guardProof flow.RouteGuardProof
	context    keyspace.ContentID
}

// RouteGuard is the exact guarded disposition of one StructuralRoute. An
// unguarded route has no RouteGuard; callers never infer polarity from a raw
// decision coordinate.
type RouteGuard struct {
	input    TransformerInput
	proof    flow.RouteGuardProof
	decision flow.Site
	path     keyspace.ContentID
	truth    bool
	route    keyspace.ContentID
	identity flow.RouteIdentity
	context  keyspace.ContentID
}

func newStructuralRoute(input TransformerInput, finalRoute flow.FinalRoute) (StructuralRoute, bool) {
	if !finalRoute.Available() || !input.owner.Flow().Causal().OwnsFinalRoute(finalRoute) {
		return StructuralRoute{}, false
	}
	identity, identityOK := finalRoute.Identity()
	if !identityOK {
		return StructuralRoute{}, false
	}
	from, fromOK := finalRoute.From()
	to, toOK := finalRoute.To()
	if !fromOK || !toOK || !input.OwnsSite(from) || !input.OwnsSite(to) {
		return StructuralRoute{}, false
	}
	decision, _ := finalRoute.Decision()
	mu, _ := finalRoute.Mu()
	recurrence, recurrenceOK := finalRoute.Recurrence()
	if recurrenceOK && !input.OwnsRouteRecurrence(recurrence) {
		return StructuralRoute{}, false
	}
	guardProof, guardProofOK := finalRoute.GuardProof()
	if finalRoute.Guarded() && !guardProofOK {
		return StructuralRoute{}, false
	}
	result := StructuralRoute{input: input, final: finalRoute, from: from, to: to, decision: decision, mu: mu, identity: identity, recurrence: recurrence, guardProof: guardProof}
	digest, digestOK := finalRoute.ID()
	if !digestOK {
		return StructuralRoute{}, false
	}
	result.context = transformerRoleID("program/transformer/structural-route", input.programID, func(writer *canonical.Writer) bool {
		return writer.Bytes(digest[:]) == nil && writer.Uint(uint64(identity.Arm())) == nil
	})
	return result, result.Available()
}

func (route StructuralRoute) Available() bool {
	if !route.input.Available() || !route.final.Available() || !route.input.owner.Flow().Causal().OwnsFinalRoute(route.final) || !route.from.Available() || !route.to.Available() || !route.context.Available() {
		return false
	}
	identity, identityOK := route.final.Identity()
	from, fromOK := route.final.From()
	to, toOK := route.final.To()
	if !identityOK || !fromOK || !toOK || !identity.Equal(route.identity) || !from.Equal(route.from) || !to.Equal(route.to) || !route.input.OwnsSite(route.from) || !route.input.OwnsSite(route.to) {
		return false
	}
	finalDecision, finalDecisionOK := route.final.Decision()
	if finalDecisionOK != route.decision.Available() || finalDecisionOK && !finalDecision.Equal(route.decision) {
		return false
	}
	finalMu, finalMuOK := route.final.Mu()
	if finalMuOK != route.mu.Available() || finalMuOK && !finalMu.Equal(route.mu) {
		return false
	}
	semanticID, semanticIDOK := route.final.ID()
	if !semanticIDOK {
		return false
	}
	expectedContext := transformerRoleID("program/transformer/structural-route", route.input.programID, func(writer *canonical.Writer) bool {
		return writer.Bytes(semanticID[:]) == nil && writer.Uint(uint64(identity.Arm())) == nil
	})
	if expectedContext != route.context {
		return false
	}
	finalGuard, finalGuardOK := route.final.GuardProof()
	if finalGuardOK != route.guardProof.Available() {
		return false
	}
	if finalGuardOK && finalGuard.ContextID() != route.guardProof.ContextID() {
		return false
	}
	finalRecurrence, finalRecurrenceOK := route.final.Recurrence()
	if finalRecurrenceOK != route.recurrence.Available() {
		return false
	}
	if finalRecurrenceOK && !finalRecurrence.Equal(route.recurrence) {
		return false
	}
	return true
}

func (route StructuralRoute) ContextID() keyspace.ContentID {
	if !route.Available() {
		return keyspace.ContentID{}
	}
	return route.context
}

func (route StructuralRoute) Equal(other StructuralRoute) bool {
	left, right := route.ContextID(), other.ContextID()
	return left.Available() && left == right
}

func (route StructuralRoute) From() (flow.Site, bool) {
	if !route.Available() {
		return flow.Site{}, false
	}
	return route.from, true
}

func (route StructuralRoute) To() (flow.Site, bool) {
	if !route.Available() {
		return flow.Site{}, false
	}
	return route.to, true
}

// FromPoint is the exact parent-issued phase vertex for this route source.
// It is distinct from FromPath, which identifies the optional Causal Site
// attachment rather than the SourceControl/WTO phase vertex.
func (route StructuralRoute) FromPoint() (flow.WTOPoint, bool) {
	if !route.Available() {
		return flow.WTOPoint{}, false
	}
	return route.final.FromPoint()
}

// ToPoint is the exact parent-issued phase vertex for this route target.
// It is distinct from ToPath, which identifies the optional Causal Site
// attachment rather than the SourceControl/WTO phase vertex.
func (route StructuralRoute) ToPoint() (flow.WTOPoint, bool) {
	if !route.Available() {
		return flow.WTOPoint{}, false
	}
	return route.final.ToPoint()
}

func (route StructuralRoute) FromPath() (keyspace.ContentID, bool) {
	if !route.Available() {
		return keyspace.ContentID{}, false
	}
	return route.final.FromPath()
}

func (route StructuralRoute) ToPath() (keyspace.ContentID, bool) {
	if !route.Available() {
		return keyspace.ContentID{}, false
	}
	return route.final.ToPath()
}

func (route StructuralRoute) OwnsSite(site flow.Site) bool {
	return route.Available() && route.input.owner.Flow().Causal().OwnsRouteSite(route.final, site)
}

func (route StructuralRoute) Decision() (flow.Site, bool) {
	if !route.Available() || !route.decision.Available() {
		return flow.Site{}, false
	}
	return route.decision, true
}

func (route StructuralRoute) Guard() (RouteGuard, bool) {
	if !route.Available() || !route.guardProof.Available() || !route.input.OwnsRouteGuardProof(route.guardProof) {
		return RouteGuard{}, false
	}
	truth, truthOK := route.guardProof.Truth()
	if !truthOK {
		return RouteGuard{}, false
	}
	digest, digestOK := route.final.ID()
	decisionPath, decisionPathOK := route.guardProof.DecisionPathID()
	if !digestOK || !decisionPathOK {
		return RouteGuard{}, false
	}
	guard := RouteGuard{input: route.input, proof: route.guardProof, decision: route.decision, path: decisionPath, truth: truth, route: digest, identity: route.identity, context: route.guardProof.ContextID()}
	return guard, guard.Available()
}

func (guard RouteGuard) Available() bool {
	if !guard.input.Available() || !guard.proof.Available() || !guard.input.OwnsRouteGuardProof(guard.proof) || !guard.route.Available() || !guard.context.Available() {
		return false
	}
	proofIdentity, identityOK := guard.proof.RouteIdentity()
	proofTruth, truthOK := guard.proof.Truth()
	proofRouteID, proofRouteIDOK := guard.proof.RouteID()
	decisionPath, decisionPathOK := guard.proof.DecisionPathID()
	if !identityOK || !proofIdentity.Equal(guard.identity) || !proofRouteIDOK || proofRouteID != guard.route || guard.context != guard.proof.ContextID() || !truthOK || proofTruth != guard.truth || !decisionPathOK || decisionPath != guard.path {
		return false
	}
	if guard.decision.Available() && guard.input.OwnsSite(guard.decision) {
		return true
	}
	return true
}

func (guard RouteGuard) Decision() (flow.Site, bool) {
	if !guard.Available() || !guard.decision.Available() || !guard.input.OwnsSite(guard.decision) {
		return flow.Site{}, false
	}
	return guard.decision, true
}

func (guard RouteGuard) Truth() (bool, bool) {
	if !guard.Available() {
		return false, false
	}
	return guard.proof.Truth()
}

// DecisionPathID is the portable semantic identity of the guarded control
// decision. The decision need not itself be a Causal endpoint Site.
func (guard RouteGuard) DecisionPathID() (keyspace.ContentID, bool) {
	return guard.path, guard.Available() && guard.path.Available()
}

// ConditionValueSpanID returns the exact Program-owned semantic Span of a
// Branch guard's value. Other guarded decision families have no condition
// value receipt. The authored Branch term never escapes this facade; reusable
// transformer compilation can join the scalar Span to computation
// occurrences without reopening Source or Flow.
func (guard RouteGuard) ConditionValueSpanID() (keyspace.ContentID, bool) {
	if !guard.Available() || keyspace.TermFamily(guard.identity.Decision()) != keyspace.FamilyBranch {
		return keyspace.ContentID{}, false
	}
	_, condition, _, _, branchOK := guard.input.owner.Flow().Authored().Control().Branches().Get(guard.identity.Decision())
	span, spanOK := guard.input.Span(condition)
	if !branchOK || !spanOK || !guard.input.OwnsSpan(span) {
		return keyspace.ContentID{}, false
	}
	id := span.ContextID()
	return id, id.Available()
}

func (guard RouteGuard) ContextID() keyspace.ContentID {
	if !guard.Available() {
		return keyspace.ContentID{}
	}
	return guard.context
}

func (input TransformerInput) OwnsRouteGuard(guard RouteGuard) bool {
	return input.Available() && guard.input == input && guard.Available() && input.OwnsRouteGuardProof(guard.proof)
}

func (input TransformerInput) OwnsRouteGuardProof(proof flow.RouteGuardProof) bool {
	return input.Available() && proof.Available() && input.owner.Flow().Causal().OwnsRouteGuardProof(proof)
}

// OwnsRouteRecurrence authenticates the exact Flow-issued recurrence proof
// for this Program's sealed Causal owner.
func (input TransformerInput) OwnsRouteRecurrence(proof flow.RouteRecurrence) bool {
	return input.Available() && proof.Available() && input.owner.Flow().Causal().OwnsRouteRecurrence(proof)
}

// Recurrence returns the parent-issued route-local recurrence/component proof
// when this route is cyclic. Acyclic routes have no recurrence proof.
func (route StructuralRoute) Recurrence() (flow.RouteRecurrence, bool) {
	if !route.Available() || !route.recurrence.Available() || !route.input.OwnsRouteRecurrence(route.recurrence) {
		return flow.RouteRecurrence{}, false
	}
	routeID, routeIDOK := route.final.ID()
	proofID, proofIDOK := route.recurrence.RouteID()
	if !routeIDOK || !proofIDOK || routeID != proofID {
		return flow.RouteRecurrence{}, false
	}
	return route.recurrence, true
}

func (route StructuralRoute) Mu() (flow.Site, bool) {
	if !route.Available() || !route.mu.Available() {
		return flow.Site{}, false
	}
	return route.mu, true
}

func (route StructuralRoute) HasMu() bool {
	return route.Available() && route.final.HasMu()
}

func (route StructuralRoute) MuPathID() (keyspace.ContentID, bool) {
	if !route.Available() {
		return keyspace.ContentID{}, false
	}
	return route.final.MuPathID()
}

func (route StructuralRoute) Kind() (flow.BoundaryArmKind, bool) {
	if !route.Available() {
		return 0, false
	}
	return route.final.Arm()
}

func (route StructuralRoute) RouteDigest() (keyspace.ContentID, bool) {
	return route.final.ID()
}

func (route StructuralRoute) RouteID() (keyspace.ContentID, bool) {
	return route.final.ID()
}

func (route StructuralRoute) ResetCount() int {
	proof, proofOK := route.Recurrence()
	if !route.Available() || !proofOK {
		return 0
	}
	count, ok := proof.ResetCount()
	if !ok {
		return 0
	}
	return count
}

func (route StructuralRoute) ResetDigest() (keyspace.ContentID, bool) {
	proof, proofOK := route.Recurrence()
	if !route.Available() || !proofOK {
		return keyspace.ContentID{}, false
	}
	return proof.ResetDigest()
}

// HasResetWitness distinguishes a real recurrence reset witness from an
// absent witness. Its associated half-open reset range may validly be empty.
func (route StructuralRoute) HasResetWitness() bool {
	return route.Available() && route.final.HasResetWitness()
}

func (route StructuralRoute) ResetAt(index int) (flow.Site, bool) {
	if !route.Available() || index < 0 {
		return flow.Site{}, false
	}
	return route.final.ResetAt(index)
}

// ResetPathAt returns the exact semantic reset-member receipt issued by
// Flow. It is stable across replay and does not reconstruct a Site/term.
func (route StructuralRoute) ResetPathAt(index int) (keyspace.ContentID, bool) {
	if !route.Available() || index < 0 {
		return keyspace.ContentID{}, false
	}
	return route.final.ResetPathAt(index)
}

func (route StructuralRoute) ResetContains(decision flow.Site) bool {
	if !route.Available() || !decision.Available() || !route.input.OwnsSite(decision) {
		return false
	}
	return route.final.ResetContains(decision)
}

// Component returns the exact recurrence component head when this route is
// cyclic. Acyclic and cross-component routes intentionally return no proof.
func (route StructuralRoute) Component() (flow.Site, bool) {
	if !route.Available() {
		return flow.Site{}, false
	}
	return route.final.Component()
}

// ResolveStructuralRoute reissues a route through Flow's sealed semantic
// inverse. No Program-side route scan or reindex table is retained.
func (input TransformerInput) ResolveStructuralRoute(identity flow.RouteIdentity) (StructuralRoute, bool) {
	if !input.Available() || !identity.Available() {
		return StructuralRoute{}, false
	}
	finalRoute, ok := input.owner.Flow().Causal().Successors().ResolveFinal(identity)
	if !ok {
		return StructuralRoute{}, false
	}
	return newStructuralRoute(input, finalRoute)
}

func (input TransformerInput) OwnsStructuralRoute(route StructuralRoute) bool {
	return input.Available() && route.input == input && route.Available()
}
