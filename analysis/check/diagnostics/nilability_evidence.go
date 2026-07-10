package diagnostics

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// nilabilityProvenanceEvidence renders the shared structured evidence emitted
// by the obligation pass. The pass has already selected the source facts; this
// function only gives those facts diagnostic-family-neutral wording.
func nilabilityProvenanceEvidence(item judgment.Judgment, subject string, got typ.Type, primary diagnostic.Span) []diagnostic.Evidence {
	if subject == "" {
		subject = "value"
	}
	var out []diagnostic.Evidence
	for _, evidence := range item.Evidence {
		detail := evidence.Detail
		switch {
		case detail.Kind == judgment.EvidenceDetailCallResultAssignment:
			name := detail.FunctionName
			if name == "" {
				name = "callee"
			}
			result := fmt.Sprintf("return %d", detail.ResultIndex+1)
			message := fmt.Sprintf("%s %s has type %s and may be nil", name, result, display.Type(got))
			if detail.UnderSupplied {
				message = fmt.Sprintf("%s does not produce %s, so Lua fills it with nil", name, result)
			}
			kind := diagnostic.EvidenceAbstractFact
			trust := diagnosticTrustFromJudgmentTrust(evidence.Trust, diagnostic.TrustProven)
			if evidence.Kind == judgment.EvidenceUserAssertion {
				kind = diagnostic.EvidenceUserAssertion
				trust = diagnosticTrustFromJudgmentTrust(evidence.Trust, diagnostic.TrustClaimed)
				message = display.CallResultDeclaredReturnEvidence(name, result, got)
			}
			out = append(out, diagnostic.Evidence{
				Kind: kind, Trust: trust, Cause: diagnosticCauseFromJudgmentEvidence(evidence),
				Span: diagnosticJudgmentEvidenceSpanOr(evidence, primary), Message: message,
			})
		case detail.Kind == judgment.EvidenceDetailMayBeNil && detail.Cause.Kind == judgment.EvidenceCauseBirth && detail.SubjectLabel != "":
			out = append(out, diagnostic.Evidence{
				Kind: diagnostic.EvidenceAbstractFact, Trust: diagnosticTrustFromJudgmentTrust(evidence.Trust, diagnostic.TrustProven),
				Cause: diagnosticCauseFromJudgmentEvidence(evidence), Span: diagnosticJudgmentEvidenceSpanOr(evidence, primary),
				Message: fmt.Sprintf("%s is an optional field and may be nil", detail.SubjectLabel),
			})
		case detail.Kind == judgment.EvidenceDetailMayBeNil && detail.Cause.Kind == judgment.EvidenceCauseFlowAssign && detail.SubjectLabel != "" && detail.Field != "":
			out = append(out, diagnostic.Evidence{
				Kind: diagnostic.EvidenceAbstractFact, Trust: diagnosticTrustFromJudgmentTrust(evidence.Trust, diagnostic.TrustProven),
				Cause: diagnosticCauseFromJudgmentEvidence(evidence), Span: diagnosticJudgmentEvidenceSpanOr(evidence, primary),
				Message: assignmentNilableAccessMessage(detail.SubjectLabel, detail.Field),
			})
		case detail.Kind == judgment.EvidenceDetailUserAssertedAny:
			out = append(out, diagnostic.Evidence{
				Kind: diagnostic.EvidenceUserAssertion, Trust: diagnosticTrustFromJudgmentTrust(evidence.Trust, diagnostic.TrustClaimed),
				Reason: diagnostic.EvidenceReasonUserAssertedAny, Cause: diagnosticCauseFromJudgmentEvidence(evidence),
				Span:    diagnosticJudgmentEvidenceSpanOr(evidence, primary),
				Message: fmt.Sprintf("%s comes from unresolved any/unknown", subject),
			})
		case evidence.Kind == judgment.EvidenceMissingProof && detail.Kind == judgment.EvidenceDetailMayBeNil && detail.SubjectLabel != "":
			out = append(out, diagnostic.Evidence{
				Kind: diagnostic.EvidenceMissingProof, Trust: diagnosticTrustFromJudgmentTrust(evidence.Trust, diagnostic.TrustUnknown),
				Cause: diagnosticCauseFromJudgmentEvidence(evidence), Span: diagnosticJudgmentEvidenceSpanOr(evidence, primary),
				Message: display.MissingNonNilGuardHereMessage(detail.SubjectLabel),
			})
		}
	}
	return out
}
