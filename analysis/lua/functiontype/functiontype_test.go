package functiontype

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func primitiveType(name string) *ast.PrimitiveTypeExpr {
	return &ast.PrimitiveTypeExpr{Name: name}
}

func resolvePrimitive(expr ast.TypeExpr) (typ.Type, bool) {
	prim, ok := expr.(*ast.PrimitiveTypeExpr)
	if !ok {
		return nil, false
	}
	switch prim.Name {
	case "string":
		return typ.String, true
	case "number":
		return typ.Number, true
	}
	return nil, false
}

func noDecl(bind.TypeDecl) (typ.Type, bool) { return nil, false }

// TestFromBindings_TypedParamsWithUntypedVariadic proves that a fully typed
// fixed parameter prefix keeps its declared types when the trailing `...` is
// untyped: function(a: string, b: number, ...) must retain both Params and
// only widen the variadic tail to Any.
func TestFromBindings_TypedParamsWithUntypedVariadic(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names:    []string{"a", "b"},
			Types:    []ast.TypeExpr{primitiveType("string"), primitiveType("number")},
			HasVargs: true,
		},
	}
	bindings := bind.BindFunction(fn, bind.Options{})

	got, ok := FromBindings(fn, bindings, resolvePrimitive, noDecl)
	if !ok {
		t.Fatalf("FromBindings ok = false, want true")
	}
	fnType, ok := got.(*typ.Function)
	if !ok {
		t.Fatalf("FromBindings type = %T, want *typ.Function", got)
	}
	if len(fnType.Params) != 2 {
		t.Fatalf("Params = %#v, want 2 typed fixed params (fixed prefix must not collapse)", fnType.Params)
	}
	if !fnType.Params[0].Type.Equals(typ.String) {
		t.Fatalf("Params[0].Type = %v, want string", fnType.Params[0].Type)
	}
	if !fnType.Params[1].Type.Equals(typ.Number) {
		t.Fatalf("Params[1].Type = %v, want number", fnType.Params[1].Type)
	}
	if fnType.Variadic == nil || !fnType.Variadic.Equals(typ.Any) {
		t.Fatalf("Variadic = %v, want Any (untyped tail)", fnType.Variadic)
	}
}

// TestFromBindings_TypedVariadicKeepsElementType proves a typed `...: T` tail
// preserves T rather than being widened to Any.
func TestFromBindings_TypedVariadicKeepsElementType(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names:      []string{"a"},
			Types:      []ast.TypeExpr{primitiveType("string")},
			HasVargs:   true,
			VarargType: primitiveType("number"),
		},
	}
	bindings := bind.BindFunction(fn, bind.Options{})

	got, ok := FromBindings(fn, bindings, resolvePrimitive, noDecl)
	if !ok {
		t.Fatalf("FromBindings ok = false, want true")
	}
	fnType, ok := got.(*typ.Function)
	if !ok {
		t.Fatalf("FromBindings type = %T, want *typ.Function", got)
	}
	if len(fnType.Params) != 1 || !fnType.Params[0].Type.Equals(typ.String) {
		t.Fatalf("Params = %#v, want [string]", fnType.Params)
	}
	if fnType.Variadic == nil || !fnType.Variadic.Equals(typ.Number) {
		t.Fatalf("Variadic = %v, want number", fnType.Variadic)
	}
}

// TestFromBindings_UntypedRegularParamStillCollapses documents the existing,
// unchanged policy for genuinely untyped fixed parameters (no annotation and
// not `...`): the whole signature still collapses to Variadic(Any), since
// there is no way to assign a fixed arity to an unannotated parameter list.
func TestFromBindings_UntypedRegularParamStillCollapses(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names: []string{"a", "b"},
			Types: []ast.TypeExpr{primitiveType("string"), nil},
		},
	}
	bindings := bind.BindFunction(fn, bind.Options{})

	got, ok := FromBindings(fn, bindings, resolvePrimitive, noDecl)
	if !ok {
		t.Fatalf("FromBindings ok = false, want true")
	}
	fnType, ok := got.(*typ.Function)
	if !ok {
		t.Fatalf("FromBindings type = %T, want *typ.Function", got)
	}
	if len(fnType.Params) != 0 {
		t.Fatalf("Params = %#v, want none (collapsed)", fnType.Params)
	}
	if fnType.Variadic == nil || !fnType.Variadic.Equals(typ.Any) {
		t.Fatalf("Variadic = %v, want Any", fnType.Variadic)
	}
}
