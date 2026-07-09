package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func renderAdviceJudgmentWithPolicy(ctx judgmentRenderContext, item judgment.Judgment, policy judgment.Policy, mode judgment.StrictnessMode) (diagnostic.Diagnostic, bool) {
	spec, ok := judgment.DefaultRegistry().Lookup(item.Code)
	if !ok || spec.Render != judgment.RenderAdvice || item.Subject.Kind != judgment.SubjectExpression || len(item.Spans) == 0 {
		return diagnostic.Diagnostic{}, false
	}
	severity, ok := diagnosticSeverityForJudgment(item, policy, mode)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	code := primaryDiagnosticCode(spec)
	span := diagnosticSpanFromJudgment(item.Spans[0])
	presentation, ok := ctx.proof.Advice(item, span)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	return diagnostic.New(diagnostic.DiagnosticSpec{
		File:        item.Spans[0].File,
		Span:        span,
		Code:        code,
		Severity:    severity,
		Message:     presentation.Message,
		Labels:      presentation.Labels,
		Explanation: presentation.Explanation,
		Help:        presentation.Help,
	}), true
}
