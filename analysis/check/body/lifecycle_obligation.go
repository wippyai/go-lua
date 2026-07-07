package body

import (
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
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

func sourceSpanFromFactflow(span factflow.SourceSpan) SourceSpan {
	return SourceSpan{
		StartLine: span.StartLine,
		StartCol:  span.StartCol,
		EndLine:   span.EndLine,
		EndCol:    span.EndCol,
	}
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

// LifecycleObligationProofs returns open typestate obligations at function
// exit with reachable lifecycle fact sites attached as proof evidence.
func (r *Result) LifecycleObligationProofs() []LifecycleObligationProof {
	if r == nil || r.Graph() == nil {
		return nil
	}
	exit, ok := r.ExitState()
	if !ok {
		return nil
	}
	obligations := exit.OpenTypestateObligations()
	if len(obligations) == 0 {
		return nil
	}
	trace := r.newLifecycleTrace()
	var out []LifecycleObligationProof
	for _, obligation := range obligations {
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
		site, ok := r.CallSite(point)
		if !ok {
			continue
		}
		bindings := r.callGuardCallBindings(site)
		span := sourceSpanFromFactflow(site.CallSpan())
		for _, fact := range outcome.NormalReturnFacts.LifecycleFacts {
			if fact.Kind == callboundary.LifecycleNone || fact.Protocol == "" {
				continue
			}
			target, ok := fact.Target.Substitute(bindings)
			if !ok || target.IsEmpty() {
				continue
			}
			resource, ok := r.TypestateResourceAtCallEntry(point, target, fact.Protocol)
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
	return out
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
