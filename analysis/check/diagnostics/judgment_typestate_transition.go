package diagnostics

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func renderTypestateInvalidTransitionJudgmentWithPolicy(ctx judgmentRenderContext, item judgment.Judgment, policy judgment.Policy, mode judgment.StrictnessMode) (diagnostic.Diagnostic, bool) {
	if item.Code != judgment.CodeTypestateInvalidTransition || item.Subject.Kind != judgment.SubjectExpression || len(item.Spans) == 0 {
		return diagnostic.Diagnostic{}, false
	}
	severity, ok := diagnosticSeverityForJudgment(item, policy, mode)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	evidence, ok := item.FirstEvidenceKindDetail(judgment.EvidenceAbstractFact, judgment.EvidenceDetailTypestateInvalidTransition)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	resource := evidence.Detail.Resource
	if resource == "" {
		resource = item.Subject.Label
	}
	span := diagnosticSpanFromJudgment(item.Spans[0])
	message := fmt.Sprintf("invalid transition for resource %s in protocol %s: expected %s, found %s", codeName(resource), evidence.Detail.Protocol, codeName(evidence.Detail.FromState), codeName(evidence.Detail.CurrentState))
	explanation := fmt.Sprintf("this transition requires %s to be in %s, but solved state is %s", codeName(resource), codeName(evidence.Detail.FromState), codeName(evidence.Detail.CurrentState))
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Location: item.Spans[0].Location,
		File:     item.Spans[0].DisplayFile(),
		Span:     span,
		Code:     diagnosticCodeForJudgment(item),
		Severity: severity,
		Message:  message,
		Explanation: diagnostic.NewExplanation(diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceAbstractFact, diagnostic.TrustProven),
			Span:    diagnosticEvidenceSpanOr(item, judgment.EvidenceAbstractFact, span),
			Message: explanation,
		}),
		Labels: []diagnostic.Label{sourceLabel(span, "invalid lifecycle transition")},
		Help:   fmt.Sprintf("Transition %s only when it is in %s state.", codeName(resource), codeName(evidence.Detail.FromState)),
	}), true
}
