package diagnostics

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func renderTypestateRequirementJudgmentWithPolicy(ctx judgmentRenderContext, item judgment.Judgment, policy judgment.Policy, mode judgment.StrictnessMode) (diagnostic.Diagnostic, bool) {
	if (item.Code != judgment.CodeTypestateInvalidRequirement && item.Code != judgment.CodeTypestateUnprovenRequirement) || item.Subject.Kind != judgment.SubjectExpression || len(item.Spans) == 0 {
		return diagnostic.Diagnostic{}, false
	}
	severity, ok := diagnosticSeverityForJudgment(item, policy, mode)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	span := diagnosticSpanFromJudgment(item.Spans[0])
	resource := item.Subject.Label
	protocol := ""
	for _, evidence := range item.Evidence {
		if evidence.Detail.Protocol != "" {
			protocol = evidence.Detail.Protocol
			break
		}
	}
	if item.Code == judgment.CodeTypestateInvalidRequirement {
		message := fmt.Sprintf("invalid typestate requirement for resource %s in protocol %s: expected %s, found %s", codeName(resource), protocol, codeName(item.Expected.Label), codeName(item.Actual.Label))
		return diagnostic.New(diagnostic.DiagnosticSpec{Location: item.Spans[0].Location, File: item.Spans[0].DisplayFile(), Span: span, Code: diagnosticCodeForJudgment(item), Severity: severity,
			Message: message, Explanation: diagnostic.NewExplanation(diagnostic.Evidence{Kind: diagnostic.EvidenceAbstractFact, Trust: diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceAbstractFact, diagnostic.TrustProven), Span: diagnosticEvidenceSpanOr(item, judgment.EvidenceAbstractFact, span), Message: fmt.Sprintf("this call requires %s to be in %s, but solved state is %s", codeName(resource), codeName(item.Expected.Label), codeName(item.Actual.Label))}),
			Labels: []diagnostic.Label{sourceLabel(span, "invalid typestate requirement")}, Help: fmt.Sprintf("Call this operation only when %s is in %s state.", codeName(resource), codeName(item.Expected.Label))}), true
	}
	message := fmt.Sprintf("cannot prove typestate requirement for resource %s: expected %s", codeName(resource), codeName(item.Expected.Label))
	return diagnostic.New(diagnostic.DiagnosticSpec{Location: item.Spans[0].Location, File: item.Spans[0].DisplayFile(), Span: span, Code: diagnosticCodeForJudgment(item), Severity: severity,
		Message: message, Explanation: diagnostic.NewExplanation(diagnostic.Evidence{Kind: diagnostic.EvidenceMissingProof, Trust: diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceMissingProof, diagnostic.TrustRefuted), Span: diagnosticEvidenceSpanOr(item, judgment.EvidenceMissingProof, span), Message: fmt.Sprintf("no proof establishes %s in %s state at this call", codeName(resource), codeName(item.Expected.Label))}),
		Labels: []diagnostic.Label{sourceLabel(span, "unproven typestate requirement")}, Help: fmt.Sprintf("Establish that %s is in %s state before this call.", codeName(resource), codeName(item.Expected.Label))}), true
}
