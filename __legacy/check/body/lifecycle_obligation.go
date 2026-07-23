package body

import (
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/dominance"
)

// LifecycleSiteKind classifies a typestate lifecycle fact site.
type LifecycleSiteKind uint8

const (
	LifecycleSiteAcquire LifecycleSiteKind = iota + 1
	LifecycleSiteTransition
	LifecycleSiteEscape
)

// LifecycleSite records one reachable lifecycle fact that contributes evidence
// to an open typestate obligation at function exit.
type LifecycleSite struct {
	Point       cfg.Point
	Kind        LifecycleSiteKind
	Resource    string
	Protocol    string
	From        string
	To          string
	TargetLabel string
	Span        SourceSpan
}

// LifecycleObligationProof is a body-owned proof for a resource whose
// typestate obligation remains open at function exit.
type LifecycleObligationProof struct {
	Point    cfg.Point
	Resource string
	Protocol string
	Current  string
	Finals   []string
	Sites    []LifecycleSite
}

type lifecycleTrace struct {
	sites []lifecycleTraceSite
	graph cfg.Graph
	reach *cfg.Reachability
	idom  map[cfg.Point]cfg.Point
}

type lifecycleTraceSite struct {
	point cfg.Point
	site  LifecycleSite
}

// LifecycleSites projects every reachable, solved lifecycle fact site. It is
// intentionally a read-only projection: callers use it to correlate existing
// typestate facts with source, never to derive a lifecycle state.
func (r *Result) LifecycleSites() []LifecycleSite {
	if r == nil || r.Graph() == nil {
		return nil
	}
	return lifecycleTraceSites(r.collectLifecycleTraceSites(r.Graph()))
}

// LifecycleObligationProofs returns open typestate obligations at function
// exit with reachable lifecycle fact sites attached as proof evidence.
func (r *Result) LifecycleObligationProofs() []LifecycleObligationProof {
	if r == nil || r.Graph() == nil {
		return nil
	}
	// The synthetic CFG exit only receives fall-through control flow.  An
	// explicit return is also a local-ownership terminal, though, and its
	// resource obligations must participate in the same exit judgment.  In
	// particular, a later close on the fall-through path cannot discharge an
	// obligation on an earlier-return path.
	var terminals []state.State
	if exit, ok := r.ExitState(); ok {
		terminals = append(terminals, exit)
	}
	for point := range r.returnFacts() {
		if !r.PointNormallyReachable(point) {
			continue
		}
		atReturn, ok := r.StateAt(point)
		if !ok {
			continue
		}
		terminals = append(terminals, atReturn)
	}
	if len(terminals) == 0 {
		return nil
	}
	domain, err := state.TryDomainWithOptionalLanes(r.registry, r.stateLanes)
	if err != nil {
		domain = state.Domain(r.registry)
	}
	joined := domain.Bottom()
	for _, terminal := range terminals {
		joined = domain.Join(joined, terminal)
	}
	obligations := make(map[typestate.Resource]typestate.OpenObligation)
	for _, obligation := range joined.OpenTypestateObligations() {
		// Joining all ownership terminals enforces conservation: a final state
		// on one terminal cannot erase an open state on another.
		obligations[obligation.Resource] = obligation
	}
	if len(obligations) == 0 {
		return nil
	}
	trace := r.newLifecycleTrace()
	resources := make([]typestate.Resource, 0, len(obligations))
	for resource := range obligations {
		resources = append(resources, resource)
	}
	sort.Slice(resources, func(i, j int) bool {
		if resources[i].ID != resources[j].ID {
			return resources[i].ID < resources[j].ID
		}
		return resources[i].Protocol < resources[j].Protocol
	})
	out := make([]LifecycleObligationProof, 0, len(resources))
	for _, resource := range resources {
		obligation := obligations[resource]
		if obligation.Resource.ID == "" || obligation.Resource.Protocol == "" || obligation.Obligation.Empty() {
			continue
		}
		out = append(out, r.lifecycleObligationProof(obligation, trace))
	}
	return out
}

func (r *Result) lifecycleObligationProof(obligation typestate.OpenObligation, trace lifecycleTrace) LifecycleObligationProof {
	resource := obligation.Resource
	return LifecycleObligationProof{
		Point:    r.Graph().Exit(),
		Resource: resource.ID.String(),
		Protocol: string(resource.Protocol),
		Current:  string(obligation.Current),
		Finals:   lifecycleFinalStateNames(obligation.Obligation),
		Sites:    trace.sitesForResource(resource),
	}
}

func lifecycleFinalStateNames(obligation typestate.Obligation) []string {
	states := obligation.FinalStateList()
	if len(states) == 0 {
		return nil
	}
	out := make([]string, 0, len(states))
	for _, state := range states {
		out = append(out, state.String())
	}
	return out
}

func (r *Result) newLifecycleTrace() lifecycleTrace {
	graph := r.Graph()
	return lifecycleTrace{
		sites: r.collectLifecycleTraceSites(graph),
		graph: graph,
		reach: cfg.NewReachability(graph),
		idom:  dominance.ComputeImmediateDominatorInfo(graph).Map(),
	}
}

func (t lifecycleTrace) sitesForResource(resource typestate.Resource) []LifecycleSite {
	acquires := t.latestSites(resource, LifecycleSiteAcquire)
	transitions := t.latestSites(resource, LifecycleSiteTransition)
	escapes := t.latestSites(resource, LifecycleSiteEscape)
	out := make([]LifecycleSite, 0, len(acquires)+len(transitions)+len(escapes))
	out = append(out, acquires...)
	out = append(out, transitions...)
	out = append(out, escapes...)
	return out
}

func (t lifecycleTrace) latestSites(resource typestate.Resource, kind LifecycleSiteKind) []LifecycleSite {
	var selected []lifecycleTraceSite
	for _, site := range t.sites {
		if site.site.Kind == kind && site.site.Resource == resource.ID.String() && site.site.Protocol == string(resource.Protocol) {
			selected = append(selected, site)
		}
	}
	if len(selected) <= 1 || t.graph == nil {
		return lifecycleTraceSites(selected)
	}
	exit := t.graph.Exit()
	out := make([]lifecycleTraceSite, 0, len(selected))
	for i, site := range selected {
		stale := false
		for j, other := range selected {
			if i == j || site.point == other.point {
				continue
			}
			if dominance.Dominates(t.idom, other.point, exit) && t.reach.CanReach(site.point, other.point) {
				stale = true
				break
			}
		}
		if !stale {
			out = append(out, site)
		}
	}
	if len(out) == 0 {
		return lifecycleTraceSites(selected)
	}
	return lifecycleTraceSites(out)
}

func lifecycleTraceSites(sites []lifecycleTraceSite) []LifecycleSite {
	if len(sites) == 0 {
		return nil
	}
	out := make([]LifecycleSite, 0, len(sites))
	for _, site := range sites {
		out = append(out, site.site)
	}
	return out
}

func (r *Result) collectLifecycleTraceSites(graph cfg.Graph) []lifecycleTraceSite {
	if graph == nil {
		return nil
	}
	var out []lifecycleTraceSite
	for _, point := range graph.RPO() {
		if !r.PointNormallyReachable(point) {
			continue
		}
		outcome, ok := r.CallOutcomeAt(point)
		if !ok || len(outcome.NormalReturnFacts.LifecycleFacts) == 0 {
			continue
		}
		if !r.callSiteExists(point) {
			continue
		}
		span := r.callSpanAt(point)
		for _, fact := range outcome.NormalReturnFacts.LifecycleFacts {
			if fact.Kind == callboundary.LifecycleNone || fact.Protocol == "" {
				continue
			}
			target, ok := r.lifecycleFactTargetAt(point, fact)
			if !ok || target.IsEmpty() {
				continue
			}
			resource, ok := r.lifecycleFactResourceAt(point, fact, target)
			if !ok {
				continue
			}
			kind, ok := lifecycleSiteKind(fact.Kind)
			if !ok {
				continue
			}
			out = append(out, lifecycleTraceSite{
				point: point,
				site: LifecycleSite{
					Point:       point,
					Kind:        kind,
					Resource:    resource.ID.String(),
					Protocol:    string(resource.Protocol),
					From:        string(fact.From),
					To:          string(fact.To),
					TargetLabel: r.DisplayPath(target),
					Span:        span,
				},
			})
		}
	}
	// Return-slot lifecycle facts are applied while the assignment consuming the
	// call result is materialized, not necessarily at the call node itself.
	// Recover those acquisition sites from the result source so a returned
	// resource retains its source location and declared obligation evidence.
	for _, point := range graph.RPO() {
		assignment, ok := r.RootAssignment(point)
		if !ok || !r.PointNormallyReachable(point) {
			continue
		}
		source := assignment.Source()
		if source.Kind != factflow.ValueSourceCall || !source.HasCallPoint || source.ResultIndex < 0 {
			continue
		}
		callPoint := source.CallPoint
		outcome, ok := r.CallOutcomeAt(callPoint)
		if !ok || len(outcome.NormalReturnFacts.LifecycleFacts) == 0 {
			continue
		}
		for _, fact := range outcome.NormalReturnFacts.LifecycleFacts {
			index, returned := callboundary.ReturnSlotIndex(fact.Target)
			if !returned || index != source.ResultIndex || fact.Kind == callboundary.LifecycleNone || fact.Protocol == "" {
				continue
			}
			resource, ok := r.TypestateResourceAtBoundary(point, assignment.TargetPathRef(), fact.Protocol)
			if !ok {
				continue
			}
			kind, ok := lifecycleSiteKind(fact.Kind)
			if !ok {
				continue
			}
			out = append(out, lifecycleTraceSite{
				point: callPoint,
				site: LifecycleSite{
					Point:       callPoint,
					Kind:        kind,
					Resource:    resource.ID.String(),
					Protocol:    string(resource.Protocol),
					From:        string(fact.From),
					To:          string(fact.To),
					TargetLabel: r.DisplayPath(assignment.TargetPathRef()),
					Span:        r.callSpanAt(callPoint),
				},
			})
		}
	}
	return out
}

func (r *Result) lifecycleFactTargetAt(point cfg.Point, fact callboundary.LifecycleFact) (pathdom.Path, bool) {
	if index, returned := callboundary.ReturnSlotIndex(fact.Target); returned {
		site, ok := r.CallSiteView(point)
		if !ok {
			return pathdom.Path{}, false
		}
		var target pathdom.Path
		found := false
		site.ForEachResultTarget(func(result factflow.CallResultTargetView) bool {
			if result.ResultIndex() != index || result.TargetPathEmpty() {
				return true
			}
			target = result.TargetPathRef().AppendSegments(fact.Target.Segments)
			found = true
			return false
		})
		return target, found
	}
	return fact.Target.Substitute(r.callGuardCallBindingsAt(point))
}

func (r *Result) lifecycleFactResourceAt(point cfg.Point, fact callboundary.LifecycleFact, target pathdom.Path) (typestate.Resource, bool) {
	if _, returned := callboundary.ReturnSlotIndex(fact.Target); returned {
		// A return-slot acquisition is materialized by this call itself, so its
		// canonical identity exists only at the post-call boundary. Parameter
		// transitions retain the call-entry lookup used by the applier.
		return r.TypestateResourceAtBoundary(point, target, fact.Protocol)
	}
	return r.TypestateResourceAtCallEntry(point, target, fact.Protocol)
}

func lifecycleSiteKind(kind callboundary.LifecycleKind) (LifecycleSiteKind, bool) {
	switch kind {
	case callboundary.LifecycleAcquire:
		return LifecycleSiteAcquire, true
	case callboundary.LifecycleTransition:
		return LifecycleSiteTransition, true
	case callboundary.LifecycleEscape:
		return LifecycleSiteEscape, true
	default:
		return 0, false
	}
}
