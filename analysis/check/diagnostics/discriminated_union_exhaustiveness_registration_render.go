package diagnostics

import "github.com/wippyai/go-lua/analysis/diagnostic"

func newRegistrationExhaustivenessDiagnostic(evidence registrationEvidence) diagnostic.Diagnostic {
	registrationWord := pluralize(len(evidence.missing), "registration", "registrations")
	missing := dispatchKeyList(evidence.missing)
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Span:        evidence.dispatchSpan,
		Code:        CodeDiscriminatedUnionExhaustive,
		Severity:    diagnostic.SeverityWarning,
		Message:     registrationExhaustivenessMessage(registrationWord, missing),
		Explanation: registrationExhaustivenessExplanation(evidence),
		Help:        registrationExhaustivenessHelp(),
		Labels: []diagnostic.Label{
			sourceLabel(evidence.registrationSpan, labelRegistrationCall),
			sourceLabel(evidence.dispatchSpan, labelDispatchCall),
		},
	})
}

func registrationExhaustivenessExplanation(evidence registrationEvidence) diagnostic.Explanation {
	return diagnostic.NewExplanation(
		diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    evidence.dispatchSpan,
			Message: registrationDispatchEvidence(evidence.registry, evidence.target),
		},
		diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    evidence.dispatchSpan,
			Message: possibleDiscriminantCasesEvidence(discriminantCaseList(evidence.possible)),
		},
		diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    evidence.registrationSpan,
			Message: registeredCasesEvidence(dispatchKeyList(evidence.registered)),
		},
		diagnostic.Evidence{
			Kind:    diagnostic.EvidenceMissingProof,
			Trust:   diagnostic.TrustUnknown,
			Span:    evidence.dispatchSpan,
			Message: missingRegistrationsEvidence(dispatchMissingKeyCases(evidence.missing, evidence.missingFor)),
		},
	)
}
