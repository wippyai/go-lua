package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func renderResultShapeJudgmentWithPolicy(item judgment.Judgment, policy judgment.Policy, mode judgment.StrictnessMode) (diagnostic.Diagnostic, bool) {
	if item.Code != judgment.CodeResultShape || item.Subject.Kind != judgment.SubjectExpression || len(item.Spans) == 0 {
		return diagnostic.Diagnostic{}, false
	}
	severity, ok := diagnosticSeverityForJudgment(item, policy, mode)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	readPath, requiredCase, ok := resultShapeJudgmentReadAndCase(item)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	span := diagnosticSpanFromJudgment(item.Spans[0])
	return diagnostic.New(diagnostic.DiagnosticSpec{
		File:        item.Spans[0].File,
		Span:        span,
		Code:        CodeDiscriminatedUnionExhaustive,
		Severity:    severity,
		Message:     resultShapeExhaustivenessMessage(readPath, requiredCase),
		Explanation: resultShapeExhaustivenessExplanation(item, span),
		Help:        resultShapeExhaustivenessHelp(),
		Labels:      []diagnostic.Label{sourceLabel(span, labelResultFieldRead)},
	}), true
}

func resultShapeJudgmentReadAndCase(item judgment.Judgment) (string, string, bool) {
	for _, evidence := range item.Evidence {
		if evidence.Detail.Kind == judgment.EvidenceDetailResultShapeFieldCase {
			return evidence.Detail.SubjectLabel, evidence.Detail.CaseList, evidence.Detail.SubjectLabel != "" && evidence.Detail.CaseList != ""
		}
	}
	return "", "", false
}

func resultShapeExhaustivenessExplanation(item judgment.Judgment, fallback diagnostic.Span) diagnostic.Explanation {
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
