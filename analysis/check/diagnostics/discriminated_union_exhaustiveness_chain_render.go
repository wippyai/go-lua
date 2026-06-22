package diagnostics

import "github.com/wippyai/go-lua/analysis/diagnostic"

func newDiscriminatedUnionExhaustivenessDiagnostic(span diagnostic.Span, evidence discriminatedUnionEvidence) diagnostic.Diagnostic {
	caseWord := pluralize(len(evidence.missing), "case", "cases")
	missing := discriminantCaseList(evidence.missing)
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Span:        span,
		Code:        CodeDiscriminatedUnionExhaustive,
		Severity:    diagnostic.SeverityWarning,
		Message:     discriminatedUnionExhaustivenessMessage(caseWord, missing),
		Explanation: discriminatedUnionExhaustivenessExplanation(span, evidence),
		Help:        discriminatedUnionExhaustivenessHelp(),
		Labels:      []diagnostic.Label{sourceLabel(span, labelUnionCaseTest)},
	})
}

func discriminatedUnionExhaustivenessExplanation(span diagnostic.Span, evidence discriminatedUnionEvidence) diagnostic.Explanation {
	items := []diagnostic.Evidence{
		{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    span,
			Message: selectedDiscriminantPathEvidence(evidence.target),
		},
		{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    span,
			Message: possibleDiscriminantCasesEvidence(discriminantCaseList(evidence.possible)),
		},
	}
	if len(evidence.handled) > 0 {
		items = append(items, diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    span,
			Message: handledDiscriminantCasesEvidence(discriminantCaseList(evidence.handled)),
		})
	}
	items = append(items,
		diagnostic.Evidence{
			Kind:    diagnostic.EvidenceMissingProof,
			Trust:   diagnostic.TrustUnknown,
			Span:    span,
			Message: missingDiscriminantCasesEvidence(discriminantCaseList(evidence.missing)),
		},
		diagnostic.Evidence{
			Kind:    diagnostic.EvidenceMissingProof,
			Trust:   diagnostic.TrustUnknown,
			Span:    span,
			Message: missingDiscriminantDefaultEvidence(),
		},
	)
	return diagnostic.NewExplanation(items...)
}
