package pathexpr

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

func ident(name string) *ast.IdentExpr {
	return &ast.IdentExpr{Value: name}
}

func dot(obj ast.Expr, name string) *ast.AttrGetExpr {
	return &ast.AttrGetExpr{
		Object:    obj,
		Key:       &ast.StringExpr{Value: name},
		KeySyntax: ast.AttrKeyDot,
	}
}

func stringIndex(obj ast.Expr, key string) *ast.AttrGetExpr {
	return &ast.AttrGetExpr{
		Object:    obj,
		Key:       &ast.StringExpr{Value: key},
		KeySyntax: ast.AttrKeyIndex,
	}
}

func intIndex(obj ast.Expr, index string) *ast.AttrGetExpr {
	return &ast.AttrGetExpr{
		Object:    obj,
		Key:       &ast.NumberExpr{Value: index},
		KeySyntax: ast.AttrKeyIndex,
	}
}

func dynamicIndex(obj ast.Expr, key ast.Expr) *ast.AttrGetExpr {
	return &ast.AttrGetExpr{
		Object:    obj,
		Key:       key,
		KeySyntax: ast.AttrKeyIndex,
	}
}

func primitiveType(name string) *ast.PrimitiveTypeExpr {
	return &ast.PrimitiveTypeExpr{Name: name}
}

func cast(expr ast.Expr, typ ast.TypeExpr) *ast.CastExpr {
	return &ast.CastExpr{Expr: expr, Type: typ, Syntax: ast.CastSyntaxColonColon}
}

func nonNil(expr ast.Expr) *ast.NonNilAssertExpr {
	return &ast.NonNilAssertExpr{Expr: expr}
}

func bindReturn(expr ast.Expr) *bind.Result {
	return bind.BindChunk([]ast.Stmt{&ast.ReturnStmt{Exprs: []ast.Expr{expr}}}, bind.Options{})
}

func mustResolvedRoot(t *testing.T, bindings *bind.Result, root *ast.IdentExpr) symbol.ID {
	t.Helper()
	id, ok := bindings.SymbolOf(root)
	if !ok || id == 0 {
		t.Fatalf("SymbolOf(%q) = %d/%v, want non-zero symbol", root.Value, id, ok)
	}
	return id
}

func assertResolved(t *testing.T, expr ast.Expr, bindings *bind.Result, want path.Path) {
	t.Helper()
	got, ok := Resolve(expr, bindings)
	if !ok {
		t.Fatalf("Resolve(%T) rejected static path", expr)
	}
	if got.Root != want.Root || got.Symbol != want.Symbol || !reflect.DeepEqual(got.Segments, want.Segments) {
		t.Fatalf("Resolve() = %#v, want %#v", got, want)
	}
}

func assertAliasResolved(t *testing.T, expr ast.Expr, bindings *bind.Result, want path.Path) {
	t.Helper()
	got, ok := ResolveAlias(expr, bindings)
	if !ok {
		t.Fatalf("ResolveAlias(%T) rejected aliased path", expr)
	}
	if got.Root != want.Root || got.Symbol != want.Symbol || !reflect.DeepEqual(got.Segments, want.Segments) {
		t.Fatalf("ResolveAlias() = %#v, want %#v", got, want)
	}
}

func assertRejected(t *testing.T, expr ast.Expr, bindings *bind.Result) {
	t.Helper()
	got, ok := Resolve(expr, bindings)
	if ok || !got.IsEmpty() {
		t.Fatalf("Resolve() = %#v/%v, want empty/false", got, ok)
	}
}

func assertAliasRejected(t *testing.T, expr ast.Expr, bindings *bind.Result) {
	t.Helper()
	got, ok := ResolveAlias(expr, bindings)
	if ok || !got.IsEmpty() {
		t.Fatalf("ResolveAlias() = %#v/%v, want empty/false", got, ok)
	}
}

func assertLengthOperandResolved(t *testing.T, expr ast.Expr, bindings *bind.Result, want path.Path) {
	t.Helper()
	got, ok := ResolveLengthOperand(expr, bindings)
	if !ok {
		t.Fatalf("ResolveLengthOperand(%T) rejected length path", expr)
	}
	if got.Root != want.Root || got.Symbol != want.Symbol || !reflect.DeepEqual(got.Segments, want.Segments) {
		t.Fatalf("ResolveLengthOperand() = %#v, want %#v", got, want)
	}
}

func assertLengthOperandRejected(t *testing.T, expr ast.Expr, bindings *bind.Result) {
	t.Helper()
	got, ok := ResolveLengthOperand(expr, bindings)
	if ok || !got.IsEmpty() {
		t.Fatalf("ResolveLengthOperand() = %#v/%v, want empty/false", got, ok)
	}
}

func assertMutationContainer(t *testing.T, expr ast.Expr, bindings *bind.Result, want path.Path) {
	t.Helper()
	got, ok := ResolveMutationContainer(expr, bindings)
	if !ok {
		t.Fatalf("ResolveMutationContainer(%T) rejected target", expr)
	}
	if got.Root != want.Root || got.Symbol != want.Symbol || !reflect.DeepEqual(got.Segments, want.Segments) {
		t.Fatalf("ResolveMutationContainer() = %#v, want %#v", got, want)
	}
}

func TestResolveIdent(t *testing.T) {
	root := ident("value")
	bindings := bindReturn(root)
	sym := mustResolvedRoot(t, bindings, root)

	assertResolved(t, root, bindings, path.NewPath(sym, "value"))
}

func TestResolveNestedDot(t *testing.T) {
	root := ident("obj")
	expr := dot(dot(root, "a"), "b")
	bindings := bindReturn(expr)
	sym := mustResolvedRoot(t, bindings, root)

	assertResolved(t, expr, bindings, path.NewPath(sym, "obj").Field("a").Field("b"))
}

func TestResolveStringIndex(t *testing.T) {
	root := ident("obj")
	expr := stringIndex(root, "key")
	bindings := bindReturn(expr)
	sym := mustResolvedRoot(t, bindings, root)

	assertResolved(t, expr, bindings, path.NewPath(sym, "obj").IndexStr("key"))
}

func TestResolveIntIndex(t *testing.T) {
	root := ident("obj")
	expr := intIndex(root, "12")
	bindings := bindReturn(expr)
	sym := mustResolvedRoot(t, bindings, root)

	assertResolved(t, expr, bindings, path.NewPath(sym, "obj").IndexInt(12))
}

func TestResolveMixedPath(t *testing.T) {
	root := ident("obj")
	expr := dot(intIndex(stringIndex(dot(root, "a"), "b"), "3"), "c")
	bindings := bindReturn(expr)
	sym := mustResolvedRoot(t, bindings, root)

	want := path.NewPath(sym, "obj").Field("a").IndexStr("b").IndexInt(3).Field("c")
	assertResolved(t, expr, bindings, want)
}

func TestResolveAliasNonNilAssertPreservesOperandPath(t *testing.T) {
	root := ident("obj")
	expr := nonNil(dot(root, "child"))
	bindings := bindReturn(expr)
	sym := mustResolvedRoot(t, bindings, root)

	assertRejected(t, expr, bindings)
	assertAliasResolved(t, expr, bindings, path.NewPath(sym, "obj").Field("child"))
}

func TestResolveAliasNonAnyCastPreservesOperandPath(t *testing.T) {
	root := ident("obj")
	expr := cast(dot(root, "child"), primitiveType("number"))
	bindings := bindReturn(expr)
	sym := mustResolvedRoot(t, bindings, root)

	assertRejected(t, expr, bindings)
	assertAliasResolved(t, expr, bindings, path.NewPath(sym, "obj").Field("child"))
}

func TestResolveAliasMapSupertypeCastWithAnyValuePreservesOperandPath(t *testing.T) {
	root := ident("suites")
	expr := cast(root, &ast.MapTypeExpr{
		Key:   primitiveType("string"),
		Value: primitiveType("any"),
	})
	bindings := bindReturn(expr)
	sym := mustResolvedRoot(t, bindings, root)

	assertRejected(t, expr, bindings)
	assertAliasResolved(t, expr, bindings, path.NewPath(sym, "suites"))
}

func TestResolveAliasPrimitiveAnyCastRejectsProofPath(t *testing.T) {
	root := ident("obj")
	expr := cast(dot(root, "child"), primitiveType("any"))
	bindings := bindReturn(expr)

	assertRejected(t, expr, bindings)
	assertAliasRejected(t, expr, bindings)
}

func TestResolveAliasPrimitiveUnknownCastRejectsProofPath(t *testing.T) {
	root := ident("obj")
	expr := cast(dot(root, "child"), primitiveType("unknown"))
	bindings := bindReturn(expr)

	assertRejected(t, expr, bindings)
	assertAliasRejected(t, expr, bindings)
}

func TestResolveLengthOperandUsesSyntaxPath(t *testing.T) {
	root := ident("items")
	expr := &ast.UnaryLenOpExpr{Expr: dot(root, "children")}
	bindings := bindReturn(expr)
	sym := mustResolvedRoot(t, bindings, root)

	assertLengthOperandResolved(t, expr, bindings, path.NewPath(sym, "items").Field("children"))
}

func TestResolveLengthOperandDoesNotCrossProofBoundary(t *testing.T) {
	root := ident("items")
	expr := &ast.UnaryLenOpExpr{Expr: cast(root, primitiveType("table"))}
	bindings := bindReturn(expr)

	assertLengthOperandRejected(t, expr, bindings)
}

func TestResolveLengthOperandRejectsNonLengthExpression(t *testing.T) {
	root := ident("items")
	bindings := bindReturn(root)

	assertLengthOperandRejected(t, root, bindings)
}

func TestResolveUnresolvedIdent(t *testing.T) {
	bound := ident("x")
	unresolved := ident("x")
	bindings := bindReturn(bound)

	assertRejected(t, unresolved, bindings)
}

func TestResolveRejectsDynamicIndex(t *testing.T) {
	root := ident("obj")
	key := ident("key")
	expr := dynamicIndex(root, key)
	bindings := bindReturn(expr)
	if _, ok := bindings.SymbolOf(key); !ok {
		t.Fatalf("test setup expected dynamic key ident to be bound")
	}

	assertRejected(t, expr, bindings)
}

func TestResolveMutationContainerUsesNearestStaticAncestor(t *testing.T) {
	root := ident("obj")
	key := ident("key")
	directDynamic := dynamicIndex(dot(root, "items"), key)
	nestedDynamic := dot(dynamicIndex(dot(root, "items"), key), "value")
	deepNestedDynamic := dot(dynamicIndex(nestedDynamic, ident("child_key")), "name")
	bindings := bindReturn(deepNestedDynamic)
	sym := mustResolvedRoot(t, bindings, root)

	assertMutationContainer(t, directDynamic, bindings, path.NewPath(sym, "obj").Field("items"))
	assertMutationContainer(t, nestedDynamic, bindings, path.NewPath(sym, "obj").Field("items"))
	assertMutationContainer(t, deepNestedDynamic, bindings, path.NewPath(sym, "obj").Field("items"))
}

func TestResolveDynamicMutationTargetCarriesTrailingStaticSuffix(t *testing.T) {
	root := ident("slots")
	key := ident("key")
	targetExpr := dot(dynamicIndex(root, key), "value")
	bindings := bindReturn(targetExpr)
	sym := mustResolvedRoot(t, bindings, root)

	target, ok := ResolveDynamicMutationTarget(targetExpr, bindings)
	if !ok {
		t.Fatal("ResolveDynamicMutationTarget rejected nested dynamic target")
	}
	if !target.Table.Equal(path.NewPath(sym, "slots")) {
		t.Fatalf("target table = %#v, want slots root", target.Table)
	}
	if target.Key != key {
		t.Fatalf("target key = %#v, want original key expr", target.Key)
	}
	wantSuffix := []segment.Segment{{Kind: segment.SegmentField, Name: "value"}}
	if !reflect.DeepEqual(target.Suffix, wantSuffix) {
		t.Fatalf("target suffix = %#v, want %#v", target.Suffix, wantSuffix)
	}
}

func TestResolveNilBinding(t *testing.T) {
	assertRejected(t, ident("x"), nil)
	assertRejected(t, dot(ident("x"), "field"), nil)
}
