package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/readmodel"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func renderConcatOperandJudgmentWithPolicy(item judgment.Judgment, policy judgment.Policy, mode judgment.StrictnessMode) (diagnostic.Diagnostic, bool) {
	if item.Code != judgment.CodeConcatOperand || item.Subject.Kind != judgment.SubjectExpression || len(item.Spans) == 0 {
		return diagnostic.Diagnostic{}, false
	}
	severity, ok := diagnosticSeverityForJudgment(item, policy, mode)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	detail, ok := concatOperandJudgmentDetail(item)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	operandSpan := diagnosticSpanFromJudgment(item.Spans[0])
	operandName := item.Subject.Label
	got := item.Actual.ProjectedType
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Span:     operandSpan,
		Code:     CodeConcatOperand,
		Severity: severity,
		Message:  concatOperandMessage(detail.Field),
		Labels:   []diagnostic.Label{sourceLabel(operandSpan, labelValueMayBeNil)},
		Explanation: diagnostic.NewExplanation(
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Reason:  concatOperandEvidenceReason(got),
				Span:    operandSpan,
				Message: concatOperandTypeEvidence(detail.Field, operandName, got),
			},
		),
		Help: concatOperandHelp(operandName),
	}), true
}

func concatOperandJudgmentDetail(item judgment.Judgment) (judgment.EvidenceDetail, bool) {
	for _, evidence := range item.Evidence {
		if evidence.Detail.Kind == judgment.EvidenceDetailConcatOperand && evidence.Detail.Field != "" {
			return evidence.Detail, true
		}
	}
	return judgment.EvidenceDetail{}, false
}

func concatOperandEvidenceReason(got typ.Type) diagnostic.EvidenceReason {
	if got != nil && typ.Nil.Equals(got) {
		return diagnostic.EvidenceReasonExactType
	}
	if readmodel.ProjectionHasNil(got) {
		return diagnostic.EvidenceReasonUnionType
	}
	return diagnostic.EvidenceReasonUnspecified
}
