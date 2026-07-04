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
	underSuppliedDetail, underSupplied := assignmentJudgmentUnderSuppliedCallResultDetail(item)
	missingField, hasMissingField := assignmentJudgmentMissingRequiredField(item)
	missingMethod, hasMissingMethod := assignmentJudgmentMissingRequiredMethod(item)
	methodMismatch, hasMethodMismatch := assignmentJudgmentMethodTypeMismatch(item)
	evidenceSourceName := sourceName
	if evidenceSourceName == "" {
		evidenceSourceName = "assigned value"
	}
	if underSupplied {
		sourceName = target
		evidenceSourceName = target
	}
	extraEvidence := assignmentJudgmentExtraEvidence(item, evidenceSourceName, got, want, span)
	expectedDisplay := assignmentJudgmentExpectedTypeLabel(item, target, want)
	dynamicTarget := assignmentJudgmentHasDynamicTargetEvidence(item)
	if underSupplied {
		source := item.Actual.Label
		if source == "" {
			source = underSuppliedDetail.FunctionName
		}
		underSuppliedEvidence := diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    span,
			Message: underSuppliedTargetEvidence(target, source, underSuppliedDetail.ResultIndex),
		}
		extraEvidence = append([]diagnostic.Evidence{underSuppliedEvidence}, extraEvidence...)
	}
	sourceEvidence := assignmentSourceTypeEvidence(evidenceSourceName, got)
	if indexedReadMissingProofMismatchForSource(evidenceSourceName, got, want, extraEvidence) && typ.Nil.Equals(got) && evidenceSourceName != "" && evidenceSourceName != unknownSourceName {
		sourceEvidence = evidenceSourceName + " can be nil here"
	}
	if hasMissingField {
		sourceEvidence = objectLiteralShapeEvidence(got)
	}
	if hasMissingMethod || hasMethodMismatch {
		sourceEvidence = objectLiteralShapeEvidence(got)
	}
	message := assignmentMessageForEvidenceDisplay(sourceName, got, want, expectedDisplay, extraEvidence)
	if assignmentJudgmentTargetLooksMember(target, sourceName) && !dynamicTarget {
		message = memberAssignmentMessageForEvidenceDisplay(target, sourceName, got, want, expectedDisplay, extraEvidence)
	}
	help := assignmentHelpForEvidence(sourceName, got, extraEvidence)
	if underSupplied {
		help = underSuppliedTargetHelp(target)
	}
	evidence := []diagnostic.Evidence{
		{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceAbstractFact, diagnostic.TrustProven),
			Span:    diagnosticEvidenceSpanOrPrimary(item, judgment.EvidenceAbstractFact),
			Message: sourceEvidence,
		},
		{
			Kind:    assignmentJudgmentExpectedEvidenceKind(item),
			Trust:   assignmentJudgmentExpectedEvidenceTrust(item),
			Span:    declSpan,
			Message: assignmentJudgmentExpectedEvidence(item, target, want, expectedDisplay),
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
	if hasMissingMethod {
		message = missingRequiredMethodMessage(want, missingMethod.Field)
		help = missingRequiredMethodHelp(missingMethod.Field)
		evidence = append(evidence, diagnostic.Evidence{
			Kind:    diagnostic.EvidenceMissingProof,
			Trust:   diagnostic.TrustUnknown,
			Span:    span,
			Message: missingRequiredMethodTypeEvidence(want, typ.Method{Name: missingMethod.Field, Type: functionTypeOrNil(missingMethod.FieldType)}),
		})
	}
	if hasMethodMismatch {
		message = methodTypeMismatchMessage(want, methodMismatch.Field, methodMismatch.ActualType, methodMismatch.FieldType)
		evidence = append(evidence, diagnostic.Evidence{
			Kind:    diagnostic.EvidenceMissingProof,
			Trust:   diagnostic.TrustUnknown,
			Span:    span,
			Message: methodTypeMismatchEvidence(want, methodMismatch.Field, methodMismatch.ActualType, methodMismatch.FieldType),
		})
	}
	sourceLabelMessage := assignmentJudgmentSourceLabel(hasMissingField)
	if hasMissingMethod || hasMethodMismatch {
		sourceLabelMessage = labelObjectLiteral
	}
	if underSupplied {
		sourceLabelMessage = labelCallResult
	}
	labels := []diagnostic.Label{sourceLabel(span, sourceLabelMessage)}
	if !diagnosticSpanEqual(declSpan, span) {
		expectedLabel := labelDeclaredType
		if dynamicTarget {
			expectedLabel = labelAssignmentTarget
		}
		labels = append(labels, sourceLabel(declSpan, expectedLabel))
	}
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Span:        span,
		Code:        CodeAssignmentType,
		Severity:    severity,
		Message:     message,
		Explanation: diagnostic.NewExplanation(evidence...),
		Help:        help,
		Labels:      labels,
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
	label := callResultSubject(detail.ResultIndex)
	name := detail.FunctionName
	if name == "" {
		name = item.Actual.Label
	}
	if name == "" {
		name = "call"
	}
	target := item.Subject.Label
	if target == "" {
		target = "assignment target"
	}
	if item.HasEvidence(judgment.EvidencePrecisionBoundary) {
		got = typ.Any
	}
	evidence := make([]diagnostic.Evidence, 0, 3)
	if retSpan, ok := assignmentJudgmentCallResultReturnSpan(item); ok {
		evidence = append(evidence, diagnostic.Evidence{
			Kind:    diagnostic.EvidenceUserAssertion,
			Trust:   diagnostic.TrustClaimed,
			Span:    retSpan,
			Message: callResultDeclaredReturnEvidence(name, label, got),
		})
	} else {
		evidence = append(evidence, diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    callSpan,
			Message: fmt.Sprintf("%s returns %s", name, formatType(got)),
		})
	}
	evidence = append(evidence, diagnostic.Evidence{
		Kind:    diagnostic.EvidenceUserAssertion,
		Trust:   diagnostic.TrustClaimed,
		Span:    typeSpan,
		Message: assignmentTargetTypeEvidence(target, want),
	})
	evidence = append(evidence, assignmentJudgmentExtraEvidence(item, label, got, want, callSpan)...)
	labels := []diagnostic.Label{
		sourceLabel(callSpan, labelCallResult),
		sourceLabel(typeSpan, labelDeclaredType),
	}
	if retSpan, ok := assignmentJudgmentCallResultReturnSpan(item); ok {
		labels = append(labels, sourceLabel(retSpan, labelDeclaredReturn))
	}
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Span:        callSpan,
		Code:        CodeDirectCallResultAssignment,
		Severity:    severity,
		Message:     fmt.Sprintf("%s is %s, not %s", label, formatType(got), formatType(want)),
		Help:        callResultAssignmentHelp(got),
		Explanation: diagnostic.NewExplanation(evidence...),
		Labels:      labels,
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
	containerSpan := diagnosticEvidenceSpanOrPrimary(item, judgment.EvidenceAbstractFact)
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

func assignmentJudgmentExtraEvidence(item judgment.Judgment, sourceName string, got, want typ.Type, sourceSpan diagnostic.Span) []diagnostic.Evidence {
	var out []diagnostic.Evidence
	out = append(out, assignmentJudgmentUserAssertionEvidence(item)...)
	if item.HasEvidence(judgment.EvidencePrecisionBoundary) {
		out = append(out, diagnostic.Evidence{
			Kind:    diagnostic.EvidencePrecisionBoundary,
			Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidencePrecisionBoundary, diagnostic.TrustUnknown),
			Reason:  diagnostic.EvidenceReasonExplicitBoundaryValidation,
			Span:    sourceSpan,
			Message: fmt.Sprintf("%s comes from any/unknown", boundaryEvidenceSubject(sourceName)),
		})
	}
	if sourceName != "" && sourceName != unknownSourceName &&
		typ.Nil.Equals(got) &&
		assignmentJudgmentMissingProofMayBeNil(item) &&
		!assignmentSourceEndsWithIndex(sourceName) {
		out = append(out, assignmentJudgmentNilableAccessEvidence(item)...)
		out = append(out, assignmentJudgmentSourceContributionEvidence(item)...)
		out = append(out, assignmentJudgmentCallInvalidationEvidence(item)...)
		if !evidenceHasKind(out, diagnostic.EvidenceMissingProof) {
			out = append(out, diagnostic.Evidence{
				Kind:    diagnostic.EvidenceMissingProof,
				Trust:   assignmentMissingProofTrust(item),
				Reason:  assignmentJudgmentMissingProofReason(item),
				Span:    sourceSpan,
				Message: assignmentJudgmentMissingProofMessage(item, sourceName, got, want),
			})
		}
		return out
	}
	if sourceName != "" && sourceName != unknownSourceName && assignmentJudgmentMissingProofMayBeNil(item) {
		out = append(out, assignmentJudgmentSourceContributionEvidence(item)...)
		if !assignmentJudgmentMissingProofIndexedRead(item) {
			out = append(out, assignmentJudgmentNilableAccessEvidence(item)...)
		}
		out = append(out, assignmentJudgmentCallInvalidationEvidence(item)...)
		if assignmentJudgmentMissingProofIndexedRead(item) {
			return appendMissingNilGuardEvidence(out, sourceName, got, sourceSpan, true)
		}
		if assignmentSourceLooksIndexed(sourceName) {
			return appendMissingNilGuardEvidence(out, sourceName, got, sourceSpan, true)
		}
		return appendMissingNilGuardEvidence(out, sourceName, got, sourceSpan, false)
	}
	out = append(out, assignmentJudgmentNilableAccessEvidence(item)...)
	out = append(out, assignmentJudgmentSourceContributionEvidence(item)...)
	out = append(out, assignmentJudgmentCallInvalidationEvidence(item)...)
	if item.Verdict == judgment.VerdictUnknown || item.HasEvidence(judgment.EvidencePrecisionBoundary) {
		if !evidenceHasKind(out, diagnostic.EvidenceMissingProof) {
			out = append(out, diagnostic.Evidence{
				Kind:    diagnostic.EvidenceMissingProof,
				Trust:   assignmentMissingProofTrust(item),
				Reason:  assignmentJudgmentMissingProofReason(item),
				Span:    sourceSpan,
				Message: assignmentJudgmentMissingProofMessage(item, sourceName, got, want),
			})
		}
	}
	if assignmentJudgmentHasDynamicTargetEvidence(item) && !evidenceHasKind(out, diagnostic.EvidenceMissingProof) {
		out = append(out, diagnostic.Evidence{
			Kind:    diagnostic.EvidenceMissingProof,
			Trust:   diagnostic.TrustUnknown,
			Reason:  diagnostic.EvidenceReasonBoundaryValidationMissing,
			Span:    sourceSpan,
			Message: assignmentJudgmentMissingProofMessage(item, sourceName, got, want),
		})
	}
	return out
}

func evidenceHasKind(items []diagnostic.Evidence, kind diagnostic.EvidenceKind) bool {
	for _, item := range items {
		if item.Kind == kind {
			return true
		}
	}
	return false
}

func assignmentMessageForEvidenceDisplay(sourceName string, got, want typ.Type, wantDisplay string, evidence []diagnostic.Evidence) string {
	if indexedReadMissingProofMismatch(got, want, evidence) && sourceName != "" && sourceName != unknownSourceName {
		return "cannot assign " + sourceName + " because it may be nil"
	}
	if sameRenderedTypeNeedsValidationProof(got, want, evidence) {
		subject := boundaryEvidenceSubject(sourceName)
		return "cannot assign " + sourceName + " because " + subject + " comes from any/unknown; no proof shows it satisfies the declared type"
	}
	return assignmentMessageDisplay(sourceName, got, want, wantDisplay)
}

func memberAssignmentMessageForEvidenceDisplay(memberName string, sourceName string, got, want typ.Type, wantDisplay string, evidence []diagnostic.Evidence) string {
	if sameRenderedTypeNeedsValidationProof(got, want, evidence) {
		subject := boundaryEvidenceSubject(sourceName)
		return "cannot assign " + sourceName + " to " + memberName + " because " + subject + " comes from any/unknown; no proof shows it satisfies the declared type"
	}
	return memberAssignmentMessageDisplay(memberName, sourceName, got, want, wantDisplay)
}

func assignmentHelpForEvidence(sourceName string, got typ.Type, evidence []diagnostic.Evidence) string {
	if indexedReadHasMissingProof(evidence) && sourceName != "" && sourceName != unknownSourceName {
		return "Guard `" + sourceName + "` with a nil check, provide a default value, or change the target type to accept nil."
	}
	return assignmentHelp(sourceName, got)
}

func indexedReadMissingProofMismatchForSource(sourceName string, got, want typ.Type, evidence []diagnostic.Evidence) bool {
	if !assignmentSourceLooksIndexed(sourceName) && !assignmentSourceEndsWithIndex(sourceName) {
		return false
	}
	return indexedReadMissingProofMismatch(got, want, evidence)
}

func indexedReadMissingProofMismatch(got, want typ.Type, evidence []diagnostic.Evidence) bool {
	return indexedReadHasMissingProof(evidence) &&
		projectionHasNil(got) &&
		!projectionHasNil(want)
}

func indexedReadHasMissingProof(items []diagnostic.Evidence) bool {
	for _, item := range items {
		if item.Kind == diagnostic.EvidenceMissingProof &&
			item.Reason == diagnostic.EvidenceReasonIndexReadValidationMissing {
			return true
		}
	}
	return false
}

func sameRenderedTypeNeedsValidationProof(got, want typ.Type, evidence []diagnostic.Evidence) bool {
	return typ.TypeEquals(got, want) && evidenceHasKind(evidence, diagnostic.EvidencePrecisionBoundary)
}

func appendMissingNilGuardEvidence(items []diagnostic.Evidence, sourceName string, got typ.Type, sourceSpan diagnostic.Span, sourceIndexed ...bool) []diagnostic.Evidence {
	indexed := assignmentSourceLooksIndexed(sourceName, sourceIndexed...)
	directIndexed := len(sourceIndexed) > 0 && sourceIndexed[0]
	if sourceName == "" ||
		sourceName == unknownSourceName ||
		(!valueMayBeNil(got) && !(directIndexed && typ.Nil.Equals(got))) ||
		evidenceHasKind(items, diagnostic.EvidenceMissingProof) {
		return items
	}
	reason := diagnostic.EvidenceReasonBoundaryValidationMissing
	message := missingNonNilGuardHereMessage(sourceName)
	if indexed {
		reason = diagnostic.EvidenceReasonIndexReadValidationMissing
		message = indexedReadExpectedProofMessage(sourceName, "declared type")
	}
	return append(items, diagnostic.Evidence{
		Kind:    diagnostic.EvidenceMissingProof,
		Trust:   diagnostic.TrustUnknown,
		Reason:  reason,
		Span:    sourceSpan,
		Message: message,
	})
}

func assignmentSourceLooksIndexed(sourceName string, sourceIndexed ...bool) bool {
	if len(sourceIndexed) > 0 && sourceIndexed[0] {
		return true
	}
	return strings.Contains(sourceName, "[") && strings.Contains(sourceName, "]")
}

func assignmentSourceEndsWithIndex(sourceName string) bool {
	if sourceName == "" {
		return false
	}
	close := strings.LastIndex(sourceName, "]")
	open := strings.LastIndex(sourceName, "[")
	return close == len(sourceName)-1 && open >= 0 && open < close
}

func assignmentJudgmentMissingProofMessage(item judgment.Judgment, sourceName string, got typ.Type, want typ.Type) string {
	subject := boundaryEvidenceSubject(sourceName)
	if assignmentJudgmentMissingProofIndexedRead(item) {
		return indexedReadExpectedProofMessage(subject, "declared type")
	}
	if assignmentJudgmentHasCallInvalidationEvidence(item) {
		return missingBoundaryProofMessageForSubject(subject, want)
	}
	if assignmentJudgmentHasDynamicTargetEvidence(item) {
		return missingBoundaryProofMessageForSubject(subject, want)
	}
	if sourceName == "assigned value" || typ.Nil.Equals(got) || item.Expected.Label == "" {
		return missingBoundaryProofMessageForSubject(subject, want)
	}
	return missingExpectedProofMessage(subject, "declared type")
}

func assignmentJudgmentHasCallInvalidationEvidence(item judgment.Judgment) bool {
	return item.AssignmentHasCallInvalidationEvidence()
}

func assignmentJudgmentHasDynamicTargetEvidence(item judgment.Judgment) bool {
	return item.AssignmentHasDynamicTargetEvidence()
}

func assignmentJudgmentMissingProofMayBeNil(item judgment.Judgment) bool {
	return item.AssignmentMissingProofMayBeNil()
}

func assignmentJudgmentMissingProofIndexedRead(item judgment.Judgment) bool {
	return item.AssignmentMissingProofIndexedRead()
}

func assignmentJudgmentUserAssertionEvidence(item judgment.Judgment) []diagnostic.Evidence {
	var out []diagnostic.Evidence
	for _, evidence := range item.AssignmentUserAssertedAnyEvidence() {
		out = append(out, diagnostic.Evidence{
			Kind:    diagnostic.EvidenceUserAssertion,
			Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceUserAssertion, diagnostic.TrustClaimed),
			Reason:  diagnostic.EvidenceReasonUserAssertedAny,
			Span:    diagnosticSpanFromJudgment(evidence.Span),
			Message: userAssertedAnyEvidence(),
		})
	}
	return out
}

func assignmentJudgmentNilableAccessEvidence(item judgment.Judgment) []diagnostic.Evidence {
	var out []diagnostic.Evidence
	for _, evidence := range item.AssignmentNilableAccessEvidence() {
		if evidence.Detail.SubjectLabel == "" || evidence.Detail.Field == "" {
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

func assignmentJudgmentSourceContributionEvidence(item judgment.Judgment) []diagnostic.Evidence {
	var out []diagnostic.Evidence
	for _, evidence := range item.AssignmentSourceContributionEvidence() {
		out = append(out, diagnostic.Evidence{
			Kind:  diagnostic.EvidenceAbstractFact,
			Trust: diagnosticTrustFromJudgmentTrust(evidence.Trust, diagnostic.TrustProven),
			Span:  diagnosticSpanFromJudgment(evidence.Span),
			Message: diagnosticDisplay{}.ReassignedCallResultFieldEvidence(
				evidence.Detail.ProviderLabel,
				evidence.Detail.SubjectLabel,
				evidence.Detail.FieldType,
			),
		})
	}
	return out
}

func assignmentJudgmentCallInvalidationEvidence(item judgment.Judgment) []diagnostic.Evidence {
	var out []diagnostic.Evidence
	for _, evidence := range item.AssignmentCallInvalidationEvidence() {
		out = append(out, diagnostic.Evidence{
			Kind:  diagnostic.EvidenceAbstractFact,
			Trust: diagnosticTrustFromJudgmentTrust(evidence.Trust, diagnostic.TrustProven),
			Span:  diagnosticSpanFromJudgment(evidence.Span),
			Message: assignmentCallInvalidationMessage(
				evidence.Detail.ProviderLabel,
				evidence.Detail.Field,
				evidence.Detail.SubjectLabel,
			),
		})
	}
	return out
}

func assignmentMissingProofTrust(item judgment.Judgment) diagnostic.TrustKind {
	if assignmentJudgmentHasCallInvalidationEvidence(item) {
		return diagnostic.TrustUnknown
	}
	return diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceMissingProof, missingProofTrustFromJudgment(item.Verdict))
}

func assignmentCallInvalidationMessage(callLabel, invalidatedLabel, readLabel string) string {
	if callLabel == "" {
		callLabel = "call"
	}
	if invalidatedLabel == "" {
		invalidatedLabel = "the source"
	}
	if readLabel == "" {
		readLabel = "assigned value"
	}
	return fmt.Sprintf("%s may change %s, so the read of %s needs a fresh check", callLabel, invalidatedLabel, readLabel)
}

func assignmentNilableAccessMessage(label, access string) string {
	if len(access) > 0 && access[0] == '[' {
		return fmt.Sprintf("%s may be nil before indexing %s", label, access)
	}
	return fmt.Sprintf("%s may be nil before reading %s", label, access)
}

func assignmentJudgmentMissingProofReason(item judgment.Judgment) diagnostic.EvidenceReason {
	if assignmentJudgmentMissingProofIndexedRead(item) {
		return diagnostic.EvidenceReasonIndexReadValidationMissing
	}
	if item.HasEvidence(judgment.EvidencePrecisionBoundary) {
		return diagnostic.EvidenceReasonBoundaryValidationMissing
	}
	if _, ok := assignmentJudgmentCallResultDetail(item); ok {
		return diagnostic.EvidenceReasonBoundaryValidationMissing
	}
	return diagnostic.EvidenceReasonUnspecified
}
