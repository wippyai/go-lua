package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func renderCallArityJudgmentWithPolicy(item judgment.Judgment, policy judgment.Policy, mode judgment.StrictnessMode) (diagnostic.Diagnostic, bool) {
	if item.Code != judgment.CodeCallArity || item.Subject.Kind != judgment.SubjectCallExpression || len(item.Spans) == 0 {
		return diagnostic.Diagnostic{}, false
	}
	severity, ok := diagnosticSeverityForJudgment(item, policy, mode)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	span := diagnosticSpanFromJudgment(item.Spans[0])
	presentation, ok := diagnosticProofContext().DirectCallArity(item, span)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	return diagnostic.New(diagnostic.DiagnosticSpec{
		File:        item.Spans[0].File,
		Span:        span,
		Code:        presentation.Code,
		Severity:    severity,
		Message:     presentation.Message,
		Help:        presentation.Help,
		Labels:      presentation.Labels,
		Explanation: diagnostic.NewExplanation(presentation.Evidence...),
	}), true
}
