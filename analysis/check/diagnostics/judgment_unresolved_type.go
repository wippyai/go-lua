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
	name := item.Subject.Label
	if name == "" {
		name = "<missing>"
	}
	span := diagnosticSpanFromJudgment(item.Spans[0])
	presentation := ctx.proof.UnresolvedType(item, name, span)
	return diagnostic.New(diagnostic.DiagnosticSpec{
		File:        item.Spans[0].File,
		Span:        span,
		Code:        CodeUnresolvedTypeReference,
		Severity:    severity,
		Message:     presentation.Message,
		Explanation: presentation.Explanation,
		Help:        presentation.Help,
		Labels:      presentation.Labels,
	}), true
}
