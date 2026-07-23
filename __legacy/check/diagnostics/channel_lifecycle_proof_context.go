package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func (ProofContext) ChannelLifecycle(item judgment.Judgment, primary diagnostic.Span) (simpleJudgmentPresentation, bool) {
	evidence, ok := item.FirstEvidenceKindDetail(judgment.EvidenceAbstractFact, judgment.EvidenceDetailChannelClosed)
	if !ok {
		return simpleJudgmentPresentation{}, false
	}
	channel := evidence.Detail.SubjectLabel
	if channel == "" {
		channel = item.Subject.Label
	}
	operation := evidence.Detail.Field
	switch item.Code {
	case judgment.CodeChannelSendClosed:
		operation = "send"
	case judgment.CodeChannelDoubleClose:
		operation = "close"
	}
	return simpleJudgmentPresentation{
		Message: display.ChannelLifecycleMessage(operation, channel),
		Help:    display.ChannelLifecycleHelp(operation),
		Labels:  []diagnostic.Label{sourceLabel(primary, labelChannelLifecycleCall)},
		Explanation: diagnostic.NewExplanation(diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceAbstractFact, diagnostic.TrustProven),
			Span:    diagnosticEvidenceSpanOr(item, judgment.EvidenceAbstractFact, primary),
			Message: display.ChannelLifecycleClosedEvidence(operation, channel),
		}),
	}, true
}
