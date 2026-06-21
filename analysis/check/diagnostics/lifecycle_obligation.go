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

type lifecycleObligations producerContext

func (p lifecycleObligations) Produce(result *body.Result) []diagnostic.Diagnostic {
	if result == nil {
		return nil
	}
	graph := result.Graph()
	if graph == nil {
		return nil
	}
	exit, ok := result.ExitState()
	if !ok {
		return nil
	}
	obligations := exit.OpenTypestateObligations()
	if len(obligations) == 0 {
		return nil
	}
	envs := cachedGuardEnvironments(result)
	sites := lifecycleFactSites(result, graph, envs)
	var out []diagnostic.Diagnostic
	for _, obligation := range obligations {
		if obligation.Resource.ID == "" || obligation.Resource.Protocol == "" || obligation.Obligation.Final == "" {
			continue
		}
		out = append(out, newLifecycleObligationDiagnostic(obligation, sites, graph, p.flow))
	}
	return out
}

type lifecycleFactSite struct {
	point     cfg.Point
	kind      callboundary.LifecycleKind
	target    pathdom.Path
	targetKey pathdom.PathKey
	protocol  typestate.Protocol
	from      typestate.State
	to        typestate.State
	span      diagnostic.Span
}

func lifecycleFactSites(result *body.Result, graph cfg.Graph, envs map[cfg.Point]guardEnv) []lifecycleFactSite {
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
			targetKey, ok := result.TypestateResourceKeyAtBoundary(point, target)
			if !ok {
				continue
			}
			out = append(out, lifecycleFactSite{
				point:     point,
				kind:      fact.Kind,
				target:    target,
				targetKey: targetKey,
				protocol:  fact.Protocol,
				from:      fact.From,
				to:        fact.To,
				span:      span,
			})
		}
	}
	return out
}

func newLifecycleObligationDiagnostic(obligation typestate.OpenObligation, sites []lifecycleFactSite, graph cfg.Graph, flow *diagnosticFlowCache) diagnostic.Diagnostic {
	resourceID := pathdom.PathKey(obligation.Resource.ID)
	acquires := lifecycleAcquireSites(resourceID, obligation.Resource.Protocol, sites, graph, flow)
	resourceName := obligation.Resource.ID
	if len(acquires) != 0 && !acquires[0].target.IsEmpty() {
		resourceName = acquires[0].target.String()
	}
	protocol := string(obligation.Resource.Protocol)
	current := string(obligation.Current)
	final := string(obligation.Obligation.Final)
	span := diagnostic.Span{}
	for _, acquire := range acquires {
		if acquire.span.Valid() {
			span = acquire.span
			break
		}
	}
	transitions := lifecycleFinalTransitionSites(resourceID, obligation.Resource.Protocol, obligation.Obligation.Final, sites, graph, flow)
	escapes := lifecycleEscapeSites(resourceID, obligation.Resource.Protocol, sites, graph, flow)
	evidence := make([]diagnostic.Evidence, 0, len(acquires)+len(transitions)+len(escapes)+1)
	labels := make([]diagnostic.Label, 0, len(acquires)+len(transitions)+len(escapes))
	for _, acquire := range acquires {
		if !acquire.span.Valid() {
			continue
		}
		evidence = append(evidence, diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    acquire.span,
			Message: resourceAcquireEvidence(lifecycleSiteResourceName(resourceName, acquire), protocol, string(acquire.to), final),
		})
		labels = append(labels, sourceLabel(acquire.span, labelLifecycleAcquire))
	}
	for _, transition := range transitions {
		if !transition.span.Valid() {
			continue
		}
		evidence = append(evidence, diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    transition.span,
			Message: resourceTransitionEvidence(lifecycleSiteResourceName(resourceName, transition), protocol, string(transition.from), string(transition.to)),
		})
		labels = append(labels, sourceLabel(transition.span, labelLifecycleTransition))
	}
	for _, escape := range escapes {
		if !escape.span.Valid() {
			continue
		}
		evidence = append(evidence, diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    escape.span,
			Message: resourceEscapeEvidence(lifecycleSiteResourceName(resourceName, escape), protocol),
		})
		labels = append(labels, sourceLabel(escape.span, labelLifecycleEscape))
	}
	evidence = append(evidence, diagnostic.Evidence{
		Kind:    diagnostic.EvidenceMissingProof,
		Trust:   diagnostic.TrustRefuted,
		Message: resourceExitObligationEvidence(resourceName, protocol, current, final),
	})
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Span:        span,
		Code:        CodeResourceUnreleased,
		Message:     resourceUnreleasedMessage(resourceName, protocol, current, final),
		Severity:    diagnostic.SeverityWarning,
		Explanation: diagnostic.NewExplanation(evidence...),
		Help:        resourceUnreleasedHelp(resourceName, final),
		Labels:      labels,
	})
}

func lifecycleSiteResourceName(fallback string, site lifecycleFactSite) string {
	if !site.target.IsEmpty() {
		return site.target.String()
	}
	return fallback
}

func lifecycleAcquireSites(resourceID pathdom.PathKey, protocol typestate.Protocol, sites []lifecycleFactSite, graph cfg.Graph, flow *diagnosticFlowCache) []lifecycleFactSite {
	var out []lifecycleFactSite
	for _, site := range sites {
		if site.kind != callboundary.LifecycleAcquire || site.targetKey != resourceID || site.protocol != protocol {
			continue
		}
		out = append(out, site)
	}
	return lifecycleLatestSites(out, graph, flow)
}

func lifecycleEscapeSites(resourceID pathdom.PathKey, protocol typestate.Protocol, sites []lifecycleFactSite, graph cfg.Graph, flow *diagnosticFlowCache) []lifecycleFactSite {
	var out []lifecycleFactSite
	for _, site := range sites {
		if site.kind != callboundary.LifecycleEscape || site.targetKey != resourceID || site.protocol != protocol {
			continue
		}
		out = append(out, site)
	}
	return lifecycleLatestSites(out, graph, flow)
}

func lifecycleFinalTransitionSites(resourceID pathdom.PathKey, protocol typestate.Protocol, final typestate.State, sites []lifecycleFactSite, graph cfg.Graph, flow *diagnosticFlowCache) []lifecycleFactSite {
	var out []lifecycleFactSite
	for _, site := range sites {
		if site.kind != callboundary.LifecycleTransition || site.targetKey != resourceID || site.protocol != protocol || site.to != final {
			continue
		}
		out = append(out, site)
	}
	return lifecycleLatestSites(out, graph, flow)
}

func lifecycleLatestSites(sites []lifecycleFactSite, graph cfg.Graph, flow *diagnosticFlowCache) []lifecycleFactSite {
	if len(sites) <= 1 || graph == nil {
		return sites
	}
	var idom map[cfg.Point]cfg.Point
	if flow != nil && flow.graph == graph {
		idom = flow.immediateDominators()
	} else {
		idom = dominance.ComputeImmediateDominatorInfo(graph).Map()
	}
	exit := graph.Exit()
	out := make([]lifecycleFactSite, 0, len(sites))
	for i, site := range sites {
		stale := false
		for j, other := range sites {
			if i == j || site.point == other.point {
				continue
			}
			if dominance.Dominates(idom, other.point, exit) && diagnosticCanReach(flow, graph, site.point, other.point) {
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
