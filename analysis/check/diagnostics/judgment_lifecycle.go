package diagnostics

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/obligation/pass"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func produceLifecycleJudgmentDiagnosticsWithPolicy(result *body.Result, sourceFile string, policy judgment.Policy, mode judgment.StrictnessMode) []diagnostic.Diagnostic {
	query := newDiagnosticQuery(result)
	items := pass.New(pass.LifecycleObligations{}).Run(pass.Context{
		FunctionKey: sourceFile,
		SourceFile:  sourceFile,
		Reader:      query.reader,
	})
	return renderJudgmentDiagnostics(items, policy, mode)
}

func renderLifecycleJudgmentWithPolicy(item judgment.Judgment, policy judgment.Policy, mode judgment.StrictnessMode) (diagnostic.Diagnostic, bool) {
	if item.Code != judgment.CodeLifecycle || item.Subject.Kind != judgment.SubjectPath {
		return diagnostic.Diagnostic{}, false
	}
	severity, ok := diagnosticSeverityForJudgment(item, policy, mode)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	detail, ok := lifecycleMissingProofDetail(item)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	resourceName := detail.Resource
	if resourceName == "" {
		resourceName = item.Subject.Label
	}
	if resourceName == "" {
		resourceName = "resource"
	}
	report := newLifecycleResourceReport(resourceName, detail.Protocol, detail.CurrentState, lifecycleFinalStateLabel(detail))
	evidence, labels, span := lifecycleJudgmentEvidence(item, report)
	return diagnostic.New(diagnostic.DiagnosticSpec{
		File:        lifecycleJudgmentFile(item),
		Span:        span,
		Code:        CodeResourceUnreleased,
		Message:     report.Message(),
		Severity:    severity,
		Explanation: diagnostic.NewExplanation(evidence...),
		Help:        report.Help(),
		Labels:      labels,
	}), true
}

func lifecycleMissingProofDetail(item judgment.Judgment) (judgment.EvidenceDetail, bool) {
	for _, evidence := range item.Evidence {
		if evidence.Kind == judgment.EvidenceMissingProof && evidence.Detail.Kind == judgment.EvidenceDetailLifecycleMissingProof {
			return evidence.Detail, true
		}
	}
	return judgment.EvidenceDetail{}, false
}

func lifecycleJudgmentEvidence(item judgment.Judgment, report lifecycleResourceReport) ([]diagnostic.Evidence, []diagnostic.Label, diagnostic.Span) {
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

func lifecycleJudgmentFile(item judgment.Judgment) string {
	for _, evidence := range item.Evidence {
		if evidence.Span.File != "" {
			return evidence.Span.File
		}
	}
	return ""
}
