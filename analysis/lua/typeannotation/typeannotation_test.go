package typeannotation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/annotation"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

type resolver map[string]typ.Type

func (r resolver) ResolveTypeRef(path []string) (typ.Type, bool) {
	if len(path) == 0 {
		return nil, false
	}
	key := path[0]
	for _, part := range path[1:] {
		key += "." + part
	}
	t, ok := r[key]
	return t, ok
}

func TestTypePrimitivesAndRefs(t *testing.T) {
	tests := []struct {
		name string
		expr ast.TypeExpr
		want typ.Type
	}{
		{name: "nil", expr: &ast.PrimitiveTypeExpr{Name: "nil"}, want: typ.Nil},
		{name: "boolean", expr: &ast.PrimitiveTypeExpr{Name: "boolean"}, want: typ.Boolean},
		{name: "number", expr: &ast.PrimitiveTypeExpr{Name: "number"}, want: typ.Number},
		{name: "integer", expr: &ast.PrimitiveTypeExpr{Name: "integer"}, want: typ.Integer},
		{name: "string", expr: &ast.PrimitiveTypeExpr{Name: "string"}, want: typ.String},
		{name: "any", expr: &ast.PrimitiveTypeExpr{Name: "any"}, want: typ.Any},
		{name: "unknown", expr: &ast.PrimitiveTypeExpr{Name: "unknown"}, want: typ.Unknown},
		{name: "never", expr: &ast.PrimitiveTypeExpr{Name: "never"}, want: typ.Never},
		{name: "self primitive", expr: &ast.PrimitiveTypeExpr{Name: "self"}, want: typ.Self},
		{name: "self node", expr: &ast.SelfTypeExpr{}, want: typ.Self},
		{name: "bare ref", expr: &ast.PrimitiveTypeExpr{Name: "User"}, want: typ.NewRef("", "User")},
		{name: "path ref", expr: &ast.TypeRefExpr{Path: []string{"http", "Request"}}, want: typ.NewRef("http", "Request")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Type(tt.expr, nil)
			if !ok {
				t.Fatal("Type returned ok=false")
			}
			if !got.Equals(tt.want) {
				t.Fatalf("Type() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestTypeResolver(t *testing.T) {
	named := typ.RebuildRecord(typ.RecordParts{Fields: []typ.Field{{Name: "id", Type: typ.String}}})
	got, ok := Type(&ast.TypeRefExpr{Path: []string{"models", "User"}}, resolver{"models.User": named})
	if !ok {
		t.Fatal("Type returned ok=false")
	}
	if got != named {
		t.Fatalf("Type() = %v, want resolver result", got)
	}
}

func TestTypeComposites(t *testing.T) {
	got, ok := Type(&ast.TupleTypeExpr{
		Elements: []ast.TypeExpr{
			&ast.OptionalTypeExpr{Inner: &ast.PrimitiveTypeExpr{Name: "string"}},
			&ast.UnionTypeExpr{Types: []ast.TypeExpr{
				&ast.PrimitiveTypeExpr{Name: "number"},
				&ast.PrimitiveTypeExpr{Name: "boolean"},
			}},
			&ast.IntersectionTypeExpr{Types: []ast.TypeExpr{
				&ast.TypeRefExpr{Path: []string{"A"}},
				&ast.TypeRefExpr{Path: []string{"B"}},
			}},
			&ast.ArrayTypeExpr{Element: &ast.PrimitiveTypeExpr{Name: "integer"}},
			&ast.MapTypeExpr{Key: &ast.PrimitiveTypeExpr{Name: "string"}, Value: &ast.PrimitiveTypeExpr{Name: "number"}},
			&ast.MapTypeExpr{Key: &ast.PrimitiveTypeExpr{Name: "string"}, Value: &ast.PrimitiveTypeExpr{Name: "boolean"}, Readonly: true},
		},
	}, nil)
	if !ok {
		t.Fatal("Type returned ok=false")
	}
	tuple, ok := got.(*typ.Tuple)
	if !ok {
		t.Fatalf("Type() = %T, want *typ.Tuple", got)
	}
	if len(tuple.Elements) != 6 {
		t.Fatalf("tuple length = %d, want 6", len(tuple.Elements))
	}
	if tuple.Elements[0].Kind() != kind.Optional {
		t.Fatalf("element 0 kind = %s, want optional", tuple.Elements[0].Kind())
	}
	if tuple.Elements[1].Kind() != kind.Union {
		t.Fatalf("element 1 kind = %s, want union", tuple.Elements[1].Kind())
	}
	if tuple.Elements[2].Kind() != kind.Intersection {
		t.Fatalf("element 2 kind = %s, want intersection", tuple.Elements[2].Kind())
	}
	if tuple.Elements[3].Kind() != kind.Array {
		t.Fatalf("element 3 kind = %s, want array", tuple.Elements[3].Kind())
	}
	if tuple.Elements[4].Kind() != kind.Map {
		t.Fatalf("element 4 kind = %s, want map", tuple.Elements[4].Kind())
	}
	if tuple.Elements[5].Kind() != kind.ReadonlyMap {
		t.Fatalf("element 5 kind = %s, want readonly map", tuple.Elements[5].Kind())
	}
}

func TestTypeRecordReadonlyAndFieldAnnotations(t *testing.T) {
	got, ok := Type(&ast.RecordTypeExpr{
		Readonly: true,
		Fields: []ast.RecordFieldExpr{{
			Name:     "name",
			Type:     &ast.PrimitiveTypeExpr{Name: "string"},
			Optional: true,
			Annotations: []ast.AnnotationExpr{{
				Name: "min_len",
				Args: []ast.Expr{&ast.NumberExpr{Value: "1"}},
			}},
		}},
	}, nil)
	if !ok {
		t.Fatal("Type returned ok=false")
	}
	rec, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("Type() = %T, want *typ.Record", got)
	}
	field := rec.GetField("name")
	if field == nil {
		t.Fatal("missing name field")
	}
	if !field.Optional || !field.Readonly {
		t.Fatalf("field flags optional=%v readonly=%v, want true true", field.Optional, field.Readonly)
	}
	ann, ok := field.Type.(*typ.Annotated)
	if !ok {
		t.Fatalf("field type = %T, want *typ.Annotated", field.Type)
	}
	if len(ann.Annotations) != 1 || !ann.Annotations[0].Equal(annotation.Annotation{Name: "min_len", Arg: int64(1)}) {
		t.Fatalf("annotations = %#v, want min_len(1)", ann.Annotations)
	}
}

func TestTypeFunctionTypeParamsVariadicAndReturns(t *testing.T) {
	got, ok := Type(&ast.FunctionTypeExpr{
		TypeParams: []ast.TypeParamExpr{{Name: "T", Constraint: &ast.PrimitiveTypeExpr{Name: "string"}}},
		Params:     []ast.FunctionParamExpr{{Name: "value", Type: &ast.TypeRefExpr{Path: []string{"T"}}}},
		Variadic:   &ast.PrimitiveTypeExpr{Name: "number"},
		Returns:    []ast.TypeExpr{&ast.TypeRefExpr{Path: []string{"T"}}},
	}, nil)
	if !ok {
		t.Fatal("Type returned ok=false")
	}
	fn, ok := got.(*typ.Function)
	if !ok {
		t.Fatalf("Type() = %T, want *typ.Function", got)
	}
	if len(fn.TypeParams) != 1 || fn.TypeParams[0].Name != "T" || !fn.TypeParams[0].Constraint.Equals(typ.String) {
		t.Fatalf("type params = %#v, want T constrained to string", fn.TypeParams)
	}
	if len(fn.Params) != 1 || fn.Params[0].Type != fn.TypeParams[0] {
		t.Fatalf("param type = %#v, want declared T node", fn.Params)
	}
	if fn.Variadic == nil || !fn.Variadic.Equals(typ.Number) {
		t.Fatalf("variadic = %v, want number", fn.Variadic)
	}
	if len(fn.Returns) != 1 || fn.Returns[0] != fn.TypeParams[0] {
		t.Fatalf("returns = %#v, want declared T node", fn.Returns)
	}
}

func TestTypeGenericInstantiation(t *testing.T) {
	param := typ.NewTypeParam("T", nil)
	generic := typ.NewGeneric("Box", []*typ.TypeParam{param}, typ.NewArray(param))

	got, ok := Type(&ast.GenericTypeExpr{
		Base: &ast.TypeRefExpr{Path: []string{"Box"}},
		Args: []ast.TypeExpr{&ast.PrimitiveTypeExpr{Name: "string"}},
	}, resolver{"Box": generic})
	if !ok {
		t.Fatal("Type returned ok=false")
	}
	inst, ok := got.(*typ.Instantiated)
	if !ok {
		t.Fatalf("Type() = %T, want *typ.Instantiated", got)
	}
	if inst.Generic != generic || len(inst.TypeArgs) != 1 || !inst.TypeArgs[0].Equals(typ.String) {
		t.Fatalf("instantiation = %#v", inst)
	}
}

func TestTypeGenericInstantiationRejectsUnresolvedBase(t *testing.T) {
	got, ok := Type(&ast.GenericTypeExpr{
		Base: &ast.TypeRefExpr{Path: []string{"Box"}},
		Args: []ast.TypeExpr{&ast.PrimitiveTypeExpr{Name: "string"}},
	}, nil)
	if ok || got != nil {
		t.Fatalf("Type(unresolved generic) = %v/%v, want nil/false", got, ok)
	}
}

func TestTypeLiteralsAndMeta(t *testing.T) {
	got, ok := Type(&ast.TupleTypeExpr{Elements: []ast.TypeExpr{
		&ast.LiteralTypeExpr{Value: "red"},
		&ast.LiteralTypeExpr{Value: int64(42)},
		&ast.LiteralTypeExpr{Value: float64(1.5)},
		&ast.LiteralTypeExpr{Value: true},
		&ast.MetaTypeExpr{Inner: &ast.PrimitiveTypeExpr{Name: "string"}},
	}}, nil)
	if !ok {
		t.Fatal("Type returned ok=false")
	}
	tuple := got.(*typ.Tuple)
	if tuple.Elements[0].Kind() != kind.Literal ||
		tuple.Elements[1].Kind() != kind.Literal ||
		tuple.Elements[2].Kind() != kind.Literal ||
		tuple.Elements[3].Kind() != kind.Literal ||
		tuple.Elements[4].Kind() != kind.Meta {
		t.Fatalf("unexpected tuple element kinds: %#v", tuple.Elements)
	}
}

func TestAnnotationsRejectUnsupportedArgs(t *testing.T) {
	anns, ok := Annotations([]ast.AnnotationExpr{{
		Name: "hex",
		Args: []ast.Expr{&ast.NumberExpr{Value: "0x10"}},
	}})
	if !ok || len(anns) != 1 || !anns[0].Equal(annotation.Annotation{Name: "hex", Arg: int64(16)}) {
		t.Fatalf("Annotations(hex) = %#v/%v, want int64(16)", anns, ok)
	}

	_, ok = Annotations([]ast.AnnotationExpr{{
		Name: "range",
		Args: []ast.Expr{&ast.NumberExpr{Value: "1"}, &ast.NumberExpr{Value: "10"}},
	}})
	if ok {
		t.Fatal("Annotations accepted multi-arg annotation")
	}

	_, ok = Annotations([]ast.AnnotationExpr{{
		Name: "computed",
		Args: []ast.Expr{&ast.IdentExpr{Value: "limit"}},
	}})
	if ok {
		t.Fatal("Annotations accepted non-literal annotation arg")
	}
}

func TestTypeUnsupportedAdvancedExpressions(t *testing.T) {
	exprs := []ast.TypeExpr{
		&ast.TypeOfExpr{Expr: &ast.IdentExpr{Value: "x"}},
		&ast.KeyOfExpr{Inner: &ast.TypeRefExpr{Path: []string{"User"}}},
		&ast.IndexAccessExpr{Object: &ast.TypeRefExpr{Path: []string{"User"}}, Index: &ast.LiteralTypeExpr{Value: "name"}},
		&ast.ConditionalTypeExpr{
			Check:   &ast.TypeRefExpr{Path: []string{"T"}},
			Extends: &ast.PrimitiveTypeExpr{Name: "string"},
			Then:    &ast.PrimitiveTypeExpr{Name: "number"},
			Else:    &ast.PrimitiveTypeExpr{Name: "boolean"},
		},
		&ast.AssertsTypeExpr{ParamName: "x", NarrowTo: &ast.PrimitiveTypeExpr{Name: "string"}},
		&ast.ArrayTypeExpr{Element: &ast.PrimitiveTypeExpr{Name: "string"}, Readonly: true},
	}
	for _, expr := range exprs {
		if got, ok := Type(expr, nil); ok {
			t.Fatalf("Type(%T) = %v, true; want nil, false", expr, got)
		}
	}
}
