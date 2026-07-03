package diagnostics

import (
	"fmt"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func renderReturnJudgmentWithPolicy(item judgment.Judgment, policy judgment.Policy, mode judgment.StrictnessMode) (diagnostic.Diagnostic, bool) {
	if item.Code != judgment.CodeReturn || item.Subject.Kind != judgment.SubjectReturnValue || len(item.Spans) == 0 {
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
	declSpan := returnJudgmentEvidenceSpan(item, judgment.EvidenceUserAssertion)
	label := item.Subject.Label
	if label == "" {
		label = "returned value"
	}
	sourceName := item.Actual.Label
	subject := returnJudgmentSubject(label, sourceName)
	evidence := []diagnostic.Evidence{
		{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceAbstractFact, diagnostic.TrustProven),
			Span:    returnJudgmentEvidenceSpan(item, judgment.EvidenceAbstractFact),
			Message: assignmentSourceTypeEvidence(subject, got),
		},
		{
			Kind:    diagnostic.EvidenceUserAssertion,
			Trust:   diagnostic.TrustClaimed,
			Span:    declSpan,
			Message: returnDeclaredTypeEvidence(label, want),
		},
	}
	evidence = append(evidence, returnJudgmentExtraEvidence(item, subject, sourceName, got, want, span)...)
	return diagnostic.New(diagnostic.DiagnosticSpec{
		File:        item.Spans[0].File,
		Span:        span,
		Code:        CodeReturnContractType,
		Severity:    severity,
		Message:     returnJudgmentMessage(subject, sourceName, label, got, want, item),
		Help:        returnJudgmentHelp(sourceName, got),
		Explanation: diagnostic.NewExplanation(evidence...),
		Labels: []diagnostic.Label{
			sourceLabel(span, labelReturnedValue),
			sourceLabel(declSpan, labelDeclaredReturn),
		},
	}), true
}

func returnJudgmentSubject(label, sourceName string) string {
	if sourceName == "" {
		return label
	}
	if sourceName == label {
		return label
	}
	if strings.HasPrefix(sourceName, label+" (") {
		return sourceName
	}
	return fmt.Sprintf("%s (%s)", label, sourceName)
}

func returnJudgmentMessage(subject, sourceName, label string, got, want typ.Type, item judgment.Judgment) string {
	if item.HasEvidence(judgment.EvidencePrecisionBoundary) && !nilSafetyMismatch(got, want) {
		return fmt.Sprintf("%s comes from any/unknown; no proof shows it satisfies declared return type %s", subject, formatType(want))
	}
	if nilSafetyMismatch(got, want) {
		if sourceName != "" && sourceName != label {
			return fmt.Sprintf("cannot return %s as %s because it may be nil", sourceName, label)
		}
		return fmt.Sprintf("cannot return %s because it may be nil", subject)
	}
	gotText := formatType(got)
	wantText := formatType(want)
	if gotText == wantText {
		return fmt.Sprintf("cannot prove %s satisfies declared return type %s", subject, wantText)
	}
	if subject == "" {
		subject = label
	}
	return fmt.Sprintf("%s is %s, not %s", subject, gotText, wantText)
}

func returnJudgmentHelp(sourceName string, got typ.Type) string {
	if sourceName != "" && sourceName != unknownSourceName && valueMayBeNil(got) {
		return fmt.Sprintf("Guard `%s` with a nil check, return a default value, or change the return type to accept nil.", sourceName)
	}
	return "Return a value compatible with the declared return type, or change the return annotation if the returned value is valid."
}

func returnJudgmentExtraEvidence(item judgment.Judgment, subject, sourceName string, got, want typ.Type, primary diagnostic.Span) []diagnostic.Evidence {
	var out []diagnostic.Evidence
	for _, evidence := range item.Evidence {
		switch evidence.Kind {
		case judgment.EvidenceUserAssertion:
			if evidence.Detail.Kind == judgment.EvidenceDetailUserAssertedAny {
				out = append(out, diagnostic.Evidence{
					Kind:    diagnostic.EvidenceUserAssertion,
					Trust:   diagnosticTrustFromJudgmentTrust(evidence.Trust, diagnostic.TrustClaimed),
					Reason:  diagnostic.EvidenceReasonUserAssertedAny,
					Span:    diagnosticSpanFromJudgment(evidence.Span),
					Message: userAssertedAnyEvidence(),
				})
			}
		case judgment.EvidencePrecisionBoundary:
			out = append(out, diagnostic.Evidence{
				Kind:    diagnostic.EvidencePrecisionBoundary,
				Trust:   diagnosticTrustFromJudgmentTrust(evidence.Trust, diagnostic.TrustUnknown),
				Reason:  diagnostic.EvidenceReasonExplicitBoundaryValidation,
				Span:    returnJudgmentEvidenceSpanOr(evidence, primary),
				Message: returnExplicitBoundaryProofMessage(subject),
			})
		}
	}
	if !typ.IsAny(got) && !typ.IsUnknown(got) && !item.HasEvidence(judgment.EvidencePrecisionBoundary) && !nilSafetyMismatch(got, want) {
		return out
	}
	missingReason := diagnostic.EvidenceReasonBoundaryValidationMissing
	missingMessage := returnMissingProofMessage(subject)
	if nilSafetyMismatch(got, want) && strings.Contains(sourceName, "[") {
		missingReason = diagnostic.EvidenceReasonIndexReadValidationMissing
		missingMessage = returnIndexedReadProofMessage(subject)
	}
	out = append(out, diagnostic.Evidence{
		Kind:    diagnostic.EvidenceMissingProof,
		Trust:   missingProofTrustFromJudgment(item.Verdict),
		Reason:  missingReason,
		Span:    primary,
		Message: missingMessage,
	})
	return out
}

func returnJudgmentEvidenceSpan(item judgment.Judgment, kind judgment.EvidenceKind) diagnostic.Span {
	for _, evidence := range item.Evidence {
		if evidence.Kind == kind && evidence.Span.StartLine != 0 {
			return diagnosticSpanFromJudgment(evidence.Span)
		}
	}
	if len(item.Spans) > 0 {
		return diagnosticSpanFromJudgment(item.Spans[0])
	}
	return diagnostic.Span{}
}

func returnJudgmentEvidenceSpanOr(evidence judgment.Evidence, fallback diagnostic.Span) diagnostic.Span {
	if evidence.Span.StartLine != 0 {
		return diagnosticSpanFromJudgment(evidence.Span)
	}
	return fallback
}
