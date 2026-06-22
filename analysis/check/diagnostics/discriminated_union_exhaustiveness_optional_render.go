package diagnostics

import "github.com/wippyai/go-lua/analysis/diagnostic"

func newOptionalExhaustivenessDiagnostic(evidence optionalEvidence) diagnostic.Diagnostic {
	caseWord := pluralize(len(evidence.missing), "case", "cases")
	missing := discriminantCaseList(evidence.missing)
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Span:        evidence.span,
		Code:        CodeDiscriminatedUnionExhaustive,
		Severity:    diagnostic.SeverityWarning,
		Message:     optionalExhaustivenessMessage(caseWord, missing),
		Explanation: optionalExhaustivenessExplanation(evidence),
		Help:        optionalExhaustivenessHelp(),
		Labels:      []diagnostic.Label{sourceLabel(evidence.span, labelOptionalCaseCheck)},
	})
}

func optionalExhaustivenessExplanation(evidence optionalEvidence) diagnostic.Explanation {
	return diagnostic.NewExplanation(
		diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    evidence.span,
			Message: selectedOptionalPathEvidence(evidence.target),
		},
		diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    evidence.span,
			Message: optionalPossibleCasesEvidence(evidence.target),
		},
		diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    evidence.span,
			Message: optionalConsumedCaseEvidence(evidence.target),
		},
		diagnostic.Evidence{
			Kind:    diagnostic.EvidenceMissingProof,
			Trust:   diagnostic.TrustUnknown,
			Span:    evidence.span,
			Message: optionalMissingCasesEvidence(discriminantCaseList(evidence.missing)),
		},
		diagnostic.Evidence{
			Kind:    diagnostic.EvidenceMissingProof,
			Trust:   diagnostic.TrustUnknown,
			Span:    evidence.span,
			Message: optionalMissingDefaultEvidence(),
		},
	)
}
