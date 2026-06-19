package diagnostics

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestClarifyTypeMismatchEvidenceUsesStructuredReason(t *testing.T) {
	span := diagnostic.Span{StartLine: 1, StartCol: 17}
	sourceSpan := diagnostic.Span{StartLine: 1, StartCol: 17, EndLine: 1, EndCol: 25}
	items := []diagnostic.Evidence{
		{
			Kind:    diagnostic.EvidenceMissingProof,
			Trust:   diagnostic.TrustUnknown,
			Reason:  diagnostic.EvidenceReasonIndexReadValidationMissing,
			Span:    span,
			Message: "wording changed upstream",
		},
		{
			Kind:    diagnostic.EvidenceMissingProof,
			Trust:   diagnostic.TrustUnknown,
			Reason:  diagnostic.EvidenceReasonBoundaryValidationMissing,
			Span:    span,
			Message: "wording changed upstream",
		},
	}

	got := clarifyTypeMismatchEvidence(items, "row.name", typ.String, sourceSpan, "declared type")
	requireDiagnosticEvidence(t, got, []diagnosticEvidenceWant{
		{
			kind:    diagnostic.EvidenceMissingProof,
			trust:   diagnostic.TrustUnknown,
			reason:  diagnostic.EvidenceReasonIndexReadValidationMissing,
			message: "row.name is an indexed read that can miss or read nil; no proof shows the selected slot satisfies the declared type here",
			span:    sourceSpan,
		},
		{
			kind:    diagnostic.EvidenceMissingProof,
			trust:   diagnostic.TrustUnknown,
			reason:  diagnostic.EvidenceReasonBoundaryValidationMissing,
			message: "no proof on this path shows row.name satisfies the declared type",
			span:    sourceSpan,
		},
	})
}

func TestClarifyTypeMismatchEvidenceUsesOptionalReasonForNilGuard(t *testing.T) {
	got := clarifyTypeMismatchEvidence([]diagnostic.Evidence{{
		Kind:   diagnostic.EvidenceMissingProof,
		Trust:  diagnostic.TrustUnknown,
		Reason: diagnostic.EvidenceReasonBoundaryValidationMissing,
	}}, "cache.value", typ.MaterializeOptional(typ.String), diagnostic.Span{}, "declared type")

	requireDiagnosticEvidence(t, got, []diagnosticEvidenceWant{
		{
			kind:    diagnostic.EvidenceMissingProof,
			trust:   diagnostic.TrustUnknown,
			reason:  diagnostic.EvidenceReasonBoundaryValidationMissing,
			message: "no guard on this path proves cache.value is non-nil",
		},
	})
}

func TestClarifyEvidenceDoesNotRewriteByEnglishPrefix(t *testing.T) {
	items := []diagnostic.Evidence{{
		Kind:    diagnostic.EvidenceMissingProof,
		Trust:   diagnostic.TrustUnknown,
		Message: "indexed read can miss or read nil; no proof shows the selected slot satisfies string here",
	}}

	got := clarifyTypeMismatchEvidence(items, "row.name", typ.String, diagnostic.Span{}, "declared type")
	requireDiagnosticEvidence(t, got, []diagnosticEvidenceWant{
		{
			kind:    diagnostic.EvidenceMissingProof,
			trust:   diagnostic.TrustUnknown,
			reason:  diagnostic.EvidenceReasonUnspecified,
			message: items[0].Message,
		},
	})
}

func TestClarifyReturnContractEvidenceUsesStructuredReason(t *testing.T) {
	items := []diagnostic.Evidence{
		{
			Kind:    diagnostic.EvidenceMissingProof,
			Trust:   diagnostic.TrustUnknown,
			Reason:  diagnostic.EvidenceReasonIndexReadValidationMissing,
			Message: "wording changed upstream",
		},
		{
			Kind:    diagnostic.EvidencePrecisionBoundary,
			Trust:   diagnostic.TrustUnknown,
			Reason:  diagnostic.EvidenceReasonExplicitBoundaryValidation,
			Message: "wording changed upstream",
		},
		{
			Kind:    diagnostic.EvidenceMissingProof,
			Trust:   diagnostic.TrustUnknown,
			Reason:  diagnostic.EvidenceReasonBoundaryValidationMissing,
			Message: "wording changed upstream",
		},
	}

	got := clarifyReturnContractEvidence(items, "returned value 1 (raw)", diagnostic.Span{})
	requireDiagnosticEvidence(t, got, []diagnosticEvidenceWant{
		{
			kind:    diagnostic.EvidenceMissingProof,
			trust:   diagnostic.TrustUnknown,
			reason:  diagnostic.EvidenceReasonIndexReadValidationMissing,
			message: "returned value 1 (raw) is an indexed read that can miss or read nil; no proof shows the selected slot satisfies the declared return type here",
		},
		{
			kind:    diagnostic.EvidencePrecisionBoundary,
			trust:   diagnostic.TrustUnknown,
			reason:  diagnostic.EvidenceReasonExplicitBoundaryValidation,
			message: "returned value 1 (raw) comes from any/unknown",
		},
		{
			kind:    diagnostic.EvidenceMissingProof,
			trust:   diagnostic.TrustUnknown,
			reason:  diagnostic.EvidenceReasonBoundaryValidationMissing,
			message: "no proof on this path shows returned value 1 (raw) satisfies the declared return type",
		},
	})
}
