package diagnostics

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func dispatchKeyName(table, key string) string {
	if identifierName(key) {
		return table + "." + key
	}
	return table + "[" + formatType(typ.LiteralString(key)) + "]"
}

func registrationCaseName(registry, key string) string {
	return dispatchKeyName(registry, key)
}

func identifierName(s string) bool {
	if s == "" {
		return false
	}
	if !((s[0] >= 'A' && s[0] <= 'Z') || (s[0] >= 'a' && s[0] <= 'z') || s[0] == '_') {
		return false
	}
	for i := 1; i < len(s); i++ {
		ch := s[i]
		if !((ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '_') {
			return false
		}
	}
	return true
}

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

func newResultShapeExhaustivenessDiagnostic(evidence resultShapeEvidence) diagnostic.Diagnostic {
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Span:        evidence.readSpan,
		Code:        CodeDiscriminatedUnionExhaustive,
		Severity:    diagnostic.SeverityWarning,
		Message:     resultShapeExhaustivenessMessage(evidence.readPath, evidence.requiredCase),
		Explanation: resultShapeExhaustivenessExplanation(evidence),
		Help:        resultShapeExhaustivenessHelp(),
		Labels:      []diagnostic.Label{sourceLabel(evidence.readSpan, labelResultFieldRead)},
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

func resultShapeExhaustivenessExplanation(evidence resultShapeEvidence) diagnostic.Explanation {
	return diagnostic.NewExplanation(
		diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    evidence.readSpan,
			Message: resultShapeUnionEvidence(evidence.receiver, evidence.discriminant),
		},
		diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    evidence.readSpan,
			Message: resultShapeFieldCaseEvidence(evidence.readPath, evidence.requiredCase),
		},
		diagnostic.Evidence{
			Kind:    diagnostic.EvidenceMissingProof,
			Trust:   diagnostic.TrustUnknown,
			Span:    evidence.readSpan,
			Message: resultShapeMissingProofEvidence(evidence.requiredCase),
		},
	)
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

func discriminantCaseList(cases []string) string {
	return strings.Join(codeNames(cases), ", ")
}

func dispatchKeyList(keys []string) string {
	if len(keys) == 0 {
		return "none"
	}
	return strings.Join(codeNames(keys), ", ")
}

func dispatchMissingKeyCases(keys []string, cases []string) string {
	if len(keys) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(keys))
	for i, key := range keys {
		if i < len(cases) && cases[i] != "" {
			parts = append(parts, codeName(key)+" for "+codeName(cases[i]))
		} else {
			parts = append(parts, codeName(key))
		}
	}
	return strings.Join(parts, ", ")
}
