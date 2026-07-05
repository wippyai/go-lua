package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func renderReturnJudgmentWithPolicy(item judgment.Judgment, policy judgment.Policy, mode judgment.StrictnessMode) (diagnostic.Diagnostic, bool) {
	if item.Code != judgment.CodeReturn || item.Subject.Kind != judgment.SubjectReturnValue || len(item.Spans) == 0 {
		return diagnostic.Diagnostic{}, false
	}
	severity, ok := diagnosticSeverityForJudgment(item, policy, mode)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	got := item.Actual.ProjectedType
	want := item.Expected.Type
	if got == nil || want == nil {
		return diagnostic.Diagnostic{}, false
	}
	span := diagnosticSpanFromJudgment(item.Spans[0])
	declSpan := diagnosticEvidenceSpanOrPrimary(item, judgment.EvidenceUserAssertion)
	label := item.Subject.Label
	if label == "" {
		label = "returned value"
	}
	sourceName := item.Actual.Label
	presentation := diagnosticProofContext().Return(item, label, sourceName, got, want, span)
	evidence := []diagnostic.Evidence{
		{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceAbstractFact, diagnostic.TrustProven),
			Span:    diagnosticEvidenceSpanOrPrimary(item, judgment.EvidenceAbstractFact),
			Message: presentation.SourceEvidence,
		},
		{
			Kind:    diagnostic.EvidenceUserAssertion,
			Trust:   diagnostic.TrustClaimed,
			Span:    declSpan,
			Message: returnDeclaredTypeEvidence(label, want),
		},
	}
	evidence = append(evidence, presentation.Evidence...)
	return diagnostic.New(diagnostic.DiagnosticSpec{
		File:        item.Spans[0].File,
		Span:        span,
		Code:        CodeReturnContractType,
		Severity:    severity,
		Message:     presentation.Message,
		Help:        presentation.Help,
		Explanation: diagnostic.NewExplanation(evidence...),
		Labels: []diagnostic.Label{
			sourceLabel(span, labelReturnedValue),
			sourceLabel(declSpan, labelDeclaredReturn),
		},
	}), true
}
