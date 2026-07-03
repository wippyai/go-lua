package diagnostics

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/obligation/pass"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func produceAssignmentJudgmentDiagnostics(result *body.Result, sourceFile string) []diagnostic.Diagnostic {
	return produceAssignmentJudgmentDiagnosticsWithPolicy(result, sourceFile, judgment.DefaultPolicy(), judgment.StrictnessDefault)
}

func produceAssignmentJudgmentDiagnosticsWithPolicy(result *body.Result, sourceFile string, policy judgment.Policy, mode judgment.StrictnessMode) []diagnostic.Diagnostic {
	query := newDiagnosticQuery(result)
	items := pass.New(pass.Assignments{}).Run(pass.Context{
		FunctionKey: sourceFile,
		SourceFile:  sourceFile,
		Reader:      query.reader,
	})
	return renderJudgmentDiagnostics(items, policy, mode)
}

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
	declSpan := assignmentJudgmentEvidenceSpan(item, judgment.EvidenceUserAssertion)
	target := item.Subject.Label
	if target == "" {
		target = "value"
	}
	sourceName := item.Actual.Label
	missingField, hasMissingField := assignmentJudgmentMissingRequiredField(item)
	evidenceSourceName := sourceName
	if evidenceSourceName == "" {
		evidenceSourceName = "assigned value"
	}
	extraEvidence := assignmentJudgmentExtraEvidence(item, evidenceSourceName, got, want, span)
	sourceEvidence := assignmentSourceTypeEvidence(evidenceSourceName, got)
	if indexedReadMissingProofMismatch(got, want, extraEvidence) && evidenceSourceName != "" && evidenceSourceName != unknownSourceName {
		sourceEvidence = evidenceSourceName + " can be nil here"
	}
	if hasMissingField {
		sourceEvidence = objectLiteralShapeEvidence(got)
	}
	message := assignmentMessageForEvidence(sourceName, got, want, extraEvidence)
	help := assignmentHelpForEvidence(sourceName, got, extraEvidence)
	evidence := []diagnostic.Evidence{
		{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceAbstractFact, diagnostic.TrustProven),
			Span:    assignmentJudgmentEvidenceSpan(item, judgment.EvidenceAbstractFact),
			Message: sourceEvidence,
		},
		{
			Kind:    diagnostic.EvidenceUserAssertion,
			Trust:   diagnostic.TrustClaimed,
			Span:    declSpan,
			Message: fmt.Sprintf("%s is declared as %s", target, assignmentJudgmentExpectedTypeLabel(item, target, want)),
		},
	}
	evidence = append(evidence, extraEvidence...)
	if hasMissingField {
		message = missingRequiredFieldMessage(missingField.Field)
		help = missingRequiredFieldHelp(missingField.Field)
		path := target
		if path != "" && missingField.Field != "" {
			path += "." + missingField.Field
		}
		evidence = append(evidence, diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    span,
			Message: missingRequiredFieldPathEvidence(path, missingField.FieldType),
		})
	}
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Span:        span,
		Code:        CodeAssignmentType,
		Severity:    severity,
		Message:     message,
		Explanation: diagnostic.NewExplanation(evidence...),
		Help:        help,
		Labels: []diagnostic.Label{
			sourceLabel(span, assignmentJudgmentSourceLabel(hasMissingField)),
			sourceLabel(declSpan, labelDeclaredType),
		},
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
	containerName := item.Actual.Label
	if containerName == "" {
		containerName = "value"
	}
	targetName := item.Subject.Label
	if targetName == "" {
		targetName = containerName
	}
	containerType := item.Actual.ProjectedType
	targetSpan := diagnosticSpanFromJudgment(item.Spans[0])
	containerSpan := assignmentJudgmentEvidenceSpan(item, judgment.EvidenceAbstractFact)
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Span:     targetSpan,
		Code:     CodeOptionalAssignmentTarget,
		Severity: severity,
		Message:  optionalAssignmentTargetMessage(containerName),
		Help:     optionalAssignmentTargetHelp(containerName),
		Explanation: diagnostic.NewExplanation(
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    containerSpan,
				Message: optionalAssignmentTargetContainerEvidence(containerName, containerType),
			},
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    targetSpan,
				Message: optionalAssignmentTargetWriteEvidence(targetName),
			},
		),
		Labels: []diagnostic.Label{
			sourceLabel(containerSpan, labelPossiblyNilContainer),
			sourceLabel(targetSpan, labelAssignmentTarget),
		},
	}), true
}

func assignmentJudgmentMissingRequiredField(item judgment.Judgment) (judgment.EvidenceDetail, bool) {
	for _, evidence := range item.Evidence {
		if evidence.Detail.Kind == judgment.EvidenceDetailMissingRequiredField && evidence.Detail.Field != "" {
			return evidence.Detail, true
		}
	}
	return judgment.EvidenceDetail{}, false
}

func assignmentJudgmentExpectedTypeLabel(item judgment.Judgment, target string, fallback typ.Type) string {
	if item.Expected.Label != "" && item.Expected.Label != target {
		return item.Expected.Label
	}
	return formatType(fallback)
}

func assignmentJudgmentSourceLabel(missingField bool) string {
	if missingField {
		return labelObjectLiteral
	}
	return labelAssignedValue
}

func assignmentJudgmentEvidenceSpan(item judgment.Judgment, kind judgment.EvidenceKind) diagnostic.Span {
	for _, evidence := range item.Evidence {
		if evidence.Kind == kind && evidence.Span.StartLine != 0 {
			return diagnosticSpanFromJudgment(evidence.Span)
		}
	}
	if len(item.Spans) == 0 {
		return diagnostic.Span{}
	}
	return diagnosticSpanFromJudgment(item.Spans[0])
}

func assignmentJudgmentExtraEvidence(item judgment.Judgment, sourceName string, got, want typ.Type, sourceSpan diagnostic.Span) []diagnostic.Evidence {
	var out []diagnostic.Evidence
	if item.HasEvidence(judgment.EvidencePrecisionBoundary) {
		out = append(out, diagnostic.Evidence{
			Kind:    diagnostic.EvidencePrecisionBoundary,
			Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidencePrecisionBoundary, diagnostic.TrustUnknown),
			Reason:  diagnostic.EvidenceReasonExplicitBoundaryValidation,
			Span:    sourceSpan,
			Message: fmt.Sprintf("%s comes from any/unknown", boundaryEvidenceSubject(sourceName)),
		})
	}
	out = append(out, assignmentJudgmentNilableAccessEvidence(item)...)
	if sourceName != "" && sourceName != unknownSourceName && projectionHasNil(got) && !projectionHasNil(want) {
		return appendMissingNilGuardEvidence(out, sourceName, got, sourceSpan, assignmentSourceLooksIndexed(sourceName))
	}
	if item.Verdict == judgment.VerdictUnknown || item.HasEvidence(judgment.EvidencePrecisionBoundary) {
		if !evidenceHasKind(out, diagnostic.EvidenceMissingProof) {
			out = append(out, diagnostic.Evidence{
				Kind:    diagnostic.EvidenceMissingProof,
				Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceMissingProof, missingProofTrustFromJudgment(item.Verdict)),
				Reason:  assignmentJudgmentMissingProofReason(item),
				Span:    sourceSpan,
				Message: fmt.Sprintf("no proof on this path shows %s satisfies the declared type", boundaryEvidenceSubject(sourceName)),
			})
		}
	}
	return out
}

func assignmentJudgmentNilableAccessEvidence(item judgment.Judgment) []diagnostic.Evidence {
	var out []diagnostic.Evidence
	for _, evidence := range item.Evidence {
		if evidence.Kind != judgment.EvidenceAbstractFact ||
			evidence.Detail.Kind != judgment.EvidenceDetailMayBeNil ||
			evidence.Detail.SubjectLabel == "" ||
			evidence.Detail.Field == "" {
			continue
		}
		out = append(out, diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnosticTrustFromJudgmentTrust(evidence.Trust, diagnostic.TrustProven),
			Span:    diagnosticSpanFromJudgment(evidence.Span),
			Message: assignmentNilableAccessMessage(evidence.Detail.SubjectLabel, evidence.Detail.Field),
		})
	}
	return out
}

func assignmentNilableAccessMessage(label, access string) string {
	if len(access) > 0 && access[0] == '[' {
		return fmt.Sprintf("%s may be nil before indexing %s", label, access)
	}
	return fmt.Sprintf("%s may be nil before reading %s", label, access)
}

func assignmentJudgmentMissingProofReason(item judgment.Judgment) diagnostic.EvidenceReason {
	if item.HasEvidence(judgment.EvidencePrecisionBoundary) {
		return diagnostic.EvidenceReasonBoundaryValidationMissing
	}
	return diagnostic.EvidenceReasonUnspecified
}
