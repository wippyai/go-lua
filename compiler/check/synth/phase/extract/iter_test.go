package extract

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/typ"
)

func TestInferIterVars_Empty(t *testing.T) {
	s := newTestSynthesizer()
	result := s.inferIterVars(nil, 2, 0, nil)
	if result != nil {
		t.Fatal("expected nil for empty exprs")
	}
}

func TestInferIterVars_UnknownType(t *testing.T) {
	s := newTestSynthesizer()
	exprs := []ast.Expr{&ast.IdentExpr{Value: "unknown"}}
	result := s.inferIterVars(exprs, 2, 0, nil)
	if len(result) != 2 {
		t.Fatalf("got %d types, want 2", len(result))
	}
	if result[0] != typ.Unknown {
		t.Fatal("expected unknown type")
	}
}

func TestInferIterVars_FunctionReturn(t *testing.T) {
	s, ident := newTestSynthesizerWithSymbol("iter", typ.Func().
		Returns(typ.Integer, typ.String).
		Build())

	exprs := []ast.Expr{ident}
	result := s.inferIterVars(exprs, 3, 0, nil)
	if len(result) != 3 {
		t.Fatalf("got %d types, want 3", len(result))
	}
	if result[0] != typ.Integer {
		t.Fatalf("got %v, want integer", result[0])
	}
	if result[1] != typ.String {
		t.Fatalf("got %v, want string", result[1])
	}
	if result[2] != typ.Unknown {
		t.Fatalf("got %v, want unknown for padding", result[2])
	}
}

func TestInferIterVarsWithSpec_Empty(t *testing.T) {
	s := newTestSynthesizer()
	specTypes := make(api.SpecTypes)
	result := s.inferIterVarsWithSpec(nil, 2, 0, specTypes)
	if result != nil {
		t.Fatal("expected nil for empty exprs")
	}
}

func TestInferIterVarsWithSpec_Unknown(t *testing.T) {
	s := newTestSynthesizer()
	specTypes := make(api.SpecTypes)
	exprs := []ast.Expr{&ast.IdentExpr{Value: "unknown"}}
	result := s.inferIterVarsWithSpec(exprs, 2, 0, specTypes)
	if len(result) != 2 {
		t.Fatalf("got %d types, want 2", len(result))
	}
}

func TestInferIterVarsFromCallCore_NonFunction(t *testing.T) {
	s := newTestSynthesizer()
	call := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "unknown"},
	}
	synthOne := func(expr ast.Expr) typ.Type { return s.SynthExpr(expr, 0, nil) }
	result := s.inferIterVarsFromCallCore(call, 2, synthOne)
	if result != nil {
		t.Fatal("expected nil for non-function")
	}
}

func TestInferIterVarsFromCallCore_WithSpec_NonFunction(t *testing.T) {
	s := newTestSynthesizer()
	specTypes := make(api.SpecTypes)
	call := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "unknown"},
	}
	synthOne := func(expr ast.Expr) typ.Type { return s.synthExprWithSpec(expr, 0, specTypes) }
	result := s.inferIterVarsFromCallCore(call, 2, synthOne)
	if result != nil {
		t.Fatal("expected nil for non-function")
	}
}
