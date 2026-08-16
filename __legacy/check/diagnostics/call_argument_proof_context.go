package diagnostics

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

type callArgumentPresentation struct {
	Message  string
	Help     string
	Evidence []diagnostic.Evidence
	Labels   []diagnostic.Label
}

func (ProofContext) DirectCallArgument(item judgment.Judgment, primary diagnostic.Span) (callArgumentPresentation, bool) {
	proof := item.CallArgumentProof()
	if !proof.Renderable(item.Verdict) {
		return callArgumentPresentation{}, false
	}
	argIndex, ok := callArgumentSubjectIndex(item.Subject.Key)
	if !ok {
		return callArgumentPresentation{}, false
	}
	argLabel := directCallArgumentJudgmentLabel(item, argIndex)
	wording := directCallArgumentWordingFor(argIndex, argLabel)
	got := item.Actual.ProjectedType
	want := item.Expected.Type
	display := diagnosticDisplay{}
	message := directCallArgumentJudgmentMessage(display, item, proof, wording, got, want)
	help := directCallArgumentJudgmentHelp(display, proof, wording)
	if proof.MissingRequiredMethod {
		detail := proof.MissingRequiredMethodDetail
		message = display.ArgumentMissingRequiredMethodMessage(wording.Role, want, detail.Field)
	}
	if proof.MethodTypeMismatch {
		detail := proof.MethodTypeMismatchDetail
		message = display.ArgumentMethodTypeMismatchMessage(wording.Role, want, detail.Field, detail.ActualType, detail.FieldType)
	}
	return callArgumentPresentation{
		Message:  message,
		Help:     help,
		Evidence: directCallArgumentJudgmentEvidence(display, item, proof, wording, primary),
		Labels: []diagnostic.Label{{
			File:      item.Spans[0].File,
			Span:      primary,
			Message:   labelArgumentValue,
			Placement: diagnostic.LabelPlacementBelow,
		}},
	}, true
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

func directCallArgumentJudgmentMessage(display diagnosticDisplay, item judgment.Judgment, proof judgment.CallArgumentProofSummary, wording directCallArgumentWording, got, want typ.Type) string {
	if callArgumentNilabilityShouldLead(proof, wording) {
		return fmt.Sprintf("cannot pass %s as %s because it may be nil", wording.SourceName, wording.Role)
	}
	if proof.PrecisionBoundary {
		return fmt.Sprintf("%s comes from any/unknown; no proof shows it is %s", wording.Subject, display.Type(want))
	}
	if got == nil {
		return fmt.Sprintf("cannot prove %s satisfies parameter type %s", wording.Subject, display.Type(want))
	}
	if want == nil {
		return fmt.Sprintf("cannot prove %s satisfies parameter type unknown", wording.Subject)
	}
	if proof.GenericConflict {
		labels := directCallArgumentJudgmentEvidenceLabels(item, judgment.EvidenceAbstractFact)
		if len(labels) >= 2 && labels[0] != "" && labels[1] != "" {
			prefix := wording.Subject
			if proof.GenericFunction != "" {
				prefix = proof.GenericFunction + " " + prefix
			}
			return fmt.Sprintf("%s gives `%s` incompatible types: %s implies %s, but %s implies %s", prefix, genericParamName(proof.GenericParam), labels[0], display.Type(want), labels[1], display.Type(got))
		}
		return fmt.Sprintf("%s gives `%s` incompatible types: %s and %s", wording.Subject, genericParamName(proof.GenericParam), display.Type(want), display.Type(got))
	}
	gotText := display.Type(got)
	wantText := display.Type(want)
	if tupleText, ok := directCallArgumentTupleValueText(display, got, want); ok {
		return fmt.Sprintf("%s is %s, not %s", wording.Subject, tupleText, wantText)
	}
	if gotText == wantText {
		return fmt.Sprintf("cannot prove %s satisfies parameter type %s", wording.Subject, wantText)
	}
	return fmt.Sprintf("%s is %s, not %s", wording.Subject, gotText, wantText)
}

func directCallArgumentTupleValueText(display diagnosticDisplay, got, want typ.Type) (string, bool) {
	tuple, ok := got.(*typ.Tuple)
	if !ok || len(tuple.Elements) == 0 || want == nil {
		return "", false
	}
	for _, elem := range tuple.Elements {
		if !typ.TypeEquals(elem, want) {
			return "", false
		}
	}
	if len(tuple.Elements) == 1 {
		return "a tuple/table value containing " + display.Type(tuple.Elements[0]), true
	}
	return "a tuple/table value containing " + display.Type(tuple.Elements[0]) + " values", true
}

func directCallArgumentJudgmentHelp(display diagnosticDisplay, proof judgment.CallArgumentProofSummary, wording directCallArgumentWording) string {
	if proof.GenericConflict {
		return directCallArgumentGenericConflictHelp(genericParamName(proof.GenericParam))
	}
	if callArgumentNilabilityShouldLead(proof, wording) {
		return fmt.Sprintf("Guard `%s` with a nil check, provide a default argument value, or change the parameter type to accept nil.", wording.SourceName)
	}
	if proof.PrecisionBoundary {
		return display.ArgumentValidationProofHelp(wording.SourceName)
	}
	if wording.SourceName != "" && wording.SourceName != unknownSourceName {
		return fmt.Sprintf("Pass `%s` as a value compatible with the parameter type, or change the callee signature if that argument is valid.", wording.SourceName)
	}
	return fmt.Sprintf("Pass a value for %s that satisfies the parameter type, or change the callee signature if that argument is valid.", wording.Role)
}

func directCallArgumentJudgmentEvidence(display diagnosticDisplay, item judgment.Judgment, proof judgment.CallArgumentProofSummary, wording directCallArgumentWording, primary diagnostic.Span) []diagnostic.Evidence {
	got := item.Actual.ProjectedType
	want := item.Expected.Type
	if proof.PrecisionBoundary && (got == nil || typ.IsAny(got) || typ.IsUnknown(got)) {
		got = typ.Any
	}
	if proof.GenericConflict {
		return directCallArgumentGenericConflictEvidence(display, item, wording, primary, genericParamName(proof.GenericParam), got, want)
	}
	expectedLabel := directCallArgumentExpectedLabel(item, wording.Subject)
	missingProof := fmt.Sprintf("no proof on this path shows %s satisfies the parameter type", wording.MissingName)
	if proof.CallParamObligation {
		missingProof = fmt.Sprintf("no proof on this path shows %s is %s", directCallArgumentSourceEvidenceLabel(proof, wording.MissingName), display.Type(want))
	} else if callArgumentNilabilityShouldLead(proof, wording) {
		missingProof = display.MissingNonNilGuardHereMessage(wording.SourceName)
	} else if proof.PrecisionBoundary {
		// Precision-boundary evidence is the primary reason when an explicit
		// any/unknown value reaches a contract. Nilability may also be visible,
		// but it is not a validation proof for the parameter type.
	} else if proof.MissingRequiredField != "" {
		missingProof = display.MissingRequiredFieldEvidence(proof.MissingRequiredField)
	} else if proof.MissingRequiredMethod {
		detail := proof.MissingRequiredMethodDetail
		missingProof = display.MissingRequiredMethodTypeEvidence(want, typ.Method{Name: detail.Field, Type: functionTypeOrNil(detail.FieldType)})
	} else if proof.MethodTypeMismatch {
		detail := proof.MethodTypeMismatchDetail
		missingProof = display.MethodTypeMismatchEvidence(want, detail.Field, detail.ActualType, detail.FieldType)
	}
	missingProofReason := diagnostic.EvidenceReasonUnspecified
	if proof.PrecisionBoundary && !callArgumentNilabilityShouldLead(proof, wording) {
		missingProofReason = diagnostic.EvidenceReasonBoundaryValidationMissing
	}
	missingProofTrust := diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceMissingProof, missingProofTrustFromJudgment(item.Verdict))
	if proof.MissingRequiredMethod {
		missingProofTrust = diagnostic.TrustUnknown
	}
	if proof.CallParamObligation {
		missingProofTrust = diagnostic.TrustUnknown
	}
	expectedEvidenceKind := diagnostic.EvidenceUserAssertion
	expectedEvidenceTrust := diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceUserAssertion, diagnostic.TrustClaimed)
	if proof.CallParamObligation {
		expectedEvidenceKind = diagnostic.EvidenceAbstractFact
		expectedEvidenceTrust = diagnostic.TrustProven
	}
	evidence := []diagnostic.Evidence{
		{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceAbstractFact, diagnostic.TrustUnknown),
			Span:    primary,
			Message: display.SourceTypeEvidence(directCallArgumentSourceEvidenceLabel(proof, wording.Subject), got),
		},
		{
			Kind:    expectedEvidenceKind,
			Trust:   expectedEvidenceTrust,
			Span:    directCallArgumentJudgmentEvidenceSpan(item, judgment.EvidenceUserAssertion),
			Message: directCallArgumentExpectedEvidenceMessage(display, proof, expectedLabel, want),
		},
	}
	evidence = append(evidence, directCallArgumentUserAssertionEvidence(item)...)
	if proof.PrecisionBoundary {
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
	if callArgumentNilabilityShouldLead(proof, wording) {
		evidence = append(evidence, nilabilityProvenanceEvidence(item, wording.SourceName, got, primary)...)
	}
	return evidence
}

func callArgumentNilabilityShouldLead(proof judgment.CallArgumentProofSummary, wording directCallArgumentWording) bool {
	return proof.MayBeNil &&
		wording.SourceName != "" &&
		(!proof.PrecisionBoundary || proof.MayBeNilFromExpandedSource)
}

func directCallArgumentUserAssertionEvidence(item judgment.Judgment) []diagnostic.Evidence {
	var out []diagnostic.Evidence
	for _, evidence := range item.EvidenceKindDetails(judgment.EvidenceUserAssertion, judgment.EvidenceDetailUserAssertedAny) {
		out = append(out, diagnostic.Evidence{
			Kind:    diagnostic.EvidenceUserAssertion,
			Trust:   diagnosticTrustFromJudgmentTrust(evidence.Trust, diagnostic.TrustClaimed),
			Reason:  diagnostic.EvidenceReasonUserAssertedAny,
			Span:    diagnosticSpanFromJudgment(evidence.Span),
			Message: display.UserAssertedAnyEvidence(),
		})
	}
	return out
}

func directCallArgumentSourceEvidenceLabel(proof judgment.CallArgumentProofSummary, fallback string) string {
	if proof.CallParamDetail.ProviderLabel != "" && proof.CallParamSubjectLabel != "" {
		return proof.CallParamSubjectLabel
	}
	return fallback
}

func directCallArgumentExpectedEvidenceMessage(display diagnosticDisplay, proof judgment.CallArgumentProofSummary, fallback string, want typ.Type) string {
	if proof.CallParamObligation {
		detail := proof.CallParamDetail
		switch {
		case detail.FunctionName == "" || detail.SubjectLabel == "":
		case detail.ProviderLabel == "" || detail.MemberParam <= 0:
			return display.CallParamObligationEvidence(detail.FunctionName, detail.SubjectLabel, want)
		default:
			return display.MemberCallParamObligationEvidence(detail.FunctionName, detail.SubjectLabel, detail.ProviderLabel, detail.MemberParam, want)
		}
	}
	return fmt.Sprintf("%s expects %s", fallback, display.Type(want))
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
	for _, evidence := range item.EvidenceOfKind(kind) {
		out = append(out, evidence.Detail.SubjectLabel)
	}
	return out
}

func directCallArgumentGenericConflictHelp(paramName string) string {
	return fmt.Sprintf("Make each use of `%s` in this argument agree on the same type, or split the callee signature into separate type parameters if those values are intentionally different.", paramName)
}

func directCallArgumentJudgmentEvidenceSpan(item judgment.Judgment, kind judgment.EvidenceKind) diagnostic.Span {
	for _, evidence := range item.EvidenceOfKind(kind) {
		if evidence.Kind != kind || evidence.Span.StartLine == 0 || evidence.Span.StartCol == 0 {
			continue
		}
		return diagnosticSpanFromJudgment(evidence.Span)
	}
	return diagnostic.Span{}
}

func directCallArgumentJudgmentEvidenceSpans(item judgment.Judgment, kind judgment.EvidenceKind) []diagnostic.Span {
	var spans []diagnostic.Span
	for _, evidence := range item.EvidenceOfKind(kind) {
		if evidence.Kind != kind || evidence.Span.StartLine == 0 || evidence.Span.StartCol == 0 {
			continue
		}
		spans = append(spans, diagnosticSpanFromJudgment(evidence.Span))
	}
	return spans
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
