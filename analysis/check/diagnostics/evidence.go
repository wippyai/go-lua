package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/internal/readmodel"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	valueevidence "github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func boundaryDiagnosticEvidenceForSubject(result *body.Result, point cfg.Point, span diagnostic.Span, subject string, want typ.Type, read boundaryValueReader) []diagnostic.Evidence {
	if result == nil || result.Registry() == nil || read == nil {
		return nil
	}
	value, ok := read(result, point)
	if !ok {
		return nil
	}
	reg := result.Registry()
	out := diagnostic.AssertionEvidence(span, product.Get(reg, value, assertion.Key))
	proof := product.Get(reg, value, valueevidence.Key)
	if proof.IsExplicitTop() {
		out = append(out, diagnostic.Evidence{
			Kind:    diagnostic.EvidencePrecisionBoundary,
			Trust:   diagnostic.TrustUnknown,
			Reason:  diagnostic.EvidenceReasonExplicitBoundaryValidation,
			Span:    span,
			Message: explicitBoundaryProofMessageForSubject(subject, want),
		})
	}
	if !readmodel.New(result).ValueProofAdmissible(value, want) {
		out = append(out, missingBoundaryProofEvidenceForSubject(span, subject, want))
	}
	return out
}

func missingBoundaryProofEvidenceForSubject(span diagnostic.Span, subject string, want typ.Type) diagnostic.Evidence {
	return diagnostic.Evidence{
		Kind:    diagnostic.EvidenceMissingProof,
		Trust:   diagnostic.TrustUnknown,
		Reason:  diagnostic.EvidenceReasonBoundaryValidationMissing,
		Span:    span,
		Message: missingBoundaryProofMessageForSubject(subject, want),
	}
}

func missingIndexReadProofEvidence(span diagnostic.Span, want typ.Type) diagnostic.Evidence {
	return diagnostic.Evidence{
		Kind:    diagnostic.EvidenceMissingProof,
		Trust:   diagnostic.TrustUnknown,
		Reason:  diagnostic.EvidenceReasonIndexReadValidationMissing,
		Span:    span,
		Message: missingIndexReadProofMessage(want),
	}
}

func hasMissingBoundaryProofEvidence(evidence []diagnostic.Evidence) bool {
	for _, item := range evidence {
		if item.Kind == diagnostic.EvidenceMissingProof {
			return true
		}
	}
	return false
}
