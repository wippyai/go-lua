package diagnostics

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
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
	envs := producerContext(p).guardEnvironments(result)
	trace := newLifecycleFactTrace(result, graph, envs, p.flow)
	var out []diagnostic.Diagnostic
	for _, obligation := range obligations {
		if obligation.Resource.ID == "" || obligation.Resource.Protocol == "" || obligation.Obligation.Empty() {
			continue
		}
		out = append(out, newLifecycleObligationDiagnostic(obligation, trace))
	}
	return out
}

func newLifecycleObligationDiagnostic(obligation typestate.OpenObligation, trace lifecycleFactTrace) diagnostic.Diagnostic {
	resource := obligation.Resource
	acquires := trace.Acquires(resource)
	resourceName := resource.ID.String()
	if len(acquires) != 0 && !acquires[0].target.IsEmpty() {
		resourceName = acquires[0].target.String()
	}
	protocol := string(resource.Protocol)
	current := string(obligation.Current)
	final := lifecycleObligationFinalName(obligation.Obligation)
	report := newLifecycleResourceReport(resourceName, protocol, current, final)
	span := diagnostic.Span{}
	for _, acquire := range acquires {
		if acquire.span.Valid() {
			span = acquire.span
			break
		}
	}
	transitions := trace.Transitions(resource)
	escapes := trace.Escapes(resource)
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
			Message: report.AcquireEvidence(lifecycleSiteResourceName(resourceName, acquire), string(acquire.to)),
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
			Message: report.TransitionEvidence(lifecycleSiteResourceName(resourceName, transition), string(transition.from), string(transition.to)),
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
			Message: report.EscapeEvidence(lifecycleSiteResourceName(resourceName, escape)),
		})
		labels = append(labels, sourceLabel(escape.span, labelLifecycleEscape))
	}
	evidence = append(evidence, diagnostic.Evidence{
		Kind:    diagnostic.EvidenceMissingProof,
		Trust:   diagnostic.TrustRefuted,
		Message: report.ExitObligationEvidence(),
	})
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Span:        span,
		Code:        CodeResourceUnreleased,
		Message:     report.Message(),
		Severity:    diagnostic.SeverityWarning,
		Explanation: diagnostic.NewExplanation(evidence...),
		Help:        report.Help(),
		Labels:      labels,
	})
}

func lifecycleSiteResourceName(fallback string, site lifecycleFactSite) string {
	if !site.target.IsEmpty() {
		return site.target.String()
	}
	return fallback
}

func lifecycleObligationFinalName(obligation typestate.Obligation) string {
	states := obligation.FinalStateList()
	if len(states) == 0 {
		return ""
	}
	names := make([]string, 0, len(states))
	for _, state := range states {
		names = append(names, codeName(state.String()))
	}
	return strings.Join(names, " or ")
}
