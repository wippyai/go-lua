package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/dominance"
	"github.com/wippyai/go-lua/compiler/ast"
)

// lifecycleFactTrace owns the diagnostic view of lifecycle evidence. Producers
// ask it for resource-specific sites; they do not scan call outcomes or
// duplicate dominance pruning policy.
type lifecycleFactTrace struct {
	sites []lifecycleFactSite
	graph cfg.Graph
	flow  *diagnosticFlowCache
}

type lifecycleFactSite struct {
	point    cfg.Point
	kind     callboundary.LifecycleKind
	target   pathdom.Path
	resource typestate.Resource
	from     typestate.State
	to       typestate.State
	span     diagnostic.Span
}

func newLifecycleFactTrace(
	result *body.Result,
	graph cfg.Graph,
	envs map[cfg.Point]guardEnv,
	flow *diagnosticFlowCache,
) lifecycleFactTrace {
	return lifecycleFactTrace{
		sites: collectLifecycleFactSites(result, graph, envs),
		graph: graph,
		flow:  flow,
	}
}

func (t lifecycleFactTrace) Acquires(resource typestate.Resource) []lifecycleFactSite {
	return t.sitesFor(resource, callboundary.LifecycleAcquire)
}

func (t lifecycleFactTrace) Transitions(resource typestate.Resource) []lifecycleFactSite {
	return t.sitesFor(resource, callboundary.LifecycleTransition)
}

func (t lifecycleFactTrace) Escapes(resource typestate.Resource) []lifecycleFactSite {
	return t.sitesFor(resource, callboundary.LifecycleEscape)
}

func (t lifecycleFactTrace) sitesFor(resource typestate.Resource, kind callboundary.LifecycleKind) []lifecycleFactSite {
	var out []lifecycleFactSite
	for _, site := range t.sites {
		if site.kind != kind || site.resource != resource {
			continue
		}
		out = append(out, site)
	}
	return t.latest(out)
}

func (t lifecycleFactTrace) latest(sites []lifecycleFactSite) []lifecycleFactSite {
	if len(sites) <= 1 || t.graph == nil {
		return sites
	}
	idom := t.immediateDominators()
	exit := t.graph.Exit()
	out := make([]lifecycleFactSite, 0, len(sites))
	for i, site := range sites {
		stale := false
		for j, other := range sites {
			if i == j || site.point == other.point {
				continue
			}
			if dominance.Dominates(idom, other.point, exit) && diagnosticCanReach(t.flow, t.graph, site.point, other.point) {
				stale = true
				break
			}
		}
		if !stale {
			out = append(out, site)
		}
	}
	if len(out) == 0 {
		return sites
	}
	return out
}

func (t lifecycleFactTrace) immediateDominators() map[cfg.Point]cfg.Point {
	if t.flow != nil && t.flow.graph == t.graph {
		return t.flow.immediateDominators()
	}
	return dominance.ComputeImmediateDominatorInfo(t.graph).Map()
}

func collectLifecycleFactSites(result *body.Result, graph cfg.Graph, envs map[cfg.Point]guardEnv) []lifecycleFactSite {
	if result == nil || graph == nil {
		return nil
	}
	var out []lifecycleFactSite
	for _, point := range graph.RPO() {
		if !guardEnvReachableAt(envs, point) {
			continue
		}
		outcome, ok := result.CallOutcomeAt(point)
		if !ok || len(outcome.NormalReturnFacts.LifecycleFacts) == 0 {
			continue
		}
		site, ok := result.CallSite(point)
		if !ok {
			continue
		}
		bindings := callGuardCallBindings(result, site)
		span := diagnostic.Span{}
		if call, ok := result.Call(point); ok && call.Call != nil {
			span = ast.SpanOf(call.Call)
		}
		for _, fact := range outcome.NormalReturnFacts.LifecycleFacts {
			if fact.Kind == callboundary.LifecycleNone || fact.Protocol == "" {
				continue
			}
			target, ok := fact.Target.Substitute(bindings)
			if !ok || target.IsEmpty() {
				continue
			}
			resource, ok := result.TypestateResourceAtBoundary(point, target, fact.Protocol)
			if !ok {
				continue
			}
			out = append(out, lifecycleFactSite{
				point:    point,
				kind:     fact.Kind,
				target:   target,
				resource: resource,
				from:     fact.From,
				to:       fact.To,
				span:     span,
			})
		}
	}
	return out
}
