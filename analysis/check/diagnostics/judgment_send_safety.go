package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func renderSendIsolationJudgmentWithPolicy(_ judgmentRenderContext, item judgment.Judgment, policy judgment.Policy, mode judgment.StrictnessMode) (diagnostic.Diagnostic, bool) {
	if item.Code != judgment.CodeSendIsolation || item.Subject.Kind != judgment.SubjectCallArgument || len(item.Spans) == 0 {
		return diagnostic.Diagnostic{}, false
	}
	severity, ok := diagnosticSeverityForJudgment(item, policy, mode)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	span := diagnosticSpanFromJudgment(item.Spans[0])
	labels := []diagnostic.Label{sourceLabel(span, labelSendPayload)}
	if proofSpan, ok := sendSafetyProofSpan(item, span); ok {
		labels = append(labels, sourceLabel(proofSpan, labelSendSafetyProof))
	}
	return diagnostic.New(diagnostic.DiagnosticSpec{
		File:        item.Spans[0].File,
		Span:        span,
		Code:        CodeSendIsolation,
		Message:     sendSafetyMessage(item),
		Severity:    severity,
		Explanation: diagnostic.NewExplanation(sendSafetyEvidence(item, span)...),
		Help:        "No checker error is emitted by default; unknown send-safety uses the runtime copy fallback.",
		Labels:      labels,
	}), true
}

func sendSafetyMessage(item judgment.Judgment) string {
	switch item.Actual.Label {
	case "isolated":
		return "send payload is proven isolated for zero-copy transfer"
	case "immutable":
		return "send payload is proven immutable for zero-copy sharing"
	default:
		return "send payload is not proven isolated or immutable; runtime will copy"
	}
}

func sendSafetyEvidence(item judgment.Judgment, primary diagnostic.Span) []diagnostic.Evidence {
	out := make([]diagnostic.Evidence, 0, len(item.Evidence))
	for _, evidence := range item.Evidence {
		message := evidence.Detail.Message
		if message == "" {
			message = sendSafetyEvidenceFallback(item)
		}
		out = append(out, diagnostic.Evidence{
			Kind:    sendSafetyDiagnosticEvidenceKind(evidence.Kind),
			Trust:   diagnosticTrustFromJudgmentTrust(evidence.Trust, diagnostic.TrustUnknown),
			Span:    diagnosticJudgmentEvidenceSpanOr(evidence, primary),
			Message: message,
		})
	}
	return out
}

func sendSafetyDiagnosticEvidenceKind(kind judgment.EvidenceKind) diagnostic.EvidenceKind {
	switch kind {
	case judgment.EvidenceAbstractFact:
		return diagnostic.EvidenceAbstractFact
	case judgment.EvidenceUserAssertion:
		return diagnostic.EvidenceUserAssertion
	case judgment.EvidenceMissingProof:
		return diagnostic.EvidenceMissingProof
	case judgment.EvidencePrecisionBoundary:
		return diagnostic.EvidencePrecisionBoundary
	default:
		return diagnostic.EvidenceAbstractFact
	}
}

func sendSafetyEvidenceFallback(item judgment.Judgment) string {
	switch item.Actual.Label {
	case "isolated":
		return "isolation proof admits zero-copy transfer"
	case "immutable":
		return "immutable proof admits zero-copy sharing"
	default:
		return "missing zero-copy admission proof"
	}
}

func sendSafetyProofSpan(item judgment.Judgment, primary diagnostic.Span) (diagnostic.Span, bool) {
	for _, evidence := range item.Evidence {
		if evidence.Detail.Kind != judgment.EvidenceDetailSendSafetyProof {
			continue
		}
		span := diagnosticJudgmentEvidenceSpanOr(evidence, primary)
		if span.Valid() {
			return span, true
		}
	}
	return diagnostic.Span{}, false
}
