package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func renderRedundantConditionJudgmentWithPolicy(item judgment.Judgment, policy judgment.Policy, mode judgment.StrictnessMode) (diagnostic.Diagnostic, bool) {
	if item.Code != judgment.CodeRedundantCondition || item.Subject.Kind != judgment.SubjectExpression || len(item.Spans) == 0 {
		return diagnostic.Diagnostic{}, false
	}
	severity, ok := diagnosticSeverityForJudgment(item, policy, mode)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	check, proof, stable, always := redundantConditionJudgmentDetails(item)
	span := diagnosticSpanFromJudgment(item.Spans[0])
	message := redundantConditionMessage(always)
	labels := []diagnostic.Label{sourceLabel(span, labelConditionCheck)}
	var proofSpan diagnostic.Span
	if len(item.Spans) > 1 {
		proofSpan = diagnosticSpanFromJudgment(item.Spans[1])
	}
	if proofSpan.Valid() && !diagnosticSpanEqual(proofSpan, span) {
		labels = append(labels, sourceLabel(proofSpan, labelProvingGuard))
	}
	proofEvidence := diagnostic.Evidence{
		Kind:    diagnostic.EvidenceAbstractFact,
		Trust:   diagnostic.TrustProven,
		Span:    proofSpan,
		Message: proof,
	}
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Span:     span,
		Code:     CodeRedundantCondition,
		Severity: severity,
		Message:  message,
		Labels:   labels,
		Explanation: diagnostic.NewExplanation(
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    span,
				Message: conditionCheckEvidence(check),
			},
			proofEvidence,
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Message: stable,
			},
		),
		Help: redundantConditionHelp(always),
	}), true
}

func redundantConditionJudgmentDetails(item judgment.Judgment) (check, proof, stable string, always bool) {
	for _, evidence := range item.Evidence {
		switch evidence.Detail.Kind {
		case judgment.EvidenceDetailRedundantConditionCheck:
			check = evidence.Detail.Message
			always = evidence.Detail.Always
		case judgment.EvidenceDetailRedundantConditionProof:
			proof = evidence.Detail.Message
		case judgment.EvidenceDetailRedundantConditionStability:
			stable = evidence.Detail.Message
		}
	}
	return check, proof, stable, always
}
