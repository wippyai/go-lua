package diagnostics

import (
	"fmt"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/type/typ"
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
	declSpan := diagnosticEvidenceSpanOrPrimary(item, judgment.EvidenceUserAssertion)
	target := item.Subject.Label
	if target == "" {
		target = "value"
	}
	sourceName := item.Actual.Label
	if detail, ok := assignmentJudgmentCallResultDetail(item); ok && !detail.UnderSupplied && assignmentJudgmentHasCallResultReturnSpan(item) {
		return renderCallResultAssignmentJudgment(item, detail, got, want, severity, span, declSpan)
	}
	proofContext := diagnosticProofContext()
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

func renderCallResultAssignmentJudgment(
	item judgment.Judgment,
	detail judgment.EvidenceDetail,
	got typ.Type,
	want typ.Type,
	severity diagnostic.Severity,
	callSpan diagnostic.Span,
	typeSpan diagnostic.Span,
) (diagnostic.Diagnostic, bool) {
	presentation := diagnosticProofContext().AssignmentCallResult(item, detail, got, want, callSpan, typeSpan)
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Span:        callSpan,
		Code:        CodeDirectCallResultAssignment,
		Severity:    severity,
		Message:     presentation.Message,
		Help:        presentation.Help,
		Explanation: diagnostic.NewExplanation(presentation.Evidence...),
		Labels:      presentation.Labels,
	}), true
}

func callResultSubject(index int) string {
	if index >= 0 {
		return fmt.Sprintf("call result %d", index+1)
	}
	return "call result"
}

func assignmentJudgmentCallResultDetail(item judgment.Judgment) (judgment.EvidenceDetail, bool) {
	return item.AssignmentCallResultDetail()
}

func assignmentJudgmentUnderSuppliedCallResultDetail(item judgment.Judgment) (judgment.EvidenceDetail, bool) {
	return item.AssignmentUnderSuppliedCallResultDetail()
}

func assignmentJudgmentCallResultReturnSpan(item judgment.Judgment) (diagnostic.Span, bool) {
	span, ok := item.AssignmentCallResultReturnSpan()
	if ok {
		return diagnosticSpanFromJudgment(span), true
	}
	return diagnostic.Span{}, false
}

func assignmentJudgmentHasCallResultReturnSpan(item judgment.Judgment) bool {
	_, ok := assignmentJudgmentCallResultReturnSpan(item)
	return ok
}

func diagnosticSpanEqual(a, b diagnostic.Span) bool {
	return a.StartLine == b.StartLine &&
		a.StartCol == b.StartCol &&
		a.EndLine == b.EndLine &&
		a.EndCol == b.EndCol
}

func assignmentJudgmentTargetLooksMember(target string, sourceName string) bool {
	if target == "" || target == sourceName {
		return false
	}
	return strings.Contains(target, ".") || strings.Contains(target, "[")
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

func assignmentJudgmentMissingRequiredField(item judgment.Judgment) (judgment.EvidenceDetail, bool) {
	return item.AssignmentMissingRequiredField()
}

func assignmentJudgmentMissingRequiredMethod(item judgment.Judgment) (judgment.EvidenceDetail, bool) {
	return item.AssignmentMissingRequiredMethod()
}

func assignmentJudgmentMethodTypeMismatch(item judgment.Judgment) (judgment.EvidenceDetail, bool) {
	return item.AssignmentMethodTypeMismatch()
}

func functionTypeOrNil(t typ.Type) *typ.Function {
	fn, _ := t.(*typ.Function)
	return fn
}

func assignmentJudgmentExpectedTypeLabel(item judgment.Judgment, target string, fallback typ.Type) string {
	if item.Expected.Label != "" && item.Expected.Label != target {
		return item.Expected.Label
	}
	return formatType(fallback)
}

func assignmentJudgmentExpectedEvidence(item judgment.Judgment, target string, fallback typ.Type, expectedDisplay string) string {
	if evidence, ok := item.FirstEvidenceDetail(judgment.EvidenceDetailDynamicAssignmentTarget); ok {
		label := evidence.Detail.SubjectLabel
		if label == "" {
			label = target
		}
		return fmt.Sprintf("assignment target %s requires %s", label, formatType(fallback))
	}
	if expectedDisplay == "" {
		expectedDisplay = assignmentJudgmentExpectedTypeLabel(item, target, fallback)
	}
	return fmt.Sprintf("%s is declared as %s", target, expectedDisplay)
}

func assignmentJudgmentExpectedEvidenceKind(item judgment.Judgment) diagnostic.EvidenceKind {
	if item.HasEvidenceDetail(judgment.EvidenceDetailDynamicAssignmentTarget) {
		return diagnostic.EvidenceAbstractFact
	}
	return diagnostic.EvidenceUserAssertion
}

func assignmentJudgmentExpectedEvidenceTrust(item judgment.Judgment) diagnostic.TrustKind {
	if item.HasEvidenceDetail(judgment.EvidenceDetailDynamicAssignmentTarget) {
		return diagnostic.TrustProven
	}
	return diagnostic.TrustClaimed
}

func assignmentJudgmentSourceLabel(missingField bool) string {
	if missingField {
		return labelObjectLiteral
	}
	return labelAssignedValue
}
