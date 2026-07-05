package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

type simpleJudgmentPresentation struct {
	Message     string
	Help        string
	Explanation diagnostic.Explanation
	Labels      []diagnostic.Label
}

func (ProofContext) NonNilAssertion(item judgment.Judgment, primary diagnostic.Span) simpleJudgmentPresentation {
	name := item.Subject.Label
	return simpleJudgmentPresentation{
		Message: nonNilAssertAlwaysNilMessage(name),
		Help:    nonNilAssertAlwaysNilHelp(name),
		Labels:  []diagnostic.Label{sourceLabel(primary, labelValueAlwaysNil)},
		Explanation: diagnostic.NewExplanation(
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceAbstractFact, diagnostic.TrustProven),
				Span:    diagnosticEvidenceSpanOr(item, judgment.EvidenceAbstractFact, primary),
				Message: nonNilAssertAlwaysNilEvidence(name),
			},
		),
	}
}

func (ProofContext) UnusedLocal(item judgment.Judgment, name string, primary diagnostic.Span) simpleJudgmentPresentation {
	return simpleJudgmentPresentation{
		Message: unusedLocalMessage(name),
		Help:    unusedLocalHelp(),
		Labels:  []diagnostic.Label{sourceLabel(primary, labelUnusedLocal)},
		Explanation: diagnostic.NewExplanation(
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceAbstractFact, diagnostic.TrustProven),
				Message: unusedLocalEvidence(name),
			},
		),
	}
}

func (ProofContext) UnresolvedType(item judgment.Judgment, name string, primary diagnostic.Span) simpleJudgmentPresentation {
	return simpleJudgmentPresentation{
		Message: unresolvedTypeMessage(name),
		Help:    unresolvedTypeHelp(),
		Labels:  []diagnostic.Label{sourceLabel(primary, labelUnknownType)},
		Explanation: diagnostic.NewExplanation(
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceAbstractFact, diagnostic.TrustProven),
				Span:    primary,
				Message: unresolvedTypeEvidence(name),
			},
		),
	}
}

func (ProofContext) UnresolvedValue(item judgment.Judgment, name string, primary diagnostic.Span) simpleJudgmentPresentation {
	return simpleJudgmentPresentation{
		Message: unresolvedValueMessage(name),
		Help:    unresolvedValueHelp(),
		Labels:  []diagnostic.Label{sourceLabel(primary, labelUnknownValue)},
		Explanation: diagnostic.NewExplanation(
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceAbstractFact, diagnostic.TrustProven),
				Span:    primary,
				Message: unresolvedValueEvidence(name),
			},
		),
	}
}
