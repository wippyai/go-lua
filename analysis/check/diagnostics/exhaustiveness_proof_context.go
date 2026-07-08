package diagnostics

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

type exhaustivenessPresentation struct {
	Message     string
	Help        string
	Explanation diagnostic.Explanation
	Labels      []diagnostic.Label
}

func (ProofContext) DiscriminatedUnion(item judgment.Judgment, primary diagnostic.Span) (exhaustivenessPresentation, bool) {
	missing := discriminatedUnionCases(item, judgment.EvidenceDetailDiscriminatedUnionMissing)
	if len(missing) == 0 {
		return exhaustivenessPresentation{}, false
	}
	caseWord := pluralize(len(missing), "case", "cases")
	return exhaustivenessPresentation{
		Message:     display.DiscriminatedUnionExhaustivenessMessage(caseWord, discriminantCaseList(missing)),
		Help:        display.DiscriminatedUnionExhaustivenessHelp(),
		Explanation: discriminatedUnionExplanation(item, primary),
		Labels:      []diagnostic.Label{sourceLabel(primary, labelUnionCaseTest)},
	}, true
}

func (ProofContext) Optional(item judgment.Judgment, primary diagnostic.Span) (exhaustivenessPresentation, bool) {
	missing := discriminatedUnionCases(item, judgment.EvidenceDetailOptionalMissing)
	if len(missing) == 0 {
		return exhaustivenessPresentation{}, false
	}
	caseWord := pluralize(len(missing), "case", "cases")
	return exhaustivenessPresentation{
		Message:     display.OptionalExhaustivenessMessage(caseWord, discriminantCaseList(missing)),
		Help:        display.OptionalExhaustivenessHelp(),
		Explanation: optionalExplanation(item, primary),
		Labels:      []diagnostic.Label{sourceLabel(primary, labelOptionalCaseCheck)},
	}, true
}

func (ProofContext) Registration(item judgment.Judgment, primary diagnostic.Span) (exhaustivenessPresentation, bool) {
	missing, _ := registrationMissing(item)
	if len(missing) == 0 {
		return exhaustivenessPresentation{}, false
	}
	registrationWord := pluralize(len(missing), "registration", "registrations")
	return exhaustivenessPresentation{
		Message:     display.RegistrationExhaustivenessMessage(registrationWord, dispatchKeyList(missing)),
		Help:        display.RegistrationExhaustivenessHelp(),
		Explanation: registrationExplanation(item, primary),
		Labels:      registrationLabels(item, primary),
	}, true
}

func (ProofContext) TableDispatch(item judgment.Judgment, lookupSpan diagnostic.Span) (exhaustivenessPresentation, bool) {
	missing, _ := tableDispatchMissing(item)
	if len(missing) == 0 {
		return exhaustivenessPresentation{}, false
	}
	tableSpan := lookupSpan
	if len(item.Spans) > 1 {
		tableSpan = diagnosticSpanFromJudgment(item.Spans[1])
	}
	keyWord := pluralize(len(missing), "key", "keys")
	return exhaustivenessPresentation{
		Message:     display.DispatchTableExhaustivenessMessage(keyWord, dispatchKeyList(missing)),
		Help:        display.DispatchTableExhaustivenessHelp(),
		Explanation: tableDispatchExplanation(item, lookupSpan),
		Labels: []diagnostic.Label{
			sourceLabel(tableSpan, labelDispatchTable),
			sourceLabel(lookupSpan, labelDispatchLookup),
		},
	}, true
}

func (ProofContext) ResultShape(item judgment.Judgment, primary diagnostic.Span) (exhaustivenessPresentation, bool) {
	readPath, requiredCase, ok := resultShapeReadAndCase(item)
	if !ok {
		return exhaustivenessPresentation{}, false
	}
	return exhaustivenessPresentation{
		Message:     display.ResultShapeExhaustivenessMessage(readPath, requiredCase),
		Help:        display.ResultShapeExhaustivenessHelp(),
		Explanation: resultShapeExplanation(item, primary),
		Labels:      []diagnostic.Label{sourceLabel(primary, labelResultFieldRead)},
	}, true
}

func discriminatedUnionExplanation(item judgment.Judgment, fallback diagnostic.Span) diagnostic.Explanation {
	var evidence []diagnostic.Evidence
	for _, itemEvidence := range item.Evidence {
		span := diagnosticSpanFromJudgment(itemEvidence.Span)
		if !span.Valid() {
			span = fallback
		}
		switch itemEvidence.Detail.Kind {
		case judgment.EvidenceDetailDiscriminatedUnionTarget:
			evidence = append(evidence, diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnosticTrustFromJudgmentTrust(itemEvidence.Trust, diagnostic.TrustProven),
				Span:    span,
				Message: selectedDiscriminantPathEvidence(itemEvidence.Detail.SubjectLabel),
			})
		case judgment.EvidenceDetailDiscriminatedUnionPossible:
			evidence = append(evidence, diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnosticTrustFromJudgmentTrust(itemEvidence.Trust, diagnostic.TrustProven),
				Span:    span,
				Message: possibleDiscriminantCasesEvidence(discriminantCaseList(discriminatedUnionCaseListFromKey(itemEvidence.Detail.CaseList))),
			})
		case judgment.EvidenceDetailDiscriminatedUnionHandled:
			evidence = append(evidence, diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnosticTrustFromJudgmentTrust(itemEvidence.Trust, diagnostic.TrustProven),
				Span:    span,
				Message: handledDiscriminantCasesEvidence(discriminantCaseList(discriminatedUnionCaseListFromKey(itemEvidence.Detail.CaseList))),
			})
		case judgment.EvidenceDetailDiscriminatedUnionMissing:
			evidence = append(evidence, diagnostic.Evidence{
				Kind:    diagnostic.EvidenceMissingProof,
				Trust:   diagnosticTrustFromJudgmentTrust(itemEvidence.Trust, diagnostic.TrustUnknown),
				Span:    span,
				Message: missingDiscriminantCasesEvidence(discriminantCaseList(discriminatedUnionCaseListFromKey(itemEvidence.Detail.CaseList))),
			})
		case judgment.EvidenceDetailDiscriminatedUnionNoDefault:
			evidence = append(evidence, diagnostic.Evidence{
				Kind:    diagnostic.EvidenceMissingProof,
				Trust:   diagnosticTrustFromJudgmentTrust(itemEvidence.Trust, diagnostic.TrustUnknown),
				Span:    span,
				Message: missingDiscriminantDefaultEvidence(),
			})
		}
	}
	return diagnostic.NewExplanation(evidence...)
}

func optionalExplanation(item judgment.Judgment, fallback diagnostic.Span) diagnostic.Explanation {
	var evidence []diagnostic.Evidence
	for _, itemEvidence := range item.Evidence {
		span := diagnosticSpanFromJudgment(itemEvidence.Span)
		if !span.Valid() {
			span = fallback
		}
		switch itemEvidence.Detail.Kind {
		case judgment.EvidenceDetailOptionalTarget:
			evidence = append(evidence, diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnosticTrustFromJudgmentTrust(itemEvidence.Trust, diagnostic.TrustProven),
				Span:    span,
				Message: selectedOptionalPathEvidence(itemEvidence.Detail.SubjectLabel),
			})
		case judgment.EvidenceDetailOptionalPossible:
			evidence = append(evidence, diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnosticTrustFromJudgmentTrust(itemEvidence.Trust, diagnostic.TrustProven),
				Span:    span,
				Message: optionalPossibleCasesEvidence(itemEvidence.Detail.SubjectLabel),
			})
		case judgment.EvidenceDetailOptionalConsumed:
			evidence = append(evidence, diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnosticTrustFromJudgmentTrust(itemEvidence.Trust, diagnostic.TrustProven),
				Span:    span,
				Message: optionalConsumedCaseEvidence(itemEvidence.Detail.SubjectLabel),
			})
		case judgment.EvidenceDetailOptionalMissing:
			evidence = append(evidence, diagnostic.Evidence{
				Kind:    diagnostic.EvidenceMissingProof,
				Trust:   diagnosticTrustFromJudgmentTrust(itemEvidence.Trust, diagnostic.TrustUnknown),
				Span:    span,
				Message: optionalMissingCasesEvidence(discriminantCaseList(discriminatedUnionCaseListFromKey(itemEvidence.Detail.CaseList))),
			})
		case judgment.EvidenceDetailOptionalNoDefault:
			evidence = append(evidence, diagnostic.Evidence{
				Kind:    diagnostic.EvidenceMissingProof,
				Trust:   diagnosticTrustFromJudgmentTrust(itemEvidence.Trust, diagnostic.TrustUnknown),
				Span:    span,
				Message: optionalMissingDefaultEvidence(),
			})
		}
	}
	return diagnostic.NewExplanation(evidence...)
}

func registrationLabels(item judgment.Judgment, primary diagnostic.Span) []diagnostic.Label {
	registration := primary
	if len(item.Spans) > 1 {
		registration = diagnosticSpanFromJudgment(item.Spans[1])
	}
	labels := []diagnostic.Label{
		sourceLabel(registration, labelRegistrationCall),
		sourceLabel(primary, labelDispatchCall),
	}
	for _, spanRef := range item.Spans[2:] {
		span := diagnosticSpanFromJudgment(spanRef)
		if !span.Valid() {
			continue
		}
		labels = append(labels, diagnostic.Label{Span: span})
	}
	return labels
}

func registrationExplanation(item judgment.Judgment, fallback diagnostic.Span) diagnostic.Explanation {
	var evidence []diagnostic.Evidence
	for _, itemEvidence := range item.Evidence {
		span := diagnosticSpanFromJudgment(itemEvidence.Span)
		if !span.Valid() {
			span = fallback
		}
		switch itemEvidence.Detail.Kind {
		case judgment.EvidenceDetailRegistrationDispatch:
			evidence = append(evidence, diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnosticTrustFromJudgmentTrust(itemEvidence.Trust, diagnostic.TrustProven),
				Span:    span,
				Message: registrationDispatchEvidence(itemEvidence.Detail.SubjectLabel, itemEvidence.Detail.Field),
			})
		case judgment.EvidenceDetailRegistrationPossible:
			evidence = append(evidence, diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnosticTrustFromJudgmentTrust(itemEvidence.Trust, diagnostic.TrustProven),
				Span:    span,
				Message: possibleDiscriminantCasesEvidence(discriminantCaseList(discriminatedUnionCaseListFromKey(itemEvidence.Detail.CaseList))),
			})
		case judgment.EvidenceDetailRegistrationRegistered:
			evidence = append(evidence, diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnosticTrustFromJudgmentTrust(itemEvidence.Trust, diagnostic.TrustProven),
				Span:    span,
				Message: registeredCasesEvidence(dispatchKeyList(discriminatedUnionCaseListFromKey(itemEvidence.Detail.CaseList))),
			})
		case judgment.EvidenceDetailRegistrationMissing:
			missing, missingFor := registrationMissingListFromKey(itemEvidence.Detail.CaseList)
			evidence = append(evidence, diagnostic.Evidence{
				Kind:    diagnostic.EvidenceMissingProof,
				Trust:   diagnosticTrustFromJudgmentTrust(itemEvidence.Trust, diagnostic.TrustUnknown),
				Span:    span,
				Message: missingRegistrationsEvidence(dispatchMissingKeyCases(missing, missingFor)),
			})
		}
	}
	return diagnostic.NewExplanation(evidence...)
}

func tableDispatchExplanation(item judgment.Judgment, fallback diagnostic.Span) diagnostic.Explanation {
	var evidence []diagnostic.Evidence
	for _, itemEvidence := range item.Evidence {
		span := diagnosticSpanFromJudgment(itemEvidence.Span)
		if !span.Valid() {
			span = fallback
		}
		switch itemEvidence.Detail.Kind {
		case judgment.EvidenceDetailTableDispatchLookup:
			evidence = append(evidence, diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnosticTrustFromJudgmentTrust(itemEvidence.Trust, diagnostic.TrustProven),
				Span:    span,
				Message: dispatchLookupEvidence(itemEvidence.Detail.SubjectLabel, itemEvidence.Detail.Field),
			})
		case judgment.EvidenceDetailTableDispatchPossible:
			evidence = append(evidence, diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnosticTrustFromJudgmentTrust(itemEvidence.Trust, diagnostic.TrustProven),
				Span:    span,
				Message: possibleDiscriminantCasesEvidence(discriminantCaseList(discriminatedUnionCaseListFromKey(itemEvidence.Detail.CaseList))),
			})
		case judgment.EvidenceDetailTableDispatchKeys:
			evidence = append(evidence, diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnosticTrustFromJudgmentTrust(itemEvidence.Trust, diagnostic.TrustProven),
				Span:    span,
				Message: dispatchTableKeysEvidence(dispatchKeyList(discriminatedUnionCaseListFromKey(itemEvidence.Detail.CaseList))),
			})
		case judgment.EvidenceDetailTableDispatchMissing:
			missing, missingFor := registrationMissingListFromKey(itemEvidence.Detail.CaseList)
			evidence = append(evidence, diagnostic.Evidence{
				Kind:    diagnostic.EvidenceMissingProof,
				Trust:   diagnosticTrustFromJudgmentTrust(itemEvidence.Trust, diagnostic.TrustUnknown),
				Span:    span,
				Message: missingDispatchKeysEvidence(dispatchMissingKeyCases(missing, missingFor)),
			})
		}
	}
	return diagnostic.NewExplanation(evidence...)
}

func resultShapeReadAndCase(item judgment.Judgment) (string, string, bool) {
	for _, evidence := range item.Evidence {
		if evidence.Detail.Kind == judgment.EvidenceDetailResultShapeFieldCase {
			return evidence.Detail.SubjectLabel, evidence.Detail.CaseList, evidence.Detail.SubjectLabel != "" && evidence.Detail.CaseList != ""
		}
	}
	return "", "", false
}

func resultShapeExplanation(item judgment.Judgment, fallback diagnostic.Span) diagnostic.Explanation {
	var evidence []diagnostic.Evidence
	for _, itemEvidence := range item.Evidence {
		span := diagnosticSpanFromJudgment(itemEvidence.Span)
		if !span.Valid() {
			span = fallback
		}
		switch itemEvidence.Detail.Kind {
		case judgment.EvidenceDetailResultShapeUnion:
			evidence = append(evidence, diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnosticTrustFromJudgmentTrust(itemEvidence.Trust, diagnostic.TrustProven),
				Span:    span,
				Message: resultShapeUnionEvidence(itemEvidence.Detail.SubjectLabel, itemEvidence.Detail.Field),
			})
		case judgment.EvidenceDetailResultShapeFieldCase:
			evidence = append(evidence, diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnosticTrustFromJudgmentTrust(itemEvidence.Trust, diagnostic.TrustProven),
				Span:    span,
				Message: resultShapeFieldCaseEvidence(itemEvidence.Detail.SubjectLabel, itemEvidence.Detail.CaseList),
			})
		case judgment.EvidenceDetailResultShapeMissingProof:
			evidence = append(evidence, diagnostic.Evidence{
				Kind:    diagnostic.EvidenceMissingProof,
				Trust:   diagnosticTrustFromJudgmentTrust(itemEvidence.Trust, diagnostic.TrustUnknown),
				Span:    span,
				Message: resultShapeMissingProofEvidence(itemEvidence.Detail.CaseList),
			})
		}
	}
	return diagnostic.NewExplanation(evidence...)
}

func discriminatedUnionCases(item judgment.Judgment, kind judgment.EvidenceDetailKind) []string {
	for _, evidence := range item.Evidence {
		if evidence.Detail.Kind == kind {
			return discriminatedUnionCaseListFromKey(evidence.Detail.CaseList)
		}
	}
	return nil
}

func registrationMissing(item judgment.Judgment) ([]string, []string) {
	for _, evidence := range item.Evidence {
		if evidence.Detail.Kind == judgment.EvidenceDetailRegistrationMissing {
			return registrationMissingListFromKey(evidence.Detail.CaseList)
		}
	}
	return nil, nil
}

func tableDispatchMissing(item judgment.Judgment) ([]string, []string) {
	for _, evidence := range item.Evidence {
		if evidence.Detail.Kind == judgment.EvidenceDetailTableDispatchMissing {
			return registrationMissingListFromKey(evidence.Detail.CaseList)
		}
	}
	return nil, nil
}

func registrationMissingListFromKey(key string) ([]string, []string) {
	if key == "" {
		return nil, nil
	}
	parts := discriminatedUnionCaseListFromKey(key)
	missing := make([]string, 0, len(parts))
	missingFor := make([]string, 0, len(parts))
	for _, part := range parts {
		left, right, ok := strings.Cut(part, "\x1e")
		if !ok {
			missing = append(missing, part)
			missingFor = append(missingFor, part)
			continue
		}
		missing = append(missing, left)
		missingFor = append(missingFor, right)
	}
	return missing, missingFor
}

func discriminatedUnionCaseListFromKey(key string) []string {
	if key == "" {
		return nil
	}
	return strings.Split(key, "\x1f")
}
