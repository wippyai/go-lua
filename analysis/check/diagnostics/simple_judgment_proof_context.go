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
		Message: display.NonNilAssertAlwaysNilMessage(name),
		Help:    display.NonNilAssertAlwaysNilHelp(name),
		Labels:  []diagnostic.Label{sourceLabel(primary, labelValueAlwaysNil)},
		Explanation: diagnostic.NewExplanation(
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceAbstractFact, diagnostic.TrustProven),
				Span:    diagnosticEvidenceSpanOr(item, judgment.EvidenceAbstractFact, primary),
				Message: display.NonNilAssertAlwaysNilEvidence(name),
			},
		),
	}
}

func (ProofContext) UnusedLocal(item judgment.Judgment, name string, primary diagnostic.Span) simpleJudgmentPresentation {
	return simpleJudgmentPresentation{
		Message: display.UnusedLocalMessage(name),
		Help:    display.UnusedLocalHelp(),
		Labels:  []diagnostic.Label{sourceLabel(primary, labelUnusedLocal)},
		Explanation: diagnostic.NewExplanation(
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceAbstractFact, diagnostic.TrustProven),
				Message: display.UnusedLocalEvidence(name),
			},
		),
	}
}

func (ProofContext) UnresolvedType(item judgment.Judgment, name string, primary diagnostic.Span) simpleJudgmentPresentation {
	return simpleJudgmentPresentation{
		Message: display.UnresolvedTypeMessage(name),
		Help:    display.UnresolvedTypeHelp(),
		Labels:  []diagnostic.Label{sourceLabel(primary, labelUnknownType)},
		Explanation: diagnostic.NewExplanation(
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceAbstractFact, diagnostic.TrustProven),
				Span:    primary,
				Message: display.UnresolvedTypeEvidence(name),
			},
		),
	}
}

func (ProofContext) UnresolvedValue(item judgment.Judgment, name string, primary diagnostic.Span) simpleJudgmentPresentation {
	return simpleJudgmentPresentation{
		Message: display.UnresolvedValueMessage(name),
		Help:    display.UnresolvedValueHelp(),
		Labels:  []diagnostic.Label{sourceLabel(primary, labelUnknownValue)},
		Explanation: diagnostic.NewExplanation(
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceAbstractFact, diagnostic.TrustProven),
				Span:    primary,
				Message: display.UnresolvedValueEvidence(name),
			},
		),
	}
}
