package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func renderOptionalJudgmentWithPolicy(item judgment.Judgment, policy judgment.Policy, mode judgment.StrictnessMode) (diagnostic.Diagnostic, bool) {
	if item.Code != judgment.CodeOptional || item.Subject.Kind != judgment.SubjectExpression || len(item.Spans) == 0 {
		return diagnostic.Diagnostic{}, false
	}
	severity, ok := diagnosticSeverityForJudgment(item, policy, mode)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	missing := discriminatedUnionJudgmentCases(item, judgment.EvidenceDetailOptionalMissing)
	if len(missing) == 0 {
		return diagnostic.Diagnostic{}, false
	}
	span := diagnosticSpanFromJudgment(item.Spans[0])
	caseWord := pluralize(len(missing), "case", "cases")
	return diagnostic.New(diagnostic.DiagnosticSpec{
		File:        item.Spans[0].File,
		Span:        span,
		Code:        CodeDiscriminatedUnionExhaustive,
		Severity:    severity,
		Message:     optionalExhaustivenessMessage(caseWord, discriminantCaseList(missing)),
		Explanation: optionalJudgmentExplanation(item, span),
		Help:        optionalExhaustivenessHelp(),
		Labels:      []diagnostic.Label{sourceLabel(span, labelOptionalCaseCheck)},
	}), true
}

func optionalJudgmentExplanation(item judgment.Judgment, fallback diagnostic.Span) diagnostic.Explanation {
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
