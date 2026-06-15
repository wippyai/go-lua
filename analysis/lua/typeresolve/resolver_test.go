package typeresolve

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/type/refinement"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

type fakeBindings struct {
	refs       map[*ast.TypeRefExpr]bind.TypeDecl
	primitives map[*ast.PrimitiveTypeExpr]bind.TypeDecl
	params     map[*ast.TypeDefStmt][]bind.TypeDecl
}

func (b fakeBindings) TypeRef(ref *ast.TypeRefExpr) (bind.TypeDecl, bool) {
	decl, ok := b.refs[ref]
	return decl, ok
}

func (b fakeBindings) PrimitiveTypeRef(expr *ast.PrimitiveTypeExpr) (bind.TypeDecl, bool) {
	decl, ok := b.primitives[expr]
	return decl, ok
}

func (b fakeBindings) TypeDefParams(stmt *ast.TypeDefStmt) []bind.TypeDecl {
	return append([]bind.TypeDecl(nil), b.params[stmt]...)
}

func TestResolverDeclBuildsGenericAliasWithLexicalTypeParam(t *testing.T) {
	paramRef := &ast.TypeRefExpr{Path: []string{"T"}}
	boxDecl := &ast.TypeDefStmt{
		Name:       "Box",
		TypeParams: []ast.TypeParamExpr{{Name: "T"}},
		Type:       paramRef,
	}
	box := bind.TypeDecl{ID: 1, Kind: bind.TypeDeclAlias, Name: "Box", Type: boxDecl}
	param := bind.TypeDecl{ID: 2, Kind: bind.TypeDeclParam, Name: "T"}
	bindings := fakeBindings{
		refs:   map[*ast.TypeRefExpr]bind.TypeDecl{paramRef: param},
		params: map[*ast.TypeDefStmt][]bind.TypeDecl{boxDecl: {param}},
	}

	got, ok := New(bindings).Decl(box)
	if !ok {
		t.Fatal("Decl returned ok=false")
	}
	generic, ok := got.(*typ.Generic)
	if !ok {
		t.Fatalf("Decl = %T, want *typ.Generic", got)
	}
	if generic.Name != "Box" || len(generic.TypeParams) != 1 || generic.TypeParams[0].Name != "T" {
		t.Fatalf("generic = %#v, want Box<T>", generic)
	}
	bodyParam, ok := unwrap.Annotations(generic.Body).(*typ.TypeParam)
	if !ok || bodyParam.Name != "T" {
		t.Fatalf("generic body = %T/%#v, want type param T", generic.Body, generic.Body)
	}
}

func TestResolverDeclBuildsGenericAliasWithBinderBindings(t *testing.T) {
	stmts, err := parse.ParseString(`type Box<T> = {T}`, "test")
	if err != nil {
		t.Fatalf("ParseString error: %v", err)
	}
	if len(stmts) != 1 {
		t.Fatalf("got %d statements, want 1", len(stmts))
	}
	stmt, ok := stmts[0].(*ast.TypeDefStmt)
	if !ok {
		t.Fatalf("stmt = %T, want *ast.TypeDefStmt", stmts[0])
	}
	bindings := bind.BindChunk(stmts, bind.Options{})
	decl, ok := bindings.TypeDef(stmt)
	if !ok {
		t.Fatal("TypeDef binding missing")
	}

	got, ok := New(bindings).Decl(decl)
	if !ok {
		t.Fatal("Decl returned ok=false")
	}
	generic, ok := got.(*typ.Generic)
	if !ok {
		t.Fatalf("Decl = %T, want *typ.Generic", got)
	}
	if generic.Name != "Box" || len(generic.TypeParams) != 1 || generic.TypeParams[0].Name != "T" {
		t.Fatalf("generic = %#v, want Box<T>", generic)
	}
	array, ok := unwrap.Annotations(generic.Body).(*typ.Array)
	if !ok {
		t.Fatalf("generic body = %T/%#v, want array", generic.Body, generic.Body)
	}
	bodyParam, ok := unwrap.Annotations(array.Element).(*typ.TypeParam)
	if !ok || bodyParam != generic.TypeParams[0] {
		t.Fatalf("array element = %T/%#v, want generic type param", array.Element, array.Element)
	}
}

func TestResolverDeclResolvesPrimitiveAliasWithBinderBindings(t *testing.T) {
	stmts, err := parse.Parse(strings.NewReader(`
type Alias = string
type Wrap = Alias
`), "test")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(stmts) != 2 {
		t.Fatalf("got %d statements, want 2", len(stmts))
	}
	stmt, ok := stmts[1].(*ast.TypeDefStmt)
	if !ok {
		t.Fatalf("stmt = %T, want *ast.TypeDefStmt", stmts[1])
	}
	bindings := bind.BindChunk(stmts, bind.Options{})
	decl, ok := bindings.TypeDef(stmt)
	if !ok {
		t.Fatal("TypeDef binding missing")
	}

	got, ok := New(bindings).Decl(decl)
	if !ok {
		t.Fatal("Decl returned ok=false")
	}
	if got != typ.String {
		t.Fatalf("Decl = %T/%#v, want string", got, got)
	}
}

func TestResolverDeclBuildsClosedRecursiveAliasWithBinderBindings(t *testing.T) {
	stmts, err := parse.Parse(strings.NewReader(`
type Node = {
	id: string,
	children: {Node},
}
`), "test")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(stmts) != 1 {
		t.Fatalf("got %d statements, want 1", len(stmts))
	}
	stmt, ok := stmts[0].(*ast.TypeDefStmt)
	if !ok {
		t.Fatalf("stmt = %T, want *ast.TypeDefStmt", stmts[0])
	}
	bindings := bind.BindChunk(stmts, bind.Options{})
	decl, ok := bindings.TypeDef(stmt)
	if !ok {
		t.Fatal("TypeDef binding missing")
	}

	got, ok := New(bindings).Decl(decl)
	if !ok {
		t.Fatal("Decl returned ok=false")
	}
	if _, ok := unwrap.Annotations(got).(*typ.Recursive); !ok {
		t.Fatalf("Decl = %T/%#v, want recursive alias", got, got)
	}
	if refinement.ContainsFreeTypeParam(got) {
		t.Fatalf("recursive alias contains free/open references: %v", got)
	}
}

func TestBindingInExprFindsPrimitiveAliasButSkipsBuiltins(t *testing.T) {
	aliasPrimitive := &ast.PrimitiveTypeExpr{Name: "Alias"}
	builtinPrimitive := &ast.PrimitiveTypeExpr{Name: "number"}
	aliasDecl := bind.TypeDecl{ID: 10, Kind: bind.TypeDeclAlias, Name: "Alias"}
	builtinDecl := bind.TypeDecl{ID: 11, Kind: bind.TypeDeclAlias, Name: "number"}
	bindings := fakeBindings{
		primitives: map[*ast.PrimitiveTypeExpr]bind.TypeDecl{
			aliasPrimitive:   aliasDecl,
			builtinPrimitive: builtinDecl,
		},
	}

	got, ok := BindingInExpr(bindings, aliasPrimitive, "Alias")
	if !ok || got.ID != aliasDecl.ID {
		t.Fatalf("BindingInExpr(alias) = %#v/%v, want alias decl", got, ok)
	}
	if got, ok := BindingInExpr(bindings, builtinPrimitive, "number"); ok {
		t.Fatalf("BindingInExpr(number) = %#v/true, want built-in primitive skipped", got)
	}
}

func TestWalkTypeNameExprVisitsRefsAndPrimitiveAliases(t *testing.T) {
	ref := &ast.TypeRefExpr{Path: []string{"T"}}
	prim := &ast.PrimitiveTypeExpr{Name: "Alias"}
	expr := &ast.FunctionTypeExpr{
		Params:  []ast.FunctionParamExpr{{Type: ref}},
		Returns: []ast.TypeExpr{prim},
	}
	var refs, primitives int

	WalkTypeNameExpr(expr, func(got *ast.TypeRefExpr) bool {
		if got == ref {
			refs++
		}
		return true
	}, func(got *ast.PrimitiveTypeExpr) bool {
		if got == prim {
			primitives++
		}
		return true
	})

	if refs != 1 || primitives != 1 {
		t.Fatalf("walk visits refs=%d primitives=%d, want 1/1", refs, primitives)
	}
}
