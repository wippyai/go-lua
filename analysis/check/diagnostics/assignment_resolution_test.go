package diagnostics

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestLocalAssignmentSourceTypeResolutionRefinementControls(t *testing.T) {
	var fallback ast.Expr = &ast.StringExpr{Value: "fallback"}
	var inner ast.Expr = &ast.StringExpr{Value: "inner"}

	if got := (localAssignmentSourceTypeResolution{}).RefineExpr(fallback); got != fallback {
		t.Fatalf("RefineExpr without cast = %#v, want fallback", got)
	}
	if got := (localAssignmentSourceTypeResolution{CastInnerExpr: inner}).RefineExpr(fallback); got != inner {
		t.Fatalf("RefineExpr with cast = %#v, want inner", got)
	}

	if !((localAssignmentSourceTypeResolution{}).AllowsFlowRefinement(typ.String)) {
		t.Fatalf("plain string source should allow flow refinement")
	}
	blocked := []localAssignmentSourceTypeResolution{
		{OptionalIndexProjection: true},
		{PresenceAwareSourceProjection: true},
	}
	for _, resolution := range blocked {
		if resolution.AllowsFlowRefinement(typ.String) {
			t.Fatalf("resolution %#v allowed flow refinement, want blocked", resolution)
		}
	}
	if (localAssignmentSourceTypeResolution{}).AllowsFlowRefinement(typ.Any) ||
		(localAssignmentSourceTypeResolution{}).AllowsFlowRefinement(typ.Unknown) {
		t.Fatalf("top-like source types should not be flow-refined")
	}
}

func TestLocalAssignmentSourceTypeResolutionMismatchBoundary(t *testing.T) {
	reader := boundaryValueReader(func(*body.Result, cfg.Point) (product.Value, bool) {
		return product.Top(), true
	})
	if got := (localAssignmentSourceTypeResolution{}).MismatchBoundary(reader); got == nil {
		t.Fatalf("plain source should keep mismatch boundary reader")
	}
	if got := (localAssignmentSourceTypeResolution{DeclarationProjection: true}).MismatchBoundary(reader); got != nil {
		t.Fatalf("declaration projection mismatch boundary = %#v, want nil", got)
	}
	if got := (localAssignmentSourceTypeResolution{OptionalIndexProjection: true}).MismatchBoundary(reader); got != nil {
		t.Fatalf("optional-index projection mismatch boundary = %#v, want nil", got)
	}
}
