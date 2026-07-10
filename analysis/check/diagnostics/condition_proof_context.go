package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

type redundantConditionPresentation struct {
	Message     string
	Help        string
	Explanation diagnostic.Explanation
	Labels      []diagnostic.Label
}

func (ProofContext) RedundantCondition(item judgment.Judgment, primary diagnostic.Span) redundantConditionPresentation {
	check, proof, stable, always := redundantConditionDetails(item)
	labels := []diagnostic.Label{sourceLabel(primary, labelConditionCheck)}
	var proofSpan diagnostic.Span
	if len(item.Spans) > 1 {
		proofSpan = diagnosticSpanFromJudgment(item.Spans[1])
	}
	if proofSpan.Valid() && !diagnosticSpanEqual(proofSpan, primary) {
		labels = append(labels, sourceLabel(proofSpan, labelProvingGuard))
	}
	return redundantConditionPresentation{
		Message: display.RedundantConditionMessage(always),
		Help:    display.RedundantConditionHelp(always),
		Labels:  labels,
		Explanation: diagnostic.NewExplanation(
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    primary,
				Message: display.ConditionCheckEvidence(check),
			},
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    proofSpan,
				Message: proof,
			},
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Message: stable,
			},
		),
	}
}

func redundantConditionDetails(item judgment.Judgment) (check, proof, stable string, always bool) {
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
