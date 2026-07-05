package diagnostics

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

type channelSelectPresentation struct {
	Message     string
	Help        string
	Explanation diagnostic.Explanation
	Labels      []diagnostic.Label
}

func (ProofContext) ChannelSelect(item judgment.Judgment, primary diagnostic.Span) (channelSelectPresentation, bool) {
	missing := channelSelectCases(item, judgment.EvidenceDetailChannelSelectMissing)
	if len(missing) == 0 {
		return channelSelectPresentation{}, false
	}
	caseWord := pluralize(len(missing), "case", "cases")
	return channelSelectPresentation{
		Message:     channelSelectExhaustivenessMessage(caseWord, channelCaseList(missing)),
		Help:        channelSelectExhaustivenessHelp(),
		Explanation: channelSelectExplanation(item, primary),
		Labels:      []diagnostic.Label{sourceLabel(primary, labelChannelCaseTest)},
	}, true
}

func channelSelectExplanation(item judgment.Judgment, fallback diagnostic.Span) diagnostic.Explanation {
	var evidence []diagnostic.Evidence
	for _, itemEvidence := range item.Evidence {
		span := diagnosticSpanFromJudgment(itemEvidence.Span)
		if !span.Valid() {
			span = fallback
		}
		switch itemEvidence.Detail.Kind {
		case judgment.EvidenceDetailChannelSelectResult:
			evidence = append(evidence, diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnosticTrustFromJudgmentTrust(itemEvidence.Trust, diagnostic.TrustProven),
				Span:    span,
				Message: selectedChannelPathEvidence(itemEvidence.Detail.SubjectLabel),
			})
		case judgment.EvidenceDetailChannelSelectHandled:
			evidence = append(evidence, diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnosticTrustFromJudgmentTrust(itemEvidence.Trust, diagnostic.TrustProven),
				Span:    span,
				Message: handledChannelCasesEvidence(channelCaseList(channelSelectCaseListFromKey(itemEvidence.Detail.CaseList))),
			})
		case judgment.EvidenceDetailChannelSelectMissing:
			evidence = append(evidence, diagnostic.Evidence{
				Kind:    diagnostic.EvidenceMissingProof,
				Trust:   diagnosticTrustFromJudgmentTrust(itemEvidence.Trust, diagnostic.TrustUnknown),
				Span:    span,
				Message: missingChannelCasesEvidence(channelCaseList(channelSelectCaseListFromKey(itemEvidence.Detail.CaseList))),
			})
		case judgment.EvidenceDetailChannelSelectNoDefault:
			evidence = append(evidence, diagnostic.Evidence{
				Kind:    diagnostic.EvidenceMissingProof,
				Trust:   diagnosticTrustFromJudgmentTrust(itemEvidence.Trust, diagnostic.TrustUnknown),
				Span:    span,
				Message: missingChannelDefaultEvidence(),
			})
		}
	}
	return diagnostic.NewExplanation(evidence...)
}

func channelSelectCases(item judgment.Judgment, kind judgment.EvidenceDetailKind) []string {
	for _, evidence := range item.Evidence {
		if evidence.Detail.Kind == kind {
			return channelSelectCaseListFromKey(evidence.Detail.CaseList)
		}
	}
	return nil
}

func channelSelectCaseListFromKey(key string) []string {
	if key == "" {
		return nil
	}
	return strings.Split(key, "\x1f")
}
