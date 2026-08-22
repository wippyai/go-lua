package activation

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
)

// MountedAdmit is the sealed activation admission request. The construction
// plane holds the assembly; this owner supplies only declaration-owned rows,
// including the execution-context edge each candidate body route runs on,
// which it reads from the Link's sealed directory.
func (rule *HotRule) MountedAdmit(mountID, reusablePointID, occurrenceID identity.ContentID, contexts executioncontext.Directory) (engine.MountedActivationAdmit, bool) {
	if rule == nil || rule.owner == nil || rule.implementation == nil || !contexts.Available() {
		return engine.MountedActivationAdmit{}, false
	}
	// Call's Algebra is the canonical owner of mounted occurrence rows. Its
	// sealed constructor validates the inverse, application key, and module
	// residence; activation consumes that row directly without an intermediate
	// projection or a second sealing lifecycle.
	algebra := rule.owner.Algebra()
	if algebra == nil {
		return engine.MountedActivationAdmit{}, false
	}
	_, key, mountedOK := algebra.MountedCallKeyForOccurrence(mountID, occurrenceID)
	applicationID, applicationIDOK := key.ApplicationID()
	application, applicationOK := identity.NewSemanticKey([32]byte(applicationID), 1)
	operandOK := mountedOK && applicationIDOK && applicationOK && application.Available()
	if !operandOK || !rule.routesValid() {
		return engine.MountedActivationAdmit{}, false
	}
	capability, capabilityOK := rule.implementation.MountedCapability()
	if !capabilityOK {
		return engine.MountedActivationAdmit{}, false
	}
	if rule.transport == nil {
		return engine.MountedActivationAdmit{}, false
	}
	ref, refOK := rule.owner.Ref(key)
	read, readOK := engine.ExactReadSurface(ref)
	if !refOK || !readOK {
		return engine.MountedActivationAdmit{}, false
	}
	bodies := algebra.Bodies()
	candidates := make([]engine.MountedActivationCandidate, 0, bodies.Count())
	for index := 0; index < bodies.Count(); index++ {
		body, bodyOK := bodies.At(index)
		moduleKey, moduleOK := body.ModuleKey()
		bodyPath, pathOK := body.BodyPath()
		item, routeOK := rule.routeAt(index)
		if !bodyOK || !moduleOK || !pathOK || !routeOK {
			return engine.MountedActivationAdmit{}, false
		}
		// A body whose module the directory does not hold is a route no
		// mount declared and refuses the occurrence. A body the directory
		// holds but connects to this trigger by no edge is a route the Link
		// declares unreachable - another actor's copy of a shared library -
		// and contributes no candidate while the occurrence keeps the routes
		// that remain.
		edges, residentOK := activationRouteEdges(contexts, mountID, moduleKey)
		if !residentOK {
			return engine.MountedActivationAdmit{}, false
		}
		for _, edge := range edges {
			candidates = append(candidates, engine.MountedActivationCandidate{
				Target: item.target, Endpoint: item.endpoint, Mount: moduleKey, Body: bodyPath,
				TransitionID: edge.ID(), FromContextID: edge.FromContextID(), ToContextID: edge.ToContextID(),
			})
		}
	}
	return engine.MountedActivationAdmit{
		Transport:   rule.transport,
		Capability:  capability,
		Mount:       mountID,
		Point:       reusablePointID,
		Occurrence:  occurrenceID,
		Application: application,
		Read:        read,
		Candidates:  candidates,
	}, true
}

// activationRouteEdges resolves the execution-context edges one candidate body
// route may run on: from a Context of the trigger's module to a Context of the
// body's module. A module can hold several Contexts, so the route is admitted
// once per activation edge the directory derives, and the directory - not the
// producer - decides which pairs are connected.
//
// The route table this walks is Call's global body table: a call value may
// name any admitted body, so a trigger in one module carries a route to a body
// in a module it never imports. The import relation therefore cannot decide
// these pairs, and the directory answers on the activation relation instead:
// two Contexts of one actor are connected in both directions, and a body in
// the trigger's own module rides the reflexive edge of that same relation.
//
// The boolean reports residence, not reachability: it is false only when the
// directory holds no Context for one of the two modules, which is a mount the
// Link never made. Two resident modules that share no actor - the two copies
// of one shared library in a same-Link deployment - are resident and produce
// no edge, because a value applied in one actor is never the value another
// actor holds.
func activationRouteEdges(contexts executioncontext.Directory, triggerModuleID, bodyModuleID identity.ContentID) ([]executioncontext.Transition, bool) {
	from, fromOK := contexts.ContextsForModule(triggerModuleID)
	to, toOK := contexts.ContextsForModule(bodyModuleID)
	if !fromOK || !toOK {
		return nil, false
	}
	edges := make([]executioncontext.Transition, 0, len(from))
	for _, source := range from {
		for _, target := range to {
			edge, ok := contexts.ActivationEdge(source.ID(), target.ID())
			if !ok || !edge.Available() {
				continue
			}
			edges = append(edges, edge)
		}
	}
	return edges, true
}
