package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

type numericForPresentation struct {
	Message  string
	Help     string
	Evidence []diagnostic.Evidence
	Labels   []diagnostic.Label
}

func (ProofContext) NumericFor(item judgment.Judgment, primary diagnostic.Span) (numericForPresentation, bool) {
	proof := item.NumericForProof()
	got := item.Actual.ProjectedType
	if got == nil {
		return numericForPresentation{}, false
	}
	role := item.Expected.Label
	if role == "" {
		role = item.Subject.Label
	}
	if role == "" {
		role = "operand"
	}
	evidence := []diagnostic.Evidence{
		{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceAbstractFact, diagnostic.TrustProven),
			Span:    diagnosticEvidenceSpanOr(item, judgment.EvidenceAbstractFact, primary),
			Message: display.NumericForOperandTypeEvidence(role, got),
		},
	}
	if proof.UserAssertion {
		evidence = append(evidence, numericForExplicitTopEvidence(item, proof, primary)...)
	}
	return numericForPresentation{
		Message:  display.NumericForOperandMessage(role, got),
		Help:     display.NumericForOperandHelp(role),
		Evidence: evidence,
		Labels:   []diagnostic.Label{sourceLabel(primary, role)},
	}, true
}

func numericForExplicitTopEvidence(item judgment.Judgment, proof judgment.NumericForProofSummary, primary diagnostic.Span) []diagnostic.Evidence {
	subject := "assigned value"
	want := item.Expected.Type
	if want == nil {
		want = typ.Number
	}
	out := diagnostic.AssertionEvidence(primary, assertion.Any())
	if proof.PrecisionBoundary {
		out = append(out, diagnostic.Evidence{
			Kind:    diagnostic.EvidencePrecisionBoundary,
			Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidencePrecisionBoundary, diagnostic.TrustUnknown),
			Reason:  diagnostic.EvidenceReasonExplicitBoundaryValidation,
			Span:    diagnosticEvidenceSpanOr(item, judgment.EvidencePrecisionBoundary, primary),
			Message: display.ExplicitBoundaryProofMessageForSubject(subject, want),
		})
	}
	if proof.MissingProof {
		out = append(out, diagnostic.Evidence{
			Kind:    diagnostic.EvidenceMissingProof,
			Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceMissingProof, diagnostic.TrustUnknown),
			Reason:  diagnostic.EvidenceReasonBoundaryValidationMissing,
			Span:    diagnosticEvidenceSpanOr(item, judgment.EvidenceMissingProof, primary),
			Message: display.MissingBoundaryProofMessageForSubject(subject, want),
		})
	}
	return out
}
