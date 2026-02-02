package core

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/typ"
)

func TestParamListConfig(t *testing.T) {
	cfg := ParamListConfig{
		ResolveType:  nil,
		ResolveScope: nil,
		Expected:     nil,
	}
	if cfg.ResolveType != nil {
		t.Error("expected nil ResolveType")
	}
	if cfg.ResolveScope != nil {
		t.Error("expected nil ResolveScope")
	}
	if cfg.Expected != nil {
		t.Error("expected nil Expected")
	}
}

func TestApplyParamList_NilBuilder(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{},
	}
	ApplyParamList(nil, fn, ParamListConfig{})
}

func TestApplyParamList_NilFn(t *testing.T) {
	builder := typ.Func()
	ApplyParamList(builder, nil, ParamListConfig{})
}

func TestApplyParamList_NilParList(t *testing.T) {
	builder := typ.Func()
	fn := &ast.FunctionExpr{
		ParList: nil,
	}
	ApplyParamList(builder, fn, ParamListConfig{})
}

func TestApplyParamList_EmptyParList(t *testing.T) {
	builder := typ.Func()
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names: []string{},
		},
	}
	ApplyParamList(builder, fn, ParamListConfig{})
	result := builder.Build()
	if result == nil {
		t.Fatal("expected non-nil function type")
	}
	if len(result.Params) != 0 {
		t.Errorf("expected 0 params, got %d", len(result.Params))
	}
}

func TestApplyParamList_UntypedParams(t *testing.T) {
	builder := typ.Func()
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names: []string{"a", "b"},
		},
	}
	ApplyParamList(builder, fn, ParamListConfig{})
	result := builder.Build()
	if result == nil {
		t.Fatal("expected non-nil function type")
	}
	if len(result.Params) != 2 {
		t.Errorf("expected 2 params, got %d", len(result.Params))
	}
	for i, p := range result.Params {
		if !p.Optional {
			t.Errorf("param %d: expected optional (untyped params default to optional)", i)
		}
	}
	if result.Variadic == nil {
		t.Error("expected variadic type for untyped function")
	}
}

func TestApplyParamList_Varargs(t *testing.T) {
	builder := typ.Func()
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names:    []string{},
			HasVargs: true,
		},
	}
	ApplyParamList(builder, fn, ParamListConfig{})
	result := builder.Build()
	if result == nil {
		t.Fatal("expected non-nil function type")
	}
	if result.Variadic == nil {
		t.Error("expected non-nil variadic type")
	}
}

func TestApplyParamList_WithExpected(t *testing.T) {
	expected := typ.Func().
		Param("x", typ.Number).
		OptParam("y", typ.String).
		Build()

	builder := typ.Func()
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names: []string{"x", "y"},
		},
	}
	ApplyParamList(builder, fn, ParamListConfig{
		Expected: expected,
	})
	result := builder.Build()
	if result == nil {
		t.Fatal("expected non-nil function type")
	}
	if len(result.Params) != 2 {
		t.Errorf("expected 2 params, got %d", len(result.Params))
	}
	if result.Params[0].Type != typ.Number {
		t.Errorf("expected first param type Number, got %v", result.Params[0].Type)
	}
	if result.Params[1].Type != typ.String {
		t.Errorf("expected second param type String, got %v", result.Params[1].Type)
	}
}

func TestApplyParamList_WithResolveType(t *testing.T) {
	sc := scope.New()
	resolveType := func(expr ast.TypeExpr, s *scope.State) typ.Type {
		if _, ok := expr.(*ast.PrimitiveTypeExpr); ok {
			return typ.Boolean
		}
		return nil
	}

	builder := typ.Func()
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names: []string{"flag"},
			Types: []ast.TypeExpr{&ast.PrimitiveTypeExpr{Name: "boolean"}},
		},
	}
	ApplyParamList(builder, fn, ParamListConfig{
		ResolveType:  resolveType,
		ResolveScope: sc,
	})
	result := builder.Build()
	if result == nil {
		t.Fatal("expected non-nil function type")
	}
	if len(result.Params) != 1 {
		t.Errorf("expected 1 param, got %d", len(result.Params))
	}
	if result.Params[0].Type != typ.Boolean {
		t.Errorf("expected param type Boolean, got %v", result.Params[0].Type)
	}
}

func TestApplyParamList_SoftAnnotationUsesExpected(t *testing.T) {
	sc := scope.New()
	resolveType := func(expr ast.TypeExpr, s *scope.State) typ.Type {
		if arr, ok := expr.(*ast.ArrayTypeExpr); ok && arr.Element != nil {
			return typ.NewArray(typ.Any)
		}
		return nil
	}

	builder := typ.Func()
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names: []string{"items"},
			Types: []ast.TypeExpr{&ast.ArrayTypeExpr{
				Element: &ast.PrimitiveTypeExpr{Name: "any"},
			}},
		},
	}
	expected := typ.Func().Param("items", typ.String).Build()
	ApplyParamList(builder, fn, ParamListConfig{
		ResolveType:  resolveType,
		ResolveScope: sc,
		Expected:     expected,
	})
	result := builder.Build()
	if result == nil {
		t.Fatal("expected non-nil function type")
	}
	if len(result.Params) != 1 {
		t.Fatalf("expected 1 param, got %d", len(result.Params))
	}
	if result.Params[0].Type != typ.String {
		t.Fatalf("expected param type String, got %v", result.Params[0].Type)
	}
}
