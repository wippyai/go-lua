package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func renderRedundantConditionJudgmentWithPolicy(ctx judgmentRenderContext, item judgment.Judgment, policy judgment.Policy, mode judgment.StrictnessMode) (diagnostic.Diagnostic, bool) {
	if item.Code != judgment.CodeRedundantCondition || item.Subject.Kind != judgment.SubjectExpression || len(item.Spans) == 0 {
		return diagnostic.Diagnostic{}, false
	}
	severity, ok := diagnosticSeverityForJudgment(item, policy, mode)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	span := diagnosticSpanFromJudgment(item.Spans[0])
	presentation := ctx.proof.RedundantCondition(item, span)
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Span:        span,
		Code:        CodeRedundantCondition,
		Severity:    severity,
		Message:     presentation.Message,
		Labels:      presentation.Labels,
		Explanation: presentation.Explanation,
		Help:        presentation.Help,
	}), true
}
