package diagnostics

import "github.com/wippyai/go-lua/analysis/diagnostic"

func newDispatchTableExhaustivenessDiagnostic(evidence dispatchTableEvidence) diagnostic.Diagnostic {
	keyWord := pluralize(len(evidence.missing), "key", "keys")
	missing := dispatchKeyList(evidence.missing)
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Span:        evidence.lookupSpan,
		Code:        CodeDiscriminatedUnionExhaustive,
		Severity:    diagnostic.SeverityWarning,
		Message:     dispatchTableExhaustivenessMessage(keyWord, missing),
		Explanation: dispatchTableExhaustivenessExplanation(evidence),
		Help:        dispatchTableExhaustivenessHelp(),
		Labels: []diagnostic.Label{
			sourceLabel(evidence.tableSpan, labelDispatchTable),
			sourceLabel(evidence.lookupSpan, labelDispatchLookup),
		},
	})
}

func dispatchTableExhaustivenessExplanation(evidence dispatchTableEvidence) diagnostic.Explanation {
	return diagnostic.NewExplanation(
		diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    evidence.lookupSpan,
			Message: dispatchLookupEvidence(evidence.table, evidence.target),
		},
		diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    evidence.lookupSpan,
			Message: possibleDiscriminantCasesEvidence(discriminantCaseList(evidence.possible)),
		},
		diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    evidence.tableSpan,
			Message: dispatchTableKeysEvidence(dispatchKeyList(evidence.keys)),
		},
		diagnostic.Evidence{
			Kind:    diagnostic.EvidenceMissingProof,
			Trust:   diagnostic.TrustUnknown,
			Span:    evidence.lookupSpan,
			Message: missingDispatchKeysEvidence(dispatchMissingKeyCases(evidence.missing, evidence.missingFor)),
		},
	)
}
