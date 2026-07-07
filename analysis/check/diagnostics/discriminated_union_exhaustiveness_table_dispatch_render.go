package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func renderTableDispatchJudgmentWithPolicy(ctx judgmentRenderContext, item judgment.Judgment, policy judgment.Policy, mode judgment.StrictnessMode) (diagnostic.Diagnostic, bool) {
	if item.Code != judgment.CodeTableDispatch || item.Subject.Kind != judgment.SubjectExpression || len(item.Spans) == 0 {
		return diagnostic.Diagnostic{}, false
	}
	severity, ok := diagnosticSeverityForJudgment(item, policy, mode)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	lookupSpan := diagnosticSpanFromJudgment(item.Spans[0])
	presentation, ok := ctx.proof.TableDispatch(item, lookupSpan)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	return diagnostic.New(diagnostic.DiagnosticSpec{
		File:        item.Spans[0].File,
		Span:        lookupSpan,
		Code:        CodeDiscriminatedUnionExhaustive,
		Severity:    severity,
		Message:     presentation.Message,
		Explanation: presentation.Explanation,
		Help:        presentation.Help,
		Labels:      presentation.Labels,
	}), true
}
