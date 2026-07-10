package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func renderRegistrationJudgmentWithPolicy(ctx judgmentRenderContext, item judgment.Judgment, policy judgment.Policy, mode judgment.StrictnessMode) (diagnostic.Diagnostic, bool) {
	if item.Code != judgment.CodeRegistration || item.Subject.Kind != judgment.SubjectExpression || len(item.Spans) == 0 {
		return diagnostic.Diagnostic{}, false
	}
	severity, ok := diagnosticSeverityForJudgment(item, policy, mode)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	code := diagnosticCodeForJudgment(item)
	primary := diagnosticSpanFromJudgment(item.Spans[0])
	presentation, ok := ctx.proof.Registration(item, primary)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Location:    item.Spans[0].Location,
		File:        item.Spans[0].DisplayFile(),
		Span:        primary,
		Code:        code,
		Severity:    severity,
		Message:     presentation.Message,
		Explanation: presentation.Explanation,
		Help:        presentation.Help,
		Labels:      presentation.Labels,
	}), true
}
