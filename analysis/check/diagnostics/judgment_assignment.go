package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func renderAssignmentJudgmentWithPolicy(ctx judgmentRenderContext, item judgment.Judgment, policy judgment.Policy, mode judgment.StrictnessMode) (diagnostic.Diagnostic, bool) {
	if item.Code != judgment.CodeAssignment || item.Subject.Kind != judgment.SubjectPath || len(item.Spans) == 0 {
		return diagnostic.Diagnostic{}, false
	}
	severity, ok := diagnosticSeverityForJudgment(item, policy, mode)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	got := item.Actual.ProjectedType
	want := item.Expected.Type
	if got == nil || want == nil {
		return diagnostic.Diagnostic{}, false
	}
	span := diagnosticSpanFromJudgment(item.Spans[0])
	target := item.Subject.Label
	if target == "" {
		target = "value"
	}
	sourceName := item.Actual.Label
	proofContext := ctx.proof
	if presentation, ok := proofContext.AssignmentCallResultForItem(item, got, want, span); ok {
		code := diagnosticCodeForJudgmentAt(item, 1)
		return diagnostic.New(diagnostic.DiagnosticSpec{
			Span:        span,
			Code:        code,
			Severity:    severity,
			Message:     presentation.Message,
			Help:        presentation.Help,
			Explanation: diagnostic.NewExplanation(presentation.Evidence...),
			Labels:      presentation.Labels,
		}), true
	}
	code := diagnosticCodeForJudgmentAt(item, 0)
	expectedDisplay := assignmentJudgmentExpectedTypeLabel(item, target, want)
	presentation := proofContext.AssignmentDiagnostic(item, target, sourceName, got, want, span, expectedDisplay)
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Span:        span,
		Code:        code,
		Severity:    severity,
		Message:     presentation.Message,
		Explanation: diagnostic.NewExplanation(presentation.Evidence...),
		Help:        presentation.Help,
		Labels:      presentation.Labels,
	}), true
}

func renderOptionalAssignmentTargetJudgmentWithPolicy(ctx judgmentRenderContext, item judgment.Judgment, policy judgment.Policy, mode judgment.StrictnessMode) (diagnostic.Diagnostic, bool) {
	if item.Code != judgment.CodeAssignmentTarget || item.Subject.Kind != judgment.SubjectPath || len(item.Spans) == 0 {
		return diagnostic.Diagnostic{}, false
	}
	severity, ok := diagnosticSeverityForJudgment(item, policy, mode)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	code := diagnosticCodeForJudgment(item)
	targetSpan := diagnosticSpanFromJudgment(item.Spans[0])
	presentation := ctx.proof.OptionalAssignmentTarget(item, targetSpan)
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Span:        targetSpan,
		Code:        code,
		Severity:    severity,
		Message:     presentation.Message,
		Help:        presentation.Help,
		Explanation: presentation.Explanation,
		Labels:      presentation.Labels,
	}), true
}
