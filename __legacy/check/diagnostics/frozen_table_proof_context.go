package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

type frozenTablePresentation struct {
	Message  string
	Help     string
	Evidence []diagnostic.Evidence
	Labels   []diagnostic.Label
}

func (ProofContext) FrozenTable(item judgment.Judgment, primary diagnostic.Span) frozenTablePresentation {
	containerName := item.Subject.Label
	if containerName == "" {
		containerName = "table"
	}
	mutatingCall := frozenTableIsCall(item)
	evidence := frozenTableEvidence(item, primary, containerName, mutatingCall)
	labels := []diagnostic.Label{sourceLabel(primary, labelFrozenTableMutation)}
	if mutatingCall {
		labels[0] = sourceLabel(primary, labelFrozenTableCall)
	}
	if proofSpan, ok := frozenTableProofSpan(item); ok {
		labels = append(labels, sourceLabel(proofSpan, labelFreezeProof))
	}
	message := display.FrozenTableMutationMessage(containerName)
	help := display.FrozenTableAssignmentHelp()
	if mutatingCall {
		message = display.FrozenTableCallMutationMessage(containerName)
		help = display.FrozenTableCallHelp()
	}
	return frozenTablePresentation{
		Message:  message,
		Help:     help,
		Evidence: evidence,
		Labels:   labels,
	}
}

func frozenTableIsCall(item judgment.Judgment) bool {
	for _, evidence := range item.Evidence {
		if evidence.Detail.Kind == judgment.EvidenceDetailFrozenTableCall {
			return true
		}
	}
	return false
}

func frozenTableEvidence(item judgment.Judgment, primary diagnostic.Span, containerName string, mutatingCall bool) []diagnostic.Evidence {
	primaryMessage := frozenAssignmentEvidence(containerName)
	incomingMessage := frozenIncomingStateEvidence(containerName)
	if mutatingCall {
		primaryMessage = frozenCallMutationEvidence(containerName)
	}
	evidence := []diagnostic.Evidence{
		{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceAbstractFact, diagnostic.TrustProven),
			Span:    primary,
			Message: primaryMessage,
		},
		{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceAbstractFact, diagnostic.TrustProven),
			Message: incomingMessage,
		},
	}
	if proofSpan, ok := frozenTableProofSpan(item); ok {
		evidence[1].Span = proofSpan
		if mutatingCall {
			evidence[1].Message = frozenCallProofEvidence(containerName)
		} else {
			evidence[1].Message = frozenAssignmentProofEvidence(containerName)
		}
	}
	return evidence
}

func frozenTableProofSpan(item judgment.Judgment) (diagnostic.Span, bool) {
	for _, evidence := range item.Evidence {
		if evidence.Origin.Key == judgment.OriginFrozenTableProof && evidence.Span.StartLine != 0 {
			span := diagnosticSpanFromJudgment(evidence.Span)
			if span.Valid() {
				return span, true
			}
		}
	}
	return diagnostic.Span{}, false
}
