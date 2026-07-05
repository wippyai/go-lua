package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func renderAssignmentJudgmentWithPolicy(item judgment.Judgment, policy judgment.Policy, mode judgment.StrictnessMode) (diagnostic.Diagnostic, bool) {
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
	proofContext := diagnosticProofContext()
	if presentation, ok := proofContext.AssignmentCallResultForItem(item, got, want, span); ok {
		return diagnostic.New(diagnostic.DiagnosticSpec{
			Span:        span,
			Code:        CodeDirectCallResultAssignment,
			Severity:    severity,
			Message:     presentation.Message,
			Help:        presentation.Help,
			Explanation: diagnostic.NewExplanation(presentation.Evidence...),
			Labels:      presentation.Labels,
		}), true
	}
	expectedDisplay := assignmentJudgmentExpectedTypeLabel(item, target, want)
	presentation := proofContext.AssignmentDiagnostic(item, target, sourceName, got, want, span, expectedDisplay)
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Span:        span,
		Code:        CodeAssignmentType,
		Severity:    severity,
		Message:     presentation.Message,
		Explanation: diagnostic.NewExplanation(presentation.Evidence...),
		Help:        presentation.Help,
		Labels:      presentation.Labels,
	}), true
}

func renderOptionalAssignmentTargetJudgmentWithPolicy(item judgment.Judgment, policy judgment.Policy, mode judgment.StrictnessMode) (diagnostic.Diagnostic, bool) {
	if item.Code != judgment.CodeAssignmentTarget || item.Subject.Kind != judgment.SubjectPath || len(item.Spans) == 0 {
		return diagnostic.Diagnostic{}, false
	}
	severity, ok := diagnosticSeverityForJudgment(item, policy, mode)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	targetSpan := diagnosticSpanFromJudgment(item.Spans[0])
	presentation := diagnosticProofContext().OptionalAssignmentTarget(item, targetSpan)
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Span:        targetSpan,
		Code:        CodeOptionalAssignmentTarget,
		Severity:    severity,
		Message:     presentation.Message,
		Help:        presentation.Help,
		Explanation: presentation.Explanation,
		Labels:      presentation.Labels,
	}), true
}
