package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

type advicePresentation struct {
	Message     string
	Help        string
	Explanation diagnostic.Explanation
	Labels      []diagnostic.Label
}

func (ProofContext) Advice(item judgment.Judgment, primary diagnostic.Span) (advicePresentation, bool) {
	switch item.Code {
	case judgment.CodeAdviceRedundantClaim:
		return adviceRedundantClaimPresentation(item, primary), true
	case judgment.CodeAdviceAlwaysTrueGuard:
		return adviceAlwaysTrueGuardPresentation(item, primary), true
	case judgment.CodeAdviceInvariantLoopRead:
		return adviceInvariantLoopReadPresentation(item, primary), true
	default:
		return advicePresentation{}, false
	}
}

func adviceRedundantClaimPresentation(item judgment.Judgment, primary diagnostic.Span) advicePresentation {
	claim, proven := adviceEvidenceMessages(item)
	provenSpan := adviceEvidenceSpan(item, judgment.EvidenceDetailAdviceProvenType, primary)
	labels := []diagnostic.Label{sourceLabel(primary, labelAdviceClaim)}
	if provenSpan.Valid() && !diagnosticSpanEqual(provenSpan, primary) {
		labels = append(labels, sourceLabel(provenSpan, labelAdviceProvenValue))
	}
	return advicePresentation{
		Message: display.AdviceRedundantClaimMessage(item.Subject.Label, item.Expected.Type),
		Help:    display.AdviceRedundantClaimHelp(),
		Labels:  labels,
		Explanation: diagnostic.NewExplanation(
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    provenSpan,
				Message: proven,
			},
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    primary,
				Message: claim,
			},
		),
	}
}

func adviceAlwaysTrueGuardPresentation(item judgment.Judgment, primary diagnostic.Span) advicePresentation {
	message, always := adviceGuardMessage(item)
	return advicePresentation{
		Message: display.AdviceAlwaysTrueGuardMessage(always),
		Help:    display.AdviceAlwaysTrueGuardHelp(always),
		Labels:  []diagnostic.Label{sourceLabel(primary, labelAdviceGuard)},
		Explanation: diagnostic.NewExplanation(diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    primary,
			Message: message,
		}),
	}
}

func adviceInvariantLoopReadPresentation(item judgment.Judgment, primary diagnostic.Span) advicePresentation {
	invariant, nonNil := adviceEvidenceMessages(item)
	loopSpan := adviceLoopSpan(item, primary)
	labels := []diagnostic.Label{sourceLabel(primary, labelAdviceLoopRead)}
	if loopSpan.Valid() && !diagnosticSpanEqual(loopSpan, primary) {
		labels = append(labels, sourceLabel(loopSpan, labelAdviceLoopHead))
	}
	return advicePresentation{
		Message: display.AdviceInvariantLoopReadMessage(item.Subject.Label),
		Help:    display.AdviceInvariantLoopReadHelp(item.Subject.Label),
		Labels:  labels,
		Explanation: diagnostic.NewExplanation(
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    primary,
				Message: invariant,
			},
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    primary,
				Message: nonNil,
			},
		),
	}
}

func adviceEvidenceMessages(item judgment.Judgment) (first, second string) {
	for _, evidence := range item.Evidence {
		switch evidence.Detail.Kind {
		case judgment.EvidenceDetailAdviceClaimSite, judgment.EvidenceDetailAdviceLoopInvariant:
			first = evidence.Detail.Message
		case judgment.EvidenceDetailAdviceProvenType, judgment.EvidenceDetailAdviceReceiverNonNil:
			second = evidence.Detail.Message
		}
	}
	return first, second
}

func adviceGuardMessage(item judgment.Judgment) (string, bool) {
	for _, evidence := range item.Evidence {
		if evidence.Detail.Kind == judgment.EvidenceDetailAdviceGuardValue {
			return evidence.Detail.Message, evidence.Detail.Always
		}
	}
	return "", false
}

func adviceEvidenceSpan(item judgment.Judgment, detail judgment.EvidenceDetailKind, fallback diagnostic.Span) diagnostic.Span {
	for _, evidence := range item.Evidence {
		if evidence.Detail.Kind != detail {
			continue
		}
		return diagnosticJudgmentEvidenceSpanOr(evidence, fallback)
	}
	return fallback
}

func adviceLoopSpan(item judgment.Judgment, fallback diagnostic.Span) diagnostic.Span {
	if len(item.Spans) > 1 {
		return diagnosticSpanFromJudgment(item.Spans[1])
	}
	return fallback
}
