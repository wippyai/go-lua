package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func renderNumericForJudgmentWithPolicy(ctx judgmentRenderContext, item judgment.Judgment, policy judgment.Policy, mode judgment.StrictnessMode) (diagnostic.Diagnostic, bool) {
	if item.Code != judgment.CodeNumericForOperand || item.Subject.Kind != judgment.SubjectExpression || len(item.Spans) == 0 {
		return diagnostic.Diagnostic{}, false
	}
	severity, ok := diagnosticSeverityForJudgment(item, policy, mode)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	code := diagnosticCodeForJudgment(item)
	span := diagnosticSpanFromJudgment(item.Spans[0])
	presentation, ok := ctx.proof.NumericFor(item, span)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	return diagnostic.New(diagnostic.DiagnosticSpec{
		File:        item.Spans[0].File,
		Span:        span,
		Code:        code,
		Severity:    severity,
		Message:     presentation.Message,
		Explanation: diagnostic.NewExplanation(presentation.Evidence...),
		Help:        presentation.Help,
		Labels:      presentation.Labels,
	}), true
}
