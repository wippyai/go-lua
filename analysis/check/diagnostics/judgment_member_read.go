package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func renderMemberReadJudgmentWithPolicy(ctx judgmentRenderContext, item judgment.Judgment, policy judgment.Policy, mode judgment.StrictnessMode) (diagnostic.Diagnostic, bool) {
	if item.Code != judgment.CodeMemberRead || item.Subject.Kind != judgment.SubjectExpression || len(item.Spans) == 0 {
		return diagnostic.Diagnostic{}, false
	}
	severity, ok := diagnosticSeverityForJudgment(item, policy, mode)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	span := diagnosticSpanFromJudgment(item.Spans[0])
	presentation, ok := ctx.proof.MemberRead(item, span)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	return diagnostic.New(diagnostic.DiagnosticSpec{
		File:        item.Spans[0].File,
		Span:        span,
		Code:        CodeMissingMember,
		Severity:    severity,
		Message:     presentation.Message,
		Explanation: diagnostic.NewExplanation(presentation.Evidence...),
		Help:        presentation.Help,
		Labels:      presentation.Labels,
	}), true
}
