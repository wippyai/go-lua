package diagnostics

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/type/subtype"
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

func TestPathAssignmentSourceTypeResolutionLiteral(t *testing.T) {
	resolution := resolvePathAssignmentSourceType(nil, producerContext{}, nil, nil, 0, semantics.OrdinaryAssignmentFact{
		Value: &ast.StringExpr{Value: "ready"},
	}, guardEnv{})
	if !resolution.OK || !subtype.IsSubtype(resolution.Type, typ.String) {
		t.Fatalf("resolution = %#v, want string-compatible literal source", resolution)
	}
	if resolution.UntrustedTopLike || resolution.CastInnerExpr != nil {
		t.Fatalf("literal resolution = %#v, want ordinary non-cast source", resolution)
	}
	if resolution.TypeMismatch(nil, 0, typ.String, nil) {
		t.Fatalf("string literal assigned to string should not mismatch")
	}
}

func TestPathAssignmentSourceTypeResolutionScalarCastUsesRuntimeValidation(t *testing.T) {
	inner := &ast.IdentExpr{Value: "payload"}
	resolution := resolvePathAssignmentSourceType(nil, producerContext{}, nil, nil, 0, semantics.OrdinaryAssignmentFact{
		Value: &ast.CastExpr{
			Expr: inner,
			Type: &ast.PrimitiveTypeExpr{Name: "string"},
		},
	}, guardEnv{})
	if !resolution.OK {
		t.Fatalf("resolution = %#v, want concrete cast source", resolution)
	}
	if resolution.UntrustedTopLike {
		t.Fatalf("resolution = %#v, want scalar cast runtime validation", resolution)
	}
	if resolution.CastInnerExpr != nil {
		t.Fatalf("cast inner = %#v, want no inner proof boundary for scalar runtime cast", resolution.CastInnerExpr)
	}
	if resolution.TypeMismatch(nil, 0, typ.String, nil) {
		t.Fatalf("scalar runtime cast assigned to string should not mismatch")
	}
}
