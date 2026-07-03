package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/obligation/pass"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func produceNonNilAssertionJudgmentDiagnosticsWithPolicy(result *body.Result, sourceFile string, policy judgment.Policy, mode judgment.StrictnessMode) []diagnostic.Diagnostic {
	query := newDiagnosticQuery(result)
	items := pass.New(pass.NonNilAssertions{}).Run(pass.Context{
		FunctionKey: sourceFile,
		SourceFile:  sourceFile,
		Reader:      query.reader,
	})
	return renderJudgmentDiagnostics(items, policy, mode)
}

func renderNonNilAssertionJudgmentWithPolicy(item judgment.Judgment, policy judgment.Policy, mode judgment.StrictnessMode) (diagnostic.Diagnostic, bool) {
	if item.Code != judgment.CodeNonNilAssertion || item.Subject.Kind != judgment.SubjectExpression || len(item.Spans) == 0 {
		return diagnostic.Diagnostic{}, false
	}
	severity, ok := diagnosticSeverityForJudgment(item, policy, mode)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	span := diagnosticSpanFromJudgment(item.Spans[0])
	name := item.Subject.Label
	return diagnostic.New(diagnostic.DiagnosticSpec{
		File:     item.Spans[0].File,
		Span:     span,
		Code:     CodeNonNilAssertAlwaysNil,
		Severity: severity,
		Message:  nonNilAssertAlwaysNilMessage(name),
		Labels:   []diagnostic.Label{sourceLabel(span, labelValueAlwaysNil)},
		Explanation: diagnostic.NewExplanation(
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceAbstractFact, diagnostic.TrustProven),
				Span:    nonNilAssertionJudgmentEvidenceSpan(item, span),
				Message: nonNilAssertAlwaysNilEvidence(name),
			},
		),
		Help: nonNilAssertAlwaysNilHelp(name),
	}), true
}

func nonNilAssertionJudgmentEvidenceSpan(item judgment.Judgment, fallback diagnostic.Span) diagnostic.Span {
	for _, evidence := range item.Evidence {
		if evidence.Kind == judgment.EvidenceAbstractFact && evidence.Span.StartLine != 0 {
			return diagnosticSpanFromJudgment(evidence.Span)
		}
	}
	return fallback
}
