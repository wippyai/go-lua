package readmodel

import (
	readapi "github.com/wippyai/go-lua/analysis/check/readmodel"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/dominance"
)

// ForEachLifecycleObligation visits typestate resources whose obligations remain
// open at function exit, with reachable lifecycle fact sites attached as
// renderer-independent evidence.
func (r Reader) ForEachLifecycleObligation(visit func(LifecycleObligation) bool) bool {
	if r.result == nil || visit == nil || r.result.Graph() == nil {
		return false
	}
	exit, ok := r.result.ExitState()
	if !ok {
		return false
	}
	obligations := exit.OpenTypestateObligations()
	if len(obligations) == 0 {
		return false
	}
	trace := r.newLifecycleTrace()
	visited := false
	for _, obligation := range obligations {
		if obligation.Resource.ID == "" || obligation.Resource.Protocol == "" || obligation.Obligation.Empty() {
			continue
		}
		item := r.lifecycleObligation(obligation, trace)
		visited = true
		if !visit(item) {
			return true
		}
	}
	return visited
}

func (r Reader) lifecycleObligation(obligation typestate.OpenObligation, trace lifecycleTrace) LifecycleObligation {
	resource := obligation.Resource
	sites := trace.sitesForResource(resource)
	return LifecycleObligation{
		Point:    r.result.Graph().Exit(),
		Resource: resource.ID.String(),
		Protocol: string(resource.Protocol),
		Current:  string(obligation.Current),
		Finals:   lifecycleFinalStateNames(obligation.Obligation),
		Sites:    sites,
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

type lifecycleTrace struct {
	sites []lifecycleTraceSite
	graph cfg.Graph
	reach *cfg.Reachability
	idom  map[cfg.Point]cfg.Point
}

type lifecycleTraceSite struct {
	point cfg.Point
	site  readapi.LifecycleSite
}

func (r Reader) newLifecycleTrace() lifecycleTrace {
	graph := r.result.Graph()
	return lifecycleTrace{
		sites: r.collectLifecycleTraceSites(graph),
		graph: graph,
		reach: cfg.NewReachability(graph),
		idom:  dominance.ComputeImmediateDominatorInfo(graph).Map(),
	}
}

func (t lifecycleTrace) sitesForResource(resource typestate.Resource) []readapi.LifecycleSite {
	acquires := t.latestSites(resource, readapi.LifecycleSiteAcquire)
	transitions := t.latestSites(resource, readapi.LifecycleSiteTransition)
	escapes := t.latestSites(resource, readapi.LifecycleSiteEscape)
	out := make([]readapi.LifecycleSite, 0, len(acquires)+len(transitions)+len(escapes))
	out = append(out, acquires...)
	out = append(out, transitions...)
	out = append(out, escapes...)
	return out
}

func (t lifecycleTrace) latestSites(resource typestate.Resource, kind readapi.LifecycleSiteKind) []readapi.LifecycleSite {
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

func lifecycleTraceSites(sites []lifecycleTraceSite) []readapi.LifecycleSite {
	if len(sites) == 0 {
		return nil
	}
	out := make([]readapi.LifecycleSite, 0, len(sites))
	for _, site := range sites {
		out = append(out, site.site)
	}
	return out
}

func (r Reader) collectLifecycleTraceSites(graph cfg.Graph) []lifecycleTraceSite {
	if graph == nil {
		return nil
	}
	var out []lifecycleTraceSite
	for _, point := range graph.RPO() {
		if !r.result.PointNormallyReachable(point) {
			continue
		}
		outcome, ok := r.result.CallOutcomeAt(point)
		if !ok || len(outcome.NormalReturnFacts.LifecycleFacts) == 0 {
			continue
		}
		site, ok := r.result.CallSite(point)
		if !ok {
			continue
		}
		bindings := r.callBindings(site)
		span := sourceSpanFromFactflow(site.CallSpan())
		for _, fact := range outcome.NormalReturnFacts.LifecycleFacts {
			if fact.Kind == callboundary.LifecycleNone || fact.Protocol == "" {
				continue
			}
			target, ok := fact.Target.Substitute(bindings)
			if !ok || target.IsEmpty() {
				continue
			}
			resource, ok := r.result.TypestateResourceAtCallEntry(point, target, fact.Protocol)
			if !ok {
				continue
			}
			kind, ok := lifecycleSiteKind(fact.Kind)
			if !ok {
				continue
			}
			out = append(out, lifecycleTraceSite{
				point: point,
				site: readapi.LifecycleSite{
					Point:       point,
					Kind:        kind,
					Resource:    resource.ID.String(),
					Protocol:    string(resource.Protocol),
					From:        string(fact.From),
					To:          string(fact.To),
					TargetLabel: r.displayPath(target),
					Span:        span,
				},
			})
		}
	}
	return out
}

func lifecycleSiteKind(kind callboundary.LifecycleKind) (readapi.LifecycleSiteKind, bool) {
	switch kind {
	case callboundary.LifecycleAcquire:
		return readapi.LifecycleSiteAcquire, true
	case callboundary.LifecycleTransition:
		return readapi.LifecycleSiteTransition, true
	case callboundary.LifecycleEscape:
		return readapi.LifecycleSiteEscape, true
	default:
		return 0, false
	}
}
