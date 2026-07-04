package diagnostics

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func firstDirectCallContractJudgmentPerCall(groups ...[]judgment.Judgment) []judgment.Judgment {
	var out []judgment.Judgment
	seen := make(map[string]struct{})
	for _, group := range groups {
		for _, item := range group {
			key := fmt.Sprintf("%s|%d", item.Subject.FunctionKey, item.Point)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, item)
		}
	}
	return out
}

func renderDirectCallArgumentJudgment(item judgment.Judgment) (diagnostic.Diagnostic, bool) {
	return renderDirectCallArgumentJudgmentWithPolicy(item, judgment.DefaultPolicy(), judgment.StrictnessDefault)
}

func renderDirectCallArgumentJudgmentWithPolicy(item judgment.Judgment, policy judgment.Policy, mode judgment.StrictnessMode) (diagnostic.Diagnostic, bool) {
	if item.Code != judgment.CodeCallArgType || item.Subject.Kind != judgment.SubjectCallArgument || len(item.Spans) == 0 {
		return diagnostic.Diagnostic{}, false
	}
	severity, ok := diagnosticSeverityForJudgment(item, policy, mode)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	if !directCallArgumentJudgmentRenderable(item) {
		return diagnostic.Diagnostic{}, false
	}
	argIndex, ok := callArgumentSubjectIndex(item.Subject.Key)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	argLabel := directCallArgumentJudgmentLabel(item, argIndex)
	wording := directCallArgumentWordingFor(argIndex, argLabel)
	got := item.Actual.ProjectedType
	want := item.Expected.Type
	display := diagnosticDisplay{}
	mayBeNil := directCallArgumentMayBeNil(item)
	precisionBoundary := item.HasEvidence(judgment.EvidencePrecisionBoundary)
	genericConflict, genericParam, genericFunction := directCallArgumentGenericConflict(item)
	message := directCallArgumentJudgmentMessage(display, item, wording, got, want, mayBeNil, precisionBoundary, genericConflict, genericParam, genericFunction)
	help := directCallArgumentJudgmentHelp(display, wording, mayBeNil, precisionBoundary, genericConflict, genericParam)
	if detail, ok := directCallArgumentMissingRequiredMethod(item); ok {
		message = argumentMissingRequiredMethodMessage(wording.Role, want, detail.Field)
	}
	if detail, ok := directCallArgumentMethodTypeMismatch(item); ok {
		message = argumentMethodTypeMismatchMessage(wording.Role, want, detail.Field, detail.ActualType, detail.FieldType)
	}
	span := diagnosticSpanFromJudgment(item.Spans[0])
	return diagnostic.New(diagnostic.DiagnosticSpec{
		File:        item.Spans[0].File,
		Span:        span,
		Code:        CodeDirectCallArgType,
		Severity:    severity,
		Message:     message,
		Explanation: diagnostic.NewExplanation(directCallArgumentJudgmentEvidence(display, item, wording, span)...),
		Help:        help,
		Labels: []diagnostic.Label{{
			File:      item.Spans[0].File,
			Span:      span,
			Message:   labelArgumentValue,
			Placement: diagnostic.LabelPlacementBelow,
		}},
	}), true
}

func directCallArgumentJudgmentLabel(item judgment.Judgment, argIndex int) string {
	if item.Subject.Label != "" {
		return item.Subject.Label
	}
	return fmt.Sprintf("argument %d", argIndex+1)
}

type directCallArgumentWording struct {
	Role        string
	Subject     string
	SourceName  string
	MissingName string
}

func directCallArgumentWordingFor(index int, label string) directCallArgumentWording {
	role := fmt.Sprintf("argument %d", index+1)
	if label == "" {
		label = role
	}
	source := directCallArgumentSourceName(label)
	missing := label
	if source != "" {
		missing = source
	}
	return directCallArgumentWording{
		Role:        role,
		Subject:     label,
		SourceName:  source,
		MissingName: missing,
	}
}

func directCallArgumentSourceName(label string) string {
	open := strings.LastIndex(label, " (")
	if open < 0 || !strings.HasSuffix(label, ")") {
		return ""
	}
	source := strings.TrimSpace(label[open+2 : len(label)-1])
	if source == "" {
		return ""
	}
	return source
}

func directCallArgumentJudgmentMessage(display diagnosticDisplay, item judgment.Judgment, wording directCallArgumentWording, got, want typ.Type, mayBeNil bool, precisionBoundary bool, genericConflict bool, genericParam string, genericFunction string) string {
	if precisionBoundary {
		return fmt.Sprintf("%s comes from any/unknown; no proof shows it is %s", wording.Subject, display.Type(want))
	}
	if got == nil {
		return fmt.Sprintf("cannot prove %s satisfies parameter type %s", wording.Subject, display.Type(want))
	}
	if want == nil {
		return fmt.Sprintf("cannot prove %s satisfies parameter type unknown", wording.Subject)
	}
	if mayBeNil {
		if wording.SourceName != "" {
			return fmt.Sprintf("cannot pass %s as %s because it may be nil", wording.SourceName, wording.Role)
		}
		return fmt.Sprintf("cannot pass %s because it may be nil", wording.Subject)
	}
	if genericConflict {
		labels := directCallArgumentJudgmentEvidenceLabels(item, judgment.EvidenceAbstractFact)
		if len(labels) >= 2 && labels[0] != "" && labels[1] != "" {
			prefix := wording.Subject
			if genericFunction != "" {
				prefix = genericFunction + " " + prefix
			}
			return fmt.Sprintf("%s gives `%s` incompatible types: %s implies %s, but %s implies %s", prefix, genericParamName(genericParam), labels[0], display.Type(want), labels[1], display.Type(got))
		}
		return fmt.Sprintf("%s gives `%s` incompatible types: %s and %s", wording.Subject, genericParamName(genericParam), display.Type(want), display.Type(got))
	}
	gotText := display.Type(got)
	wantText := display.Type(want)
	if gotText == wantText {
		return fmt.Sprintf("cannot prove %s satisfies parameter type %s", wording.Subject, wantText)
	}
	return fmt.Sprintf("%s is %s, not %s", wording.Subject, gotText, wantText)
}

func directCallArgumentJudgmentHelp(display diagnosticDisplay, wording directCallArgumentWording, mayBeNil bool, precisionBoundary bool, genericConflict bool, genericParam string) string {
	if mayBeNil && wording.SourceName != "" {
		return fmt.Sprintf("Guard `%s` with a nil check, provide a default argument value, or change the parameter type to accept nil.", wording.SourceName)
	}
	if genericConflict {
		return directCallArgumentGenericConflictHelp(genericParamName(genericParam))
	}
	if precisionBoundary {
		return display.ArgumentValidationProofHelp(wording.SourceName)
	}
	if wording.SourceName != "" && wording.SourceName != unknownSourceName {
		return fmt.Sprintf("Pass `%s` as a value compatible with the parameter type, or change the callee signature if that argument is valid.", wording.SourceName)
	}
	return fmt.Sprintf("Pass a value for %s that satisfies the parameter type, or change the callee signature if that argument is valid.", wording.Role)
}

func directCallArgumentJudgmentEvidence(display diagnosticDisplay, item judgment.Judgment, wording directCallArgumentWording, primary diagnostic.Span) []diagnostic.Evidence {
	got := item.Actual.ProjectedType
	want := item.Expected.Type
	if genericConflict, genericParam, _ := directCallArgumentGenericConflict(item); genericConflict {
		return directCallArgumentGenericConflictEvidence(display, item, wording, primary, genericParamName(genericParam), got, want)
	}
	expectedLabel := directCallArgumentExpectedLabel(item, wording.Subject)
	missingProof := fmt.Sprintf("no proof on this path shows %s satisfies the parameter type", wording.MissingName)
	if directCallArgumentMayBeNil(item) && wording.SourceName != "" {
		missingProof = missingNonNilGuardHereMessage(wording.SourceName)
	} else if field, ok := directCallArgumentMissingRequiredField(item); ok {
		missingProof = display.MissingRequiredFieldEvidence(field)
	} else if detail, ok := directCallArgumentMissingRequiredMethod(item); ok {
		missingProof = missingRequiredMethodTypeEvidence(want, typ.Method{Name: detail.Field, Type: functionTypeOrNil(detail.FieldType)})
	} else if detail, ok := directCallArgumentMethodTypeMismatch(item); ok {
		missingProof = methodTypeMismatchEvidence(want, detail.Field, detail.ActualType, detail.FieldType)
	} else if directCallArgumentHasCallParamObligation(item) {
		missingProof = fmt.Sprintf("no proof on this path shows %s is %s", directCallArgumentSourceEvidenceLabel(item, wording.MissingName), display.Type(want))
	}
	missingProofReason := diagnostic.EvidenceReasonUnspecified
	if item.HasEvidence(judgment.EvidencePrecisionBoundary) {
		missingProofReason = diagnostic.EvidenceReasonBoundaryValidationMissing
	}
	missingProofTrust := diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceMissingProof, missingProofTrustFromJudgment(item.Verdict))
	if _, ok := directCallArgumentMissingRequiredMethod(item); ok {
		missingProofTrust = diagnostic.TrustUnknown
	}
	if directCallArgumentHasCallParamObligation(item) {
		missingProofTrust = diagnostic.TrustUnknown
	}
	expectedEvidenceKind := diagnostic.EvidenceUserAssertion
	expectedEvidenceTrust := diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceUserAssertion, diagnostic.TrustClaimed)
	if directCallArgumentHasCallParamObligation(item) {
		expectedEvidenceKind = diagnostic.EvidenceAbstractFact
		expectedEvidenceTrust = diagnostic.TrustProven
	}
	evidence := []diagnostic.Evidence{
		{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceAbstractFact, diagnostic.TrustUnknown),
			Span:    primary,
			Message: display.SourceTypeEvidence(directCallArgumentSourceEvidenceLabel(item, wording.Subject), got),
		},
		{
			Kind:    expectedEvidenceKind,
			Trust:   expectedEvidenceTrust,
			Span:    directCallArgumentJudgmentEvidenceSpan(item, judgment.EvidenceUserAssertion),
			Message: directCallArgumentExpectedEvidenceMessage(display, item, expectedLabel, want),
		},
	}
	evidence = append(evidence, directCallArgumentUserAssertionEvidence(item)...)
	if item.HasEvidence(judgment.EvidencePrecisionBoundary) {
		sourceName := wording.Subject
		if wording.SourceName != "" {
			sourceName = wording.SourceName
		}
		evidence = append(evidence, diagnostic.Evidence{
			Kind:    diagnostic.EvidencePrecisionBoundary,
			Trust:   diagnostic.TrustUnknown,
			Reason:  diagnostic.EvidenceReasonExplicitBoundaryValidation,
			Span:    primary,
			Message: fmt.Sprintf("%s comes from any/unknown", sourceName),
		})
	}
	evidence = append(evidence, diagnostic.Evidence{
		Kind:    diagnostic.EvidenceMissingProof,
		Trust:   missingProofTrust,
		Reason:  missingProofReason,
		Span:    primary,
		Message: missingProof,
	})
	return evidence
}

func directCallArgumentUserAssertionEvidence(item judgment.Judgment) []diagnostic.Evidence {
	var out []diagnostic.Evidence
	for _, evidence := range item.Evidence {
		if evidence.Kind != judgment.EvidenceUserAssertion ||
			evidence.Detail.Kind != judgment.EvidenceDetailUserAssertedAny {
			continue
		}
		out = append(out, diagnostic.Evidence{
			Kind:    diagnostic.EvidenceUserAssertion,
			Trust:   diagnosticTrustFromJudgmentTrust(evidence.Trust, diagnostic.TrustClaimed),
			Reason:  diagnostic.EvidenceReasonUserAssertedAny,
			Span:    diagnosticSpanFromJudgment(evidence.Span),
			Message: userAssertedAnyEvidence(),
		})
	}
	return out
}

func directCallArgumentSourceEvidenceLabel(item judgment.Judgment, fallback string) string {
	for _, evidence := range item.Evidence {
		if evidence.Kind == judgment.EvidenceUserAssertion &&
			evidence.Detail.Kind == judgment.EvidenceDetailCallParamObligation &&
			evidence.Detail.ProviderLabel != "" &&
			evidence.Detail.SubjectLabel != "" {
			return evidence.Detail.SubjectLabel
		}
	}
	return fallback
}

func directCallArgumentExpectedEvidenceMessage(display diagnosticDisplay, item judgment.Judgment, fallback string, want typ.Type) string {
	for _, evidence := range item.Evidence {
		if evidence.Kind != judgment.EvidenceUserAssertion || evidence.Detail.Kind != judgment.EvidenceDetailCallParamObligation {
			continue
		}
		detail := evidence.Detail
		if detail.FunctionName == "" || detail.SubjectLabel == "" {
			break
		}
		if detail.ProviderLabel == "" || detail.MemberParam <= 0 {
			return display.CallParamObligationEvidence(detail.FunctionName, detail.SubjectLabel, want)
		}
		return display.MemberCallParamObligationEvidence(detail.FunctionName, detail.SubjectLabel, detail.ProviderLabel, detail.MemberParam, want)
	}
	return fmt.Sprintf("%s expects %s", fallback, display.Type(want))
}

func directCallArgumentHasCallParamObligation(item judgment.Judgment) bool {
	for _, evidence := range item.Evidence {
		if evidence.Detail.Kind == judgment.EvidenceDetailCallParamObligation {
			return true
		}
	}
	return false
}

func directCallArgumentGenericConflictEvidence(display diagnosticDisplay, item judgment.Judgment, wording directCallArgumentWording, primary diagnostic.Span, paramName string, got, want typ.Type) []diagnostic.Evidence {
	expectedLabel := directCallArgumentExpectedLabel(item, wording.Subject)
	contributionSpans := directCallArgumentJudgmentEvidenceSpans(item, judgment.EvidenceAbstractFact)
	contributionLabels := directCallArgumentJudgmentEvidenceLabels(item, judgment.EvidenceAbstractFact)
	firstSpan := primary
	secondSpan := primary
	if len(contributionSpans) > 0 {
		firstSpan = contributionSpans[0]
	}
	if len(contributionSpans) > 1 {
		secondSpan = contributionSpans[1]
	}
	firstSubject := wording.Subject
	secondSubject := wording.Subject
	if len(contributionLabels) > 0 && contributionLabels[0] != "" {
		firstSubject = contributionLabels[0]
	}
	if len(contributionLabels) > 1 && contributionLabels[1] != "" {
		secondSubject = contributionLabels[1]
	}
	return []diagnostic.Evidence{
		{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    firstSpan,
			Message: fmt.Sprintf("%s contributes %s for `%s`", firstSubject, display.Type(want), paramName),
		},
		{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    secondSpan,
			Message: fmt.Sprintf("%s also contributes %s for `%s`", secondSubject, display.Type(got), paramName),
		},
		{
			Kind:    diagnostic.EvidenceUserAssertion,
			Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceUserAssertion, diagnostic.TrustClaimed),
			Span:    directCallArgumentJudgmentEvidenceSpan(item, judgment.EvidenceUserAssertion),
			Message: fmt.Sprintf("%s expects one consistent type for `%s`", expectedLabel, paramName),
		},
		{
			Kind:    diagnostic.EvidenceMissingProof,
			Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceMissingProof, missingProofTrustFromJudgment(item.Verdict)),
			Span:    primary,
			Message: fmt.Sprintf("no single type for `%s` satisfies every contribution in %s", paramName, wording.MissingName),
		},
	}
}

func directCallArgumentJudgmentEvidenceLabels(item judgment.Judgment, kind judgment.EvidenceKind) []string {
	var out []string
	for _, evidence := range item.Evidence {
		if evidence.Kind == kind {
			out = append(out, evidence.Detail.SubjectLabel)
		}
	}
	return out
}

func directCallArgumentGenericConflictHelp(paramName string) string {
	return fmt.Sprintf("Make each use of `%s` in this argument agree on the same type, or split the callee signature into separate type parameters if those values are intentionally different.", paramName)
}

func directCallArgumentJudgmentEvidenceSpan(item judgment.Judgment, kind judgment.EvidenceKind) diagnostic.Span {
	for _, evidence := range item.Evidence {
		if evidence.Kind != kind || evidence.Span.StartLine == 0 || evidence.Span.StartCol == 0 {
			continue
		}
		return diagnosticSpanFromJudgment(evidence.Span)
	}
	return diagnostic.Span{}
}

func directCallArgumentJudgmentEvidenceSpans(item judgment.Judgment, kind judgment.EvidenceKind) []diagnostic.Span {
	var spans []diagnostic.Span
	for _, evidence := range item.Evidence {
		if evidence.Kind != kind || evidence.Span.StartLine == 0 || evidence.Span.StartCol == 0 {
			continue
		}
		spans = append(spans, diagnosticSpanFromJudgment(evidence.Span))
	}
	return spans
}

func directCallArgumentMissingRequiredField(item judgment.Judgment) (string, bool) {
	for _, evidence := range item.Evidence {
		if evidence.Kind != judgment.EvidenceMissingProof {
			continue
		}
		if evidence.Detail.Kind == judgment.EvidenceDetailMissingRequiredField && evidence.Detail.Field != "" {
			return evidence.Detail.Field, true
		}
	}
	return "", false
}

func directCallArgumentMissingRequiredMethod(item judgment.Judgment) (judgment.EvidenceDetail, bool) {
	for _, evidence := range item.Evidence {
		if evidence.Kind != judgment.EvidenceMissingProof {
			continue
		}
		if evidence.Detail.Kind == judgment.EvidenceDetailMissingRequiredMethod && evidence.Detail.Field != "" {
			return evidence.Detail, true
		}
	}
	return judgment.EvidenceDetail{}, false
}

func directCallArgumentMethodTypeMismatch(item judgment.Judgment) (judgment.EvidenceDetail, bool) {
	for _, evidence := range item.Evidence {
		if evidence.Kind != judgment.EvidenceMissingProof {
			continue
		}
		if evidence.Detail.Kind == judgment.EvidenceDetailMethodTypeMismatch && evidence.Detail.Field != "" {
			return evidence.Detail, true
		}
	}
	return judgment.EvidenceDetail{}, false
}

func directCallArgumentMayBeNil(item judgment.Judgment) bool {
	for _, evidence := range item.Evidence {
		if evidence.Kind == judgment.EvidenceMissingProof && evidence.Detail.Kind == judgment.EvidenceDetailMayBeNil {
			return true
		}
	}
	return false
}

func directCallArgumentGenericConflict(item judgment.Judgment) (bool, string, string) {
	for _, evidence := range item.Evidence {
		if evidence.Kind == judgment.EvidenceMissingProof && evidence.Detail.Kind == judgment.EvidenceDetailGenericConflict {
			return true, evidence.Detail.Param, evidence.Detail.FunctionName
		}
	}
	return false, "", ""
}

func genericParamName(name string) string {
	if name == "" {
		return "type parameter"
	}
	return name
}

func directCallArgumentExpectedLabel(item judgment.Judgment, fallback string) string {
	if item.Expected.Label != "" {
		return item.Expected.Label
	}
	return fallback
}

func directCallArgumentJudgmentRenderable(item judgment.Judgment) bool {
	return item.Verdict == judgment.VerdictRefuted || item.HasEvidence(judgment.EvidencePrecisionBoundary)
}

func callArgumentSubjectIndex(key string) (int, bool) {
	parts := strings.Split(key, ":")
	for i := 0; i+1 < len(parts); i++ {
		if parts[i] != "arg" {
			continue
		}
		n, err := strconv.Atoi(parts[i+1])
		if err == nil && n >= 0 {
			return n, true
		}
	}
	return 0, false
}
