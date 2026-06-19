package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
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
		out = append(out, newLifecycleObligationDiagnostic(obligation, sites))
	}
	return out
}

type lifecycleFactSite struct {
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

func newLifecycleObligationDiagnostic(obligation typestate.OpenObligation, sites []lifecycleFactSite) diagnostic.Diagnostic {
	resourceID := pathdom.PathKey(obligation.Resource.ID)
	acquire, hasAcquire := lifecycleAcquireSite(resourceID, obligation.Resource.Protocol, sites)
	resourceName := obligation.Resource.ID
	if hasAcquire && !acquire.target.IsEmpty() {
		resourceName = acquire.target.String()
	}
	protocol := string(obligation.Resource.Protocol)
	current := string(obligation.Current)
	final := string(obligation.Obligation.Final)
	span := diagnostic.Span{}
	if hasAcquire {
		span = acquire.span
	}
	evidence := make([]diagnostic.Evidence, 0, 3)
	labels := make([]diagnostic.Label, 0, 2)
	if hasAcquire && acquire.span.Valid() {
		evidence = append(evidence, diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    acquire.span,
			Message: resourceAcquireEvidence(resourceName, protocol, string(acquire.to), final),
		})
		labels = append(labels, sourceLabel(acquire.span, labelLifecycleAcquire))
	}
	if transition, ok := lifecycleFinalTransitionSite(resourceID, obligation.Resource.Protocol, obligation.Obligation.Final, sites); ok && transition.span.Valid() {
		evidence = append(evidence, diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    transition.span,
			Message: resourceTransitionEvidence(resourceName, protocol, string(transition.from), string(transition.to)),
		})
		labels = append(labels, sourceLabel(transition.span, labelLifecycleTransition))
	}
	if escape, ok := lifecycleEscapeSite(resourceID, obligation.Resource.Protocol, sites); ok && escape.span.Valid() {
		evidence = append(evidence, diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    escape.span,
			Message: resourceEscapeEvidence(resourceName, protocol),
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

func lifecycleAcquireSite(resourceID pathdom.PathKey, protocol typestate.Protocol, sites []lifecycleFactSite) (lifecycleFactSite, bool) {
	var out lifecycleFactSite
	ok := false
	for _, site := range sites {
		if site.kind != callboundary.LifecycleAcquire || site.targetKey != resourceID || site.protocol != protocol {
			continue
		}
		out = site
		ok = true
	}
	return out, ok
}

func lifecycleEscapeSite(resourceID pathdom.PathKey, protocol typestate.Protocol, sites []lifecycleFactSite) (lifecycleFactSite, bool) {
	var out lifecycleFactSite
	ok := false
	for _, site := range sites {
		if site.kind != callboundary.LifecycleEscape || site.targetKey != resourceID || site.protocol != protocol {
			continue
		}
		out = site
		ok = true
	}
	return out, ok
}

func lifecycleFinalTransitionSite(resourceID pathdom.PathKey, protocol typestate.Protocol, final typestate.State, sites []lifecycleFactSite) (lifecycleFactSite, bool) {
	var out lifecycleFactSite
	ok := false
	for _, site := range sites {
		if site.kind != callboundary.LifecycleTransition || site.targetKey != resourceID || site.protocol != protocol || site.to != final {
			continue
		}
		out = site
		ok = true
	}
	return out, ok
}
