package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func renderReturnJudgmentWithPolicy(ctx judgmentRenderContext, item judgment.Judgment, policy judgment.Policy, mode judgment.StrictnessMode) (diagnostic.Diagnostic, bool) {
	if item.Code != judgment.CodeReturn || item.Subject.Kind != judgment.SubjectReturnValue || len(item.Spans) == 0 {
		return diagnostic.Diagnostic{}, false
	}
	severity, ok := diagnosticSeverityForJudgment(item, policy, mode)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	code := diagnosticCodeForJudgment(item)
	got := item.Actual.ProjectedType
	want := item.Expected.Type
	if got == nil || want == nil {
		return diagnostic.Diagnostic{}, false
	}
	span := diagnosticSpanFromJudgment(item.Spans[0])
	label := item.Subject.Label
	if label == "" {
		label = "returned value"
	}
	sourceName := item.Actual.Label
	presentation := ctx.proof.Return(item, label, sourceName, got, want, span)
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Location:    item.Spans[0].Location,
		File:        item.Spans[0].DisplayFile(),
		Span:        span,
		Code:        code,
		Severity:    severity,
		Message:     presentation.Message,
		Help:        presentation.Help,
		Explanation: diagnostic.NewExplanation(presentation.Evidence...),
		Labels:      presentation.Labels,
	}), true
}
