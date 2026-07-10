package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func renderUnresolvedTypeJudgmentWithPolicy(ctx judgmentRenderContext, item judgment.Judgment, policy judgment.Policy, mode judgment.StrictnessMode) (diagnostic.Diagnostic, bool) {
	if item.Code != judgment.CodeUnresolvedType || item.Subject.Kind != judgment.SubjectPath || len(item.Spans) == 0 {
		return diagnostic.Diagnostic{}, false
	}
	severity, ok := diagnosticSeverityForJudgment(item, policy, mode)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	code := diagnosticCodeForJudgment(item)
	name := item.Subject.Label
	if name == "" {
		name = "<missing>"
	}
	span := diagnosticSpanFromJudgment(item.Spans[0])
	presentation := ctx.proof.UnresolvedType(item, name, span)
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Location:    item.Spans[0].Location,
		File:        item.Spans[0].DisplayFile(),
		Span:        span,
		Code:        code,
		Severity:    severity,
		Message:     presentation.Message,
		Explanation: presentation.Explanation,
		Help:        presentation.Help,
		Labels:      presentation.Labels,
	}), true
}
