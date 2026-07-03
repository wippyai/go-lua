package diagnostics

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func renderRegistrationJudgmentWithPolicy(item judgment.Judgment, policy judgment.Policy, mode judgment.StrictnessMode) (diagnostic.Diagnostic, bool) {
	if item.Code != judgment.CodeRegistration || item.Subject.Kind != judgment.SubjectExpression || len(item.Spans) == 0 {
		return diagnostic.Diagnostic{}, false
	}
	severity, ok := diagnosticSeverityForJudgment(item, policy, mode)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	missing, _ := registrationJudgmentMissing(item)
	if len(missing) == 0 {
		return diagnostic.Diagnostic{}, false
	}
	primary := diagnosticSpanFromJudgment(item.Spans[0])
	registrationWord := pluralize(len(missing), "registration", "registrations")
	return diagnostic.New(diagnostic.DiagnosticSpec{
		File:        item.Spans[0].File,
		Span:        primary,
		Code:        CodeDiscriminatedUnionExhaustive,
		Severity:    severity,
		Message:     registrationExhaustivenessMessage(registrationWord, dispatchKeyList(missing)),
		Explanation: registrationJudgmentExplanation(item, primary),
		Help:        registrationExhaustivenessHelp(),
		Labels:      registrationJudgmentLabels(item, primary),
	}), true
}

func registrationJudgmentLabels(item judgment.Judgment, primary diagnostic.Span) []diagnostic.Label {
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

func registrationJudgmentExplanation(item judgment.Judgment, fallback diagnostic.Span) diagnostic.Explanation {
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

func registrationJudgmentMissing(item judgment.Judgment) ([]string, []string) {
	for _, evidence := range item.Evidence {
		if evidence.Detail.Kind == judgment.EvidenceDetailRegistrationMissing {
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
