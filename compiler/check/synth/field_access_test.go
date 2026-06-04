package synth

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/typ"
)

func TestFieldAccessResult_Zero(t *testing.T) {
	var r FieldAccessResult
	if r.Found || r.SkipCheck || r.NotIndexable || r.Type != nil {
		t.Fatal("zero FieldAccessResult should have all false/nil")
	}
}

func TestFieldAccessResult_WithType(t *testing.T) {
	r := FieldAccessResult{Type: typ.String, Found: true}
	if !r.Found {
		t.Fatal("expected Found=true")
	}
	if r.Type != typ.String {
		t.Fatal("expected string type")
	}
}

func TestResolveFieldAccess_NilObjType(t *testing.T) {
	e := newTestEngine()
	result := e.ResolveFieldAccess(nil, nil, "field", cfg.Point(0))
	if !result.SkipCheck {
		t.Fatal("expected SkipCheck for nil obj type")
	}
}

func TestResolveFieldAccess_AnyType(t *testing.T) {
	e := newTestEngine()
	result := e.ResolveFieldAccess(nil, typ.Any, "field", cfg.Point(0))
	if !result.SkipCheck {
		t.Fatal("expected SkipCheck for any type")
	}
}

func TestResolveFieldAccess_UnknownType(t *testing.T) {
	e := newTestEngine()
	result := e.ResolveFieldAccess(nil, typ.Unknown, "field", cfg.Point(0))
	if !result.SkipCheck {
		t.Fatal("expected SkipCheck for unknown type")
	}
}

func TestResolveFieldAccess_RecordField(t *testing.T) {
	e := newTestEngine()
	rec := typ.NewRecord().Field("name", typ.String).Build()
	result := e.ResolveFieldAccess(nil, rec, "name", cfg.Point(0))
	if !result.Found {
		t.Fatal("expected field to be found")
	}
	if result.Type != typ.String {
		t.Fatal("expected string type")
	}
}

func TestResolveFieldAccess_RecordFieldNotFound(t *testing.T) {
	e := newTestEngine()
	rec := typ.NewRecord().Field("name", typ.String).Build()
	result := e.ResolveFieldAccess(nil, rec, "age", cfg.Point(0))
	if result.Found {
		t.Fatal("expected field not found")
	}
}

func TestResolveFieldAccess_MissingTableFieldTrustsPresentFullPathValue(t *testing.T) {
	rec := typ.NewRecord().Field("name", typ.String).Build()
	expr := &ast.AttrGetExpr{
		Object: &ast.IdentExpr{Value: "obj"},
		Key:    &ast.StringExpr{Value: "installed"},
	}
	resolver := presentFieldAccessResolver{
		fullType: typ.NewArray(typ.String),
		present:  true,
	}

	result := ResolveFieldAccess(resolver, expr, rec, "installed", cfg.Point(0))
	if !result.Found || !result.SkipCheck {
		t.Fatalf("expected present full-path value to satisfy missing table field access, got %#v", result)
	}
	if !typ.TypeEquals(result.Type, typ.NewArray(typ.String)) {
		t.Fatalf("resolved type = %v, want {string}", result.Type)
	}
}

func TestResolveFieldAccess_MissingTableFieldTrustsPresentFullPathAny(t *testing.T) {
	rec := typ.NewRecord().Field("name", typ.String).Build()
	expr := &ast.AttrGetExpr{
		Object: &ast.IdentExpr{Value: "obj"},
		Key:    &ast.StringExpr{Value: "installed"},
	}
	resolver := presentFieldAccessResolver{
		fullType: typ.Any,
		present:  true,
	}

	result := ResolveFieldAccess(resolver, expr, rec, "installed", cfg.Point(0))
	if !result.Found || !result.SkipCheck {
		t.Fatalf("expected present full-path any to satisfy missing table field access, got %#v", result)
	}
	if !typ.TypeEquals(result.Type, typ.Any) {
		t.Fatalf("resolved type = %v, want any", result.Type)
	}
}

func TestResolveFieldAccess_MissingTableFieldRejectsUnprovedFullPathType(t *testing.T) {
	rec := typ.NewRecord().Field("name", typ.String).Build()
	expr := &ast.AttrGetExpr{
		Object: &ast.IdentExpr{Value: "obj"},
		Key:    &ast.StringExpr{Value: "missing"},
	}
	resolver := presentFieldAccessResolver{
		fullType: typ.String,
		present:  false,
	}

	result := ResolveFieldAccess(resolver, expr, rec, "missing", cfg.Point(0))
	if result.Found || result.SkipCheck {
		t.Fatalf("expected unproved missing table field to remain missing, got %#v", result)
	}
}

func TestResolveFieldAccess_EmptyFieldNameMap(t *testing.T) {
	e := newTestEngine()
	m := typ.NewMap(typ.String, typ.Integer)
	result := e.ResolveFieldAccess(nil, m, "", cfg.Point(0))
	if !result.SkipCheck {
		t.Fatal("expected SkipCheck for map with empty field")
	}
}

func TestResolveFieldAccess_EmptyFieldNameArray(t *testing.T) {
	e := newTestEngine()
	arr := typ.NewArray(typ.String)
	result := e.ResolveFieldAccess(nil, arr, "", cfg.Point(0))
	if !result.SkipCheck {
		t.Fatal("expected SkipCheck for array with empty field")
	}
}

func TestResolveFieldAccess_MapWithFieldName(t *testing.T) {
	e := newTestEngine()
	m := typ.NewMap(typ.String, typ.Integer)
	result := e.ResolveFieldAccess(nil, m, "key", cfg.Point(0))
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

func TestResolveFieldAccess_ArrayWithFieldName(t *testing.T) {
	e := newTestEngine()
	arr := typ.NewArray(typ.String)
	result := e.ResolveFieldAccess(nil, arr, "index", cfg.Point(0))
	if result.SkipCheck {
		t.Fatal("expected array field lookup to be checked")
	}
	if result.Found {
		t.Fatal("expected field not found on array")
	}
}

func TestResolveFieldAccess_IdentKey(t *testing.T) {
	e := newTestEngine()
	rec := typ.NewRecord().Field("name", typ.String).Build()
	ex := &ast.AttrGetExpr{
		Object: &ast.IdentExpr{Value: "obj"},
		Key:    &ast.IdentExpr{Value: "name"},
	}
	result := e.ResolveFieldAccess(ex, rec, "name", cfg.Point(0))
	if !result.SkipCheck {
		t.Fatal("expected SkipCheck for ident key")
	}
}

func TestResolveFieldAccess_EmptyFieldNamePrimitive(t *testing.T) {
	e := newTestEngine()
	result := e.ResolveFieldAccess(nil, typ.String, "", cfg.Point(0))
	if !result.NotIndexable {
		t.Fatal("expected NotIndexable for primitive with empty field")
	}
}

func TestResolveFieldAccess_TypeParam(t *testing.T) {
	e := newTestEngine()
	tp := typ.NewTypeParam("T", nil)
	result := e.ResolveFieldAccess(nil, tp, "field", cfg.Point(0))
	if !result.SkipCheck {
		t.Fatal("expected SkipCheck for type param")
	}
}

func TestResolveFieldAccess_EmptyFieldTypeParam(t *testing.T) {
	e := newTestEngine()
	tp := typ.NewTypeParam("T", nil)
	result := e.ResolveFieldAccess(nil, tp, "", cfg.Point(0))
	if !result.SkipCheck {
		t.Fatal("expected SkipCheck for type param with empty field")
	}
}

func TestResolveFieldAccess_UnexpandedRecursivePlaceholder(t *testing.T) {
	e := newTestEngine()
	rec := typ.NewRecursivePlaceholder("Inferred")

	result := e.ResolveFieldAccess(nil, rec, "field", cfg.Point(0))
	if !result.SkipCheck {
		t.Fatal("expected SkipCheck for unexpanded recursive placeholder")
	}
}

type presentFieldAccessResolver struct {
	fullType typ.Type
	fields   map[string]typ.Type
	present  bool
}

func (r presentFieldAccessResolver) TypeOf(ast.Expr, cfg.Point) typ.Type {
	return r.fullType
}

func (r presentFieldAccessResolver) Field(_ typ.Type, name string) (typ.Type, bool) {
	if r.fields == nil {
		return nil, false
	}
	t, ok := r.fields[name]
	return t, ok
}

func (r presentFieldAccessResolver) FieldAccessHasPresentValue(*ast.AttrGetExpr, cfg.Point) bool {
	return r.present
}
