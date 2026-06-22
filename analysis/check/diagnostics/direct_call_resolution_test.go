package diagnostics

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestDirectCallArgumentSourceTypeResolutionLiteral(t *testing.T) {
	resolution := resolveDirectCallArgumentSourceType(nil, nil, 0, guardEnv{}, semantics.CallFact{}, 0, &ast.StringExpr{Value: "ready"}, producerContext{}, nil)
	if !resolution.OK || !subtype.IsSubtype(resolution.Type, typ.String) {
		t.Fatalf("resolution = %#v, want string-compatible literal argument", resolution)
	}
	if resolution.UntrustedTopLike {
		t.Fatalf("literal resolution = %#v, want ordinary argument source", resolution)
	}
	if resolution.ReadBoundary == nil {
		t.Fatalf("literal resolution should preserve the argument boundary reader")
	}
	if resolution.TypeMismatch(nil, 0, typ.String) {
		t.Fatalf("string literal passed to string parameter should not mismatch")
	}
}

func TestDirectCallArgumentSourceTypeResolutionConcreteCastUsesProofSource(t *testing.T) {
	inner := &ast.IdentExpr{Value: "payload"}
	resolution := resolveDirectCallArgumentSourceType(nil, nil, 0, guardEnv{}, semantics.CallFact{}, 0, &ast.CastExpr{
		Expr: inner,
		Type: &ast.PrimitiveTypeExpr{Name: "string"},
	}, producerContext{}, nil)
	if !resolution.OK {
		t.Fatalf("resolution = %#v, want concrete cast argument", resolution)
	}
	if !resolution.UntrustedTopLike {
		t.Fatalf("resolution = %#v, want proof mismatch path for concrete cast", resolution)
	}
	if resolution.ReadBoundary == nil {
		t.Fatalf("concrete cast resolution should preserve an inner-value boundary reader")
	}
	if !resolution.TypeMismatch(nil, 0, typ.String) {
		t.Fatalf("cast obligation should still require proof against string")
	}
}
