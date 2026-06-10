package fieldaccess

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/types/cfg"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

func TestResult_Zero(t *testing.T) {
	var r Result
	if r.Found || r.SkipCheck || r.NotIndexable || r.Type != nil {
		t.Fatal("zero Result should have all false/nil")
	}
}

func TestResult_WithType(t *testing.T) {
	r := Result{Type: typ.String, Found: true}
	if !r.Found {
		t.Fatal("expected Found=true")
	}
	if r.Type != typ.String {
		t.Fatal("expected string type")
	}
}

func TestResolve_NilObjType(t *testing.T) {
	result := Resolve(testResolver{}, nil, nil, "field", cfg.Point(0))
	if !result.SkipCheck {
		t.Fatal("expected SkipCheck for nil obj type")
	}
}

func TestResolve_AnyType(t *testing.T) {
	result := Resolve(testResolver{}, nil, typ.Any, "field", cfg.Point(0))
	if !result.SkipCheck {
		t.Fatal("expected SkipCheck for any type")
	}
}

func TestResolve_UnknownType(t *testing.T) {
	result := Resolve(testResolver{}, nil, typ.Unknown, "field", cfg.Point(0))
	if !result.SkipCheck {
		t.Fatal("expected SkipCheck for unknown type")
	}
}

func TestResolve_RecordField(t *testing.T) {
	rec := typ.NewRecord().Field("name", typ.String).Build()
	result := Resolve(testResolver{}, nil, rec, "name", cfg.Point(0))
	if !result.Found {
		t.Fatal("expected field to be found")
	}
	if result.Type != typ.String {
		t.Fatal("expected string type")
	}
}

func TestResolve_RecordFieldNotFound(t *testing.T) {
	rec := typ.NewRecord().Field("name", typ.String).Build()
	result := Resolve(testResolver{}, nil, rec, "age", cfg.Point(0))
	if result.Found {
		t.Fatal("expected field not found")
	}
}

func TestResolve_MissingTableFieldTrustsPresentFullPathValue(t *testing.T) {
	rec := typ.NewRecord().Field("name", typ.String).Build()
	expr := &ast.AttrGetExpr{
		Object: &ast.IdentExpr{Value: "obj"},
		Key:    &ast.StringExpr{Value: "installed"},
	}
	resolver := testResolver{
		fullType: typ.NewArray(typ.String),
		present:  true,
	}

	result := Resolve(resolver, expr, rec, "installed", cfg.Point(0))
	if !result.Found || !result.SkipCheck {
		t.Fatalf("expected present full-path value to satisfy missing table field access, got %#v", result)
	}
	if !typ.TypeEquals(result.Type, typ.NewArray(typ.String)) {
		t.Fatalf("resolved type = %v, want {string}", result.Type)
	}
}

func TestResolve_MissingTableFieldTrustsPresentFullPathAny(t *testing.T) {
	rec := typ.NewRecord().Field("name", typ.String).Build()
	expr := &ast.AttrGetExpr{
		Object: &ast.IdentExpr{Value: "obj"},
		Key:    &ast.StringExpr{Value: "installed"},
	}
	resolver := testResolver{
		fullType: typ.Any,
		present:  true,
	}

	result := Resolve(resolver, expr, rec, "installed", cfg.Point(0))
	if !result.Found || !result.SkipCheck {
		t.Fatalf("expected present full-path any to satisfy missing table field access, got %#v", result)
	}
	if !typ.TypeEquals(result.Type, typ.Any) {
		t.Fatalf("resolved type = %v, want any", result.Type)
	}
}

func TestResolve_MissingTableFieldRejectsUnprovedFullPathType(t *testing.T) {
	rec := typ.NewRecord().Field("name", typ.String).Build()
	expr := &ast.AttrGetExpr{
		Object: &ast.IdentExpr{Value: "obj"},
		Key:    &ast.StringExpr{Value: "missing"},
	}
	resolver := testResolver{
		fullType: typ.String,
		present:  false,
	}

	result := Resolve(resolver, expr, rec, "missing", cfg.Point(0))
	if result.Found || result.SkipCheck {
		t.Fatalf("expected unproved missing table field to remain missing, got %#v", result)
	}
}

func TestResolve_EmptyFieldNameMap(t *testing.T) {
	m := typ.NewMap(typ.String, typ.Integer)
	result := Resolve(testResolver{}, nil, m, "", cfg.Point(0))
	if !result.SkipCheck {
		t.Fatal("expected SkipCheck for map with empty field")
	}
}

func TestResolve_EmptyFieldNameArray(t *testing.T) {
	arr := typ.NewArray(typ.String)
	result := Resolve(testResolver{}, nil, arr, "", cfg.Point(0))
	if !result.SkipCheck {
		t.Fatal("expected SkipCheck for array with empty field")
	}
}

func TestResolve_MapWithFieldName(t *testing.T) {
	m := typ.NewMap(typ.String, typ.Integer)
	result := Resolve(testResolver{}, nil, m, "key", cfg.Point(0))
	if result.SkipCheck {
		t.Fatal("expected map field lookup to be checked")
	}
	if !result.Found {
		t.Fatal("expected field to be found on map")
	}
	opt, ok := result.Type.(*typ.Optional)
	if !ok || opt.Inner != typ.Integer {
		t.Fatal("expected optional integer type for map field")
	}
}

func TestResolve_ArrayWithFieldName(t *testing.T) {
	arr := typ.NewArray(typ.String)
	result := Resolve(testResolver{}, nil, arr, "index", cfg.Point(0))
	if result.SkipCheck {
		t.Fatal("expected array field lookup to be checked")
	}
	if result.Found {
		t.Fatal("expected field not found on array")
	}
}

func TestResolve_IdentKey(t *testing.T) {
	rec := typ.NewRecord().Field("name", typ.String).Build()
	ex := &ast.AttrGetExpr{
		Object: &ast.IdentExpr{Value: "obj"},
		Key:    &ast.IdentExpr{Value: "name"},
	}
	result := Resolve(testResolver{}, ex, rec, "name", cfg.Point(0))
	if !result.SkipCheck {
		t.Fatal("expected SkipCheck for ident key")
	}
}

func TestResolve_EmptyFieldNamePrimitive(t *testing.T) {
	result := Resolve(testResolver{}, nil, typ.String, "", cfg.Point(0))
	if !result.NotIndexable {
		t.Fatal("expected NotIndexable for primitive with empty field")
	}
}

func TestResolve_TypeParam(t *testing.T) {
	tp := typ.NewTypeParam("T", nil)
	result := Resolve(testResolver{}, nil, tp, "field", cfg.Point(0))
	if !result.SkipCheck {
		t.Fatal("expected SkipCheck for type param")
	}
}

func TestResolve_EmptyFieldTypeParam(t *testing.T) {
	tp := typ.NewTypeParam("T", nil)
	result := Resolve(testResolver{}, nil, tp, "", cfg.Point(0))
	if !result.SkipCheck {
		t.Fatal("expected SkipCheck for type param with empty field")
	}
}

func TestResolve_UnexpandedRecursivePlaceholder(t *testing.T) {
	rec := typ.NewRecursivePlaceholder("Inferred")

	result := Resolve(testResolver{}, nil, rec, "field", cfg.Point(0))
	if !result.SkipCheck {
		t.Fatal("expected SkipCheck for unexpanded recursive placeholder")
	}
}

type testResolver struct {
	fullType typ.Type
	fields   map[string]typ.Type
	present  bool
}

func (r testResolver) TypeOf(ast.Expr, cfg.Point) typ.Type {
	return r.fullType
}

func (r testResolver) Field(t typ.Type, name string) (typ.Type, bool) {
	if r.fields != nil {
		fieldType, ok := r.fields[name]
		return fieldType, ok
	}
	return querycore.Field(t, name)
}

func (r testResolver) FieldAccessHasPresentValue(*ast.AttrGetExpr, cfg.Point) bool {
	return r.present
}
