package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/diagnostics/internal/readmodel"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	valueevidence "github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func boundaryDiagnosticEvidence(result *body.Result, point cfg.Point, span diagnostic.Span, want typ.Type, read boundaryValueReader) []diagnostic.Evidence {
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
			Span:    span,
			Message: "explicit any/unknown boundary has no structural proof for " + formatType(want),
		})
	}
	if !readmodel.New(result).ValueProofAdmissible(value, want) {
		out = append(out, diagnostic.Evidence{
			Kind:    diagnostic.EvidenceMissingProof,
			Trust:   diagnostic.TrustUnknown,
			Span:    span,
			Message: "no boundary proof establishes " + formatType(want),
		})
	}
	return out
}
