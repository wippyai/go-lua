package diagnostics

import (
	"fmt"

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
	case judgment.CodeAdviceSplitBirthDiscriminant:
		return adviceSplitBirthDiscriminantPresentation(item, primary), true
	case judgment.CodeAdviceShapePolymorphic:
		return adviceShapePolymorphicPresentation(item, primary), true
	default:
		return advicePresentation{}, false
	}
}

func adviceRedundantClaimPresentation(item judgment.Judgment, primary diagnostic.Span) advicePresentation {
	claimEvidence, _ := item.FirstEvidenceKindDetail(judgment.EvidenceAbstractFact, judgment.EvidenceDetailAdviceClaimSite)
	provenEvidence, _ := item.FirstEvidenceKindDetail(judgment.EvidenceAbstractFact, judgment.EvidenceDetailAdviceProvenType)
	claim := adviceEvidenceMessage(claimEvidence)
	proven := adviceEvidenceMessage(provenEvidence)
	provenSpan := diagnosticJudgmentEvidenceSpanOr(provenEvidence, primary)
	labels := []diagnostic.Label{sourceLabel(primary, labelAdviceClaim)}
	labels = appendDistinctSourceLabel(labels, provenSpan, primary, labelAdviceProvenValue)
	return advicePresentation{
		Message: display.AdviceRedundantClaimMessage(item.Subject.Label, item.Expected.Type),
		Help:    display.AdviceRedundantClaimHelp(),
		Labels:  labels,
		Explanation: diagnostic.NewExplanation(
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Cause:   diagnosticCauseFromJudgmentEvidence(provenEvidence),
				Span:    provenSpan,
				Message: proven,
			},
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Cause:   diagnosticCauseFromJudgmentEvidence(claimEvidence),
				Span:    primary,
				Message: claim,
			},
		),
	}
}

func adviceAlwaysTrueGuardPresentation(item judgment.Judgment, primary diagnostic.Span) advicePresentation {
	evidence, message, always := adviceGuardMessage(item)
	return advicePresentation{
		Message: display.AdviceAlwaysTrueGuardMessage(always),
		Help:    display.AdviceAlwaysTrueGuardHelp(always),
		Labels:  []diagnostic.Label{sourceLabel(primary, labelAdviceGuard)},
		Explanation: diagnostic.NewExplanation(diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Cause:   diagnosticCauseFromJudgmentEvidence(evidence),
			Span:    primary,
			Message: message,
		}),
	}
}

func adviceInvariantLoopReadPresentation(item judgment.Judgment, primary diagnostic.Span) advicePresentation {
	invariantEvidence, _ := item.FirstEvidenceKindDetail(judgment.EvidenceAbstractFact, judgment.EvidenceDetailAdviceLoopInvariant)
	nonNilEvidence, _ := item.FirstEvidenceKindDetail(judgment.EvidenceAbstractFact, judgment.EvidenceDetailAdviceReceiverNonNil)
	invariant := adviceEvidenceMessage(invariantEvidence)
	nonNil := adviceEvidenceMessage(nonNilEvidence)
	loopSpan := adviceLoopSpan(item, primary)
	labels := []diagnostic.Label{sourceLabel(primary, labelAdviceLoopRead)}
	labels = appendDistinctSourceLabel(labels, loopSpan, primary, labelAdviceLoopHead)
	return advicePresentation{
		Message: display.AdviceInvariantLoopReadMessage(item.Subject.Label),
		Help:    display.AdviceInvariantLoopReadHelp(item.Subject.Label),
		Labels:  labels,
		Explanation: diagnostic.NewExplanation(
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Cause:   diagnosticCauseFromJudgmentEvidence(invariantEvidence),
				Span:    primary,
				Message: invariant,
			},
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Cause:   diagnosticCauseFromJudgmentEvidence(nonNilEvidence),
				Span:    primary,
				Message: nonNil,
			},
		),
	}
}

func adviceSplitBirthDiscriminantPresentation(item judgment.Judgment, primary diagnostic.Span) advicePresentation {
	labels := []diagnostic.Label{sourceLabel(primary, labelAdviceTagWrite)}
	var evidence []diagnostic.Evidence
	for _, itemEvidence := range item.Evidence {
		span := diagnosticJudgmentEvidenceSpanOr(itemEvidence, primary)
		switch itemEvidence.Detail.Kind {
		case judgment.EvidenceDetailAdviceTableBirth:
			labels = appendDistinctSourceLabel(labels, span, primary, labelAdviceTableBirth)
		case judgment.EvidenceDetailAdvicePayloadWrite:
			labels = appendDistinctSourceLabel(labels, span, primary, labelAdvicePayloadWrite)
		case judgment.EvidenceDetailAdviceDiscriminantUse:
			labels = appendDistinctSourceLabel(labels, span, primary, labelAdviceDiscriminantUse)
		}
		evidence = append(evidence, diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Cause:   diagnosticCauseFromJudgmentEvidence(itemEvidence),
			Span:    span,
			Message: adviceEvidenceMessage(itemEvidence),
		})
	}
	return advicePresentation{
		Message:     display.AdviceSplitBirthDiscriminantMessage(item.Subject.Label),
		Help:        display.AdviceSplitBirthDiscriminantHelp(),
		Labels:      labels,
		Explanation: diagnostic.NewExplanation(evidence...),
	}
}

func adviceShapePolymorphicPresentation(item judgment.Judgment, primary diagnostic.Span) advicePresentation {
	labels := []diagnostic.Label{sourceLabel(primary, labelAdviceShapeUse)}
	var evidence []diagnostic.Evidence
	for _, itemEvidence := range item.Evidence {
		span := diagnosticJudgmentEvidenceSpanOr(itemEvidence, primary)
		switch itemEvidence.Detail.Kind {
		case judgment.EvidenceDetailAdviceTableBirth:
			labels = appendDistinctSourceLabel(labels, span, primary, labelAdviceTableBirth)
		case judgment.EvidenceDetailAdviceShapeConditionalField:
			labels = appendDistinctSourceLabel(labels, span, primary, labelAdviceShapeConditionalField)
		}
		evidence = append(evidence, diagnostic.Evidence{Kind: diagnostic.EvidenceAbstractFact, Trust: diagnostic.TrustProven, Cause: diagnosticCauseFromJudgmentEvidence(itemEvidence), Span: span, Message: adviceEvidenceMessage(itemEvidence)})
	}
	return advicePresentation{Message: display.AdviceShapePolymorphicMessage(item.Subject.Label), Help: display.AdviceShapePolymorphicHelp(), Labels: labels, Explanation: diagnostic.NewExplanation(evidence...)}
}

func adviceGuardMessage(item judgment.Judgment) (judgment.Evidence, string, bool) {
	evidence, ok := item.FirstEvidenceKindDetail(judgment.EvidenceAbstractFact, judgment.EvidenceDetailAdviceGuardValue)
	if !ok {
		return judgment.Evidence{}, "", false
	}
	return evidence, adviceEvidenceMessage(evidence), evidence.Detail.Always
}

func adviceEvidenceMessage(evidence judgment.Evidence) string {
	if !evidence.Detail.Cause.IsZero() {
		if message, ok := adviceCauseMessage(evidence.Detail); ok {
			return message
		}
	}
	return evidence.Detail.Message
}

func adviceCauseMessage(detail judgment.EvidenceDetail) (string, bool) {
	params := detail.Cause.Params
	switch detail.Kind {
	case judgment.EvidenceDetailAdviceClaimSite:
		return fmt.Sprintf("claim checks %s at this site", params.Type), true
	case judgment.EvidenceDetailAdviceProvenType:
		return fmt.Sprintf("%s is proven to be %s before the claim", params.Subject, params.Type), true
	case judgment.EvidenceDetailAdviceGuardValue:
		value := "false"
		if detail.Always {
			value = "true"
		}
		return fmt.Sprintf("condition is proven to be %s on every reachable path", value), true
	case judgment.EvidenceDetailAdviceLoopInvariant:
		return fmt.Sprintf("%s is not written by the loop body", params.Path), true
	case judgment.EvidenceDetailAdviceReceiverNonNil:
		return fmt.Sprintf("%s is non-nil on all loop paths", params.Subject), true
	case judgment.EvidenceDetailAdviceTableBirth:
		return fmt.Sprintf("%s is born as a table here", params.Subject), true
	case judgment.EvidenceDetailAdviceTagWrite:
		return fmt.Sprintf("%s is assigned literal %q here", params.Subject, params.Field), true
	case judgment.EvidenceDetailAdvicePayloadWrite:
		return fmt.Sprintf("%s is assigned separately", params.Subject), true
	case judgment.EvidenceDetailAdviceDiscriminantUse:
		return fmt.Sprintf("%s is used as a discriminant here", params.Subject), true
	case judgment.EvidenceDetailAdviceShapeConditionalField:
		return fmt.Sprintf("%s is added only on some paths", params.Subject+"."+params.Field), true
	case judgment.EvidenceDetailAdviceShapeStableRefused:
		return fmt.Sprintf("StableShape is refused because %s has a non-uniform field set", params.Subject), true
	case judgment.EvidenceDetailAdviceShapeUse:
		return fmt.Sprintf("%s is used where a fixed shape matters", params.Subject), true
	case judgment.EvidenceDetailAdviceShapeUnionField:
		return fmt.Sprintf("%s.%s belongs in the fixed-shape constructor", params.Subject, params.Field), true
	default:
		return "", false
	}
}

func adviceLoopSpan(item judgment.Judgment, fallback diagnostic.Span) diagnostic.Span {
	if len(item.Spans) > 1 {
		return diagnosticSpanFromJudgment(item.Spans[1])
	}
	return fallback
}
