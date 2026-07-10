package diagnostics

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

type lifecyclePresentation struct {
	File     string
	Span     diagnostic.Span
	Message  string
	Help     string
	Evidence []diagnostic.Evidence
	Labels   []diagnostic.Label
}

func (ProofContext) Lifecycle(item judgment.Judgment) (lifecyclePresentation, bool) {
	proof := item.LifecycleProof()
	if !proof.Found {
		return lifecyclePresentation{}, false
	}
	detail := proof.Detail
	resourceName := detail.Resource
	if resourceName == "" {
		resourceName = item.Subject.Label
	}
	if resourceName == "" {
		resourceName = "resource"
	}
	report := newLifecycleResourceReport(resourceName, detail.Protocol, detail.CurrentState, lifecycleFinalStateLabel(detail))
	evidence, labels, span := lifecycleEvidence(item, report)
	return lifecyclePresentation{
		File:     lifecycleFile(item),
		Span:     span,
		Message:  report.Message(),
		Help:     report.Help(),
		Evidence: evidence,
		Labels:   labels,
	}, true
}

func lifecycleEvidence(item judgment.Judgment, report lifecycleResourceReport) ([]diagnostic.Evidence, []diagnostic.Label, diagnostic.Span) {
	var evidence []diagnostic.Evidence
	var labels []diagnostic.Label
	var primary diagnostic.Span
	for _, itemEvidence := range item.Evidence {
		span := diagnosticSpanFromJudgment(itemEvidence.Span)
		switch itemEvidence.Detail.Kind {
		case judgment.EvidenceDetailLifecycleAcquire:
			if primary == (diagnostic.Span{}) && span.Valid() {
				primary = span
			}
			if span.Valid() {
				evidence = append(evidence, diagnostic.Evidence{
					Kind:    diagnostic.EvidenceAbstractFact,
					Trust:   diagnosticTrustFromJudgmentTrust(itemEvidence.Trust, diagnostic.TrustProven),
					Span:    span,
					Message: report.AcquireEvidence(lifecycleDetailResourceName(report.resourceName, itemEvidence.Detail), itemEvidence.Detail.ToState),
				})
				labels = append(labels, sourceLabel(span, labelLifecycleAcquire))
			}
		case judgment.EvidenceDetailLifecycleTransition:
			if span.Valid() {
				evidence = append(evidence, diagnostic.Evidence{
					Kind:    diagnostic.EvidenceAbstractFact,
					Trust:   diagnosticTrustFromJudgmentTrust(itemEvidence.Trust, diagnostic.TrustProven),
					Span:    span,
					Message: report.TransitionEvidence(lifecycleDetailResourceName(report.resourceName, itemEvidence.Detail), itemEvidence.Detail.FromState, itemEvidence.Detail.ToState),
				})
				labels = append(labels, sourceLabel(span, labelLifecycleTransition))
			}
		case judgment.EvidenceDetailLifecycleEscape:
			if span.Valid() {
				evidence = append(evidence, diagnostic.Evidence{
					Kind:    diagnostic.EvidenceAbstractFact,
					Trust:   diagnosticTrustFromJudgmentTrust(itemEvidence.Trust, diagnostic.TrustProven),
					Span:    span,
					Message: report.EscapeEvidence(lifecycleDetailResourceName(report.resourceName, itemEvidence.Detail)),
				})
				labels = append(labels, sourceLabel(span, labelLifecycleEscape))
			}
		case judgment.EvidenceDetailLifecycleMissingProof:
			evidence = append(evidence, diagnostic.Evidence{
				Kind:    diagnostic.EvidenceMissingProof,
				Trust:   diagnosticTrustFromJudgmentTrust(itemEvidence.Trust, diagnostic.TrustRefuted),
				Message: report.ExitObligationEvidence(),
			})
		}
	}
	return evidence, labels, primary
}

func lifecycleDetailResourceName(fallback string, detail judgment.EvidenceDetail) string {
	if strings.TrimSpace(detail.Resource) != "" {
		return detail.Resource
	}
	return fallback
}

func lifecycleFinalStateLabel(detail judgment.EvidenceDetail) string {
	if detail.FinalState == "" {
		return detail.FinalState
	}
	states := strings.Split(detail.FinalState, "\x1f")
	parts := make([]string, 0, len(states))
	for _, state := range states {
		parts = append(parts, codeName(state))
	}
	return strings.Join(parts, " or ")
}

func lifecycleFile(item judgment.Judgment) string {
	for _, evidence := range item.Evidence {
		if file := evidence.Span.DisplayFile(); file != "" {
			return file
		}
	}
	return ""
}
