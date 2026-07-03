package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func renderTableDispatchJudgmentWithPolicy(item judgment.Judgment, policy judgment.Policy, mode judgment.StrictnessMode) (diagnostic.Diagnostic, bool) {
	if item.Code != judgment.CodeTableDispatch || item.Subject.Kind != judgment.SubjectExpression || len(item.Spans) == 0 {
		return diagnostic.Diagnostic{}, false
	}
	severity, ok := diagnosticSeverityForJudgment(item, policy, mode)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	missing, _ := tableDispatchJudgmentMissing(item)
	if len(missing) == 0 {
		return diagnostic.Diagnostic{}, false
	}
	lookupSpan := diagnosticSpanFromJudgment(item.Spans[0])
	tableSpan := lookupSpan
	if len(item.Spans) > 1 {
		tableSpan = diagnosticSpanFromJudgment(item.Spans[1])
	}
	keyWord := pluralize(len(missing), "key", "keys")
	return diagnostic.New(diagnostic.DiagnosticSpec{
		File:        item.Spans[0].File,
		Span:        lookupSpan,
		Code:        CodeDiscriminatedUnionExhaustive,
		Severity:    severity,
		Message:     dispatchTableExhaustivenessMessage(keyWord, dispatchKeyList(missing)),
		Explanation: tableDispatchJudgmentExplanation(item, lookupSpan),
		Help:        dispatchTableExhaustivenessHelp(),
		Labels: []diagnostic.Label{
			sourceLabel(tableSpan, labelDispatchTable),
			sourceLabel(lookupSpan, labelDispatchLookup),
		},
	}), true
}

func tableDispatchJudgmentExplanation(item judgment.Judgment, fallback diagnostic.Span) diagnostic.Explanation {
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

func tableDispatchJudgmentMissing(item judgment.Judgment) ([]string, []string) {
	for _, evidence := range item.Evidence {
		if evidence.Detail.Kind == judgment.EvidenceDetailTableDispatchMissing {
			return registrationMissingListFromKey(evidence.Detail.CaseList)
		}
	}
	return nil, nil
}
