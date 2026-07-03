package diagnostics

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func renderDiscriminatedUnionJudgmentWithPolicy(item judgment.Judgment, policy judgment.Policy, mode judgment.StrictnessMode) (diagnostic.Diagnostic, bool) {
	if item.Code != judgment.CodeDiscriminatedUnion || item.Subject.Kind != judgment.SubjectExpression || len(item.Spans) == 0 {
		return diagnostic.Diagnostic{}, false
	}
	severity, ok := diagnosticSeverityForJudgment(item, policy, mode)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	missing := discriminatedUnionJudgmentCases(item, judgment.EvidenceDetailDiscriminatedUnionMissing)
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
		Message:     discriminatedUnionExhaustivenessMessage(caseWord, discriminantCaseList(missing)),
		Explanation: discriminatedUnionExhaustivenessExplanation(item, span),
		Help:        discriminatedUnionExhaustivenessHelp(),
		Labels:      []diagnostic.Label{sourceLabel(span, labelUnionCaseTest)},
	}), true
}

func discriminatedUnionExhaustivenessExplanation(item judgment.Judgment, fallback diagnostic.Span) diagnostic.Explanation {
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

func discriminatedUnionJudgmentCases(item judgment.Judgment, kind judgment.EvidenceDetailKind) []string {
	for _, evidence := range item.Evidence {
		if evidence.Detail.Kind == kind {
			return discriminatedUnionCaseListFromKey(evidence.Detail.CaseList)
		}
	}
	return nil
}

func discriminatedUnionCaseListFromKey(key string) []string {
	if key == "" {
		return nil
	}
	return strings.Split(key, "\x1f")
}
