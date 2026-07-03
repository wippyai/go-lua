package typeresolve

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/type/refinement"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
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

func TestResolverTypeInstantiatesGenericAliasWithFunctionTypeParamArg(t *testing.T) {
	stmts, err := parse.ParseString(`
type Collection<T> = { value: T }
local M = {}
function M.new<T>(): Collection<T>
    return nil :: Collection<T>
end
`, "test")
	if err != nil {
		t.Fatalf("ParseString error: %v", err)
	}
	funcDef, ok := stmts[2].(*ast.FuncDefStmt)
	if !ok {
		t.Fatalf("stmt = %T, want FuncDefStmt", stmts[2])
	}
	bindings := bind.BindChunk(stmts, bind.Options{})
	params := bindings.FunctionTypeParams(funcDef.Func)
	if len(params) != 1 {
		t.Fatalf("FunctionTypeParams = %#v, want T", params)
	}
	resolver := New(bindings)
	paramType, ok := resolver.Decl(params[0])
	if !ok {
		t.Fatalf("function type param did not resolve")
	}
	got, ok := resolver.Type(funcDef.Func.ReturnTypes[0])
	if !ok {
		t.Fatal("Type(Collection<T>) returned ok=false")
	}
	inst, ok := unwrap.Annotations(got).(*typ.Instantiated)
	if !ok {
		t.Fatalf("Type(Collection<T>) = %T/%v, want instantiated generic", got, got)
	}
	if len(inst.TypeArgs) != 1 || inst.TypeArgs[0] != paramType {
		t.Fatalf("instantiated args = %#v, want function T %v", inst.TypeArgs, paramType)
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

func TestResolverDeclBuildsInterfaceMethodSetWithBinderBindings(t *testing.T) {
	stmts, err := parse.ParseString(`
interface Reader
	function read(self: self): string
end
`, "test")
	if err != nil {
		t.Fatalf("ParseString error: %v", err)
	}
	ifaceStmt, ok := stmts[0].(*ast.InterfaceDefStmt)
	if !ok {
		t.Fatalf("stmt = %T, want *ast.InterfaceDefStmt", stmts[0])
	}
	bindings := bind.BindChunk(stmts, bind.Options{})
	decl, ok := bindings.InterfaceDef(ifaceStmt)
	if !ok {
		t.Fatal("InterfaceDef binding missing")
	}

	got, ok := New(bindings).Decl(decl)
	if !ok {
		t.Fatal("Decl returned ok=false")
	}
	iface, ok := got.(*typ.Interface)
	if !ok {
		t.Fatalf("Decl = %T/%v, want *typ.Interface", got, got)
	}
	if iface.Name != "Reader" || len(iface.Methods) != 1 || iface.Methods[0].Name != "read" {
		t.Fatalf("interface = %#v, want Reader with read method", iface)
	}
	method := iface.Methods[0].Type
	if len(method.Params) != 1 || method.Params[0].Type != typ.Self || len(method.Returns) != 1 || method.Returns[0] != typ.String {
		t.Fatalf("read method = %#v, want (self) -> string", method)
	}
	record := typetable.NewRecord().
		Field("read", typ.Func().Param("self", typ.Any).Returns(typ.String).Build()).
		Build()
	if !subtype.IsSubtype(record, iface) {
		t.Fatalf("record with read(self)->string should satisfy %v", iface)
	}
}

func TestResolverDeclBuildsInterfaceWithInheritedMethodSet(t *testing.T) {
	stmts, err := parse.ParseString(`
interface Reader
	function read(): string
end
interface Closer
	function close(): boolean
end
interface ReadCloser: Reader, Closer
	function reset(): boolean
end
`, "test")
	if err != nil {
		t.Fatalf("ParseString error: %v", err)
	}
	child, ok := stmts[2].(*ast.InterfaceDefStmt)
	if !ok {
		t.Fatalf("stmt = %T, want child interface", stmts[2])
	}
	bindings := bind.BindChunk(stmts, bind.Options{})
	decl, ok := bindings.InterfaceDef(child)
	if !ok {
		t.Fatal("InterfaceDef binding missing")
	}

	got, ok := New(bindings).Decl(decl)
	if !ok {
		t.Fatal("Decl returned ok=false")
	}
	iface, ok := got.(*typ.Interface)
	if !ok {
		t.Fatalf("Decl = %T/%v, want *typ.Interface", got, got)
	}
	wantMethods := map[string]typ.Type{
		"read":  typ.Func().Returns(typ.String).Build(),
		"close": typ.Func().Returns(typ.Boolean).Build(),
		"reset": typ.Func().Returns(typ.Boolean).Build(),
	}
	if len(iface.Methods) != len(wantMethods) {
		t.Fatalf("methods = %#v, want inherited read/close plus reset", iface.Methods)
	}
	for _, method := range iface.Methods {
		want, ok := wantMethods[method.Name]
		if !ok {
			t.Fatalf("unexpected method %q in %#v", method.Name, iface.Methods)
		}
		if !typ.TypeEquals(method.Type, want) {
			t.Fatalf("%s type = %v, want %v", method.Name, method.Type, want)
		}
		delete(wantMethods, method.Name)
	}
	if len(wantMethods) != 0 {
		t.Fatalf("missing inherited methods: %#v", wantMethods)
	}
}

func TestResolverDeclRejectsConflictingInheritedInterfaceMethods(t *testing.T) {
	stmts, err := parse.ParseString(`
interface StringReader
	function read(): string
end
interface NumberReader
	function read(): number
end
interface Broken: StringReader, NumberReader
end
`, "test")
	if err != nil {
		t.Fatalf("ParseString error: %v", err)
	}
	bindings := bind.BindChunk(stmts, bind.Options{})
	decl, ok := bindings.InterfaceDef(stmts[2].(*ast.InterfaceDefStmt))
	if !ok {
		t.Fatal("InterfaceDef binding missing")
	}
	if got, ok := New(bindings).Decl(decl); ok {
		t.Fatalf("Decl = %T/%v, true; want conflict to fail closed", got, got)
	}
}

func TestResolverDeclRejectsInterfaceFields(t *testing.T) {
	stmt := &ast.InterfaceDefStmt{
		Name: "HasID",
		Fields: []ast.RecordFieldExpr{{
			Name: "id",
			Type: &ast.PrimitiveTypeExpr{Name: "string"},
		}},
	}
	decl := bind.TypeDecl{ID: 100, Kind: bind.TypeDeclInterface, Name: "HasID", Interface: stmt}
	if got, ok := New(fakeBindings{}).Decl(decl); ok {
		t.Fatalf("Decl = %T/%v, true; want interface fields to fail closed", got, got)
	}
}

func TestResolverDeclBuildsRecursiveInterfaceMethodSet(t *testing.T) {
	stmts, err := parse.ParseString(`
interface Node
	function next(): Node?
end
`, "test")
	if err != nil {
		t.Fatalf("ParseString error: %v", err)
	}
	bindings := bind.BindChunk(stmts, bind.Options{})
	decl, ok := bindings.InterfaceDef(stmts[0].(*ast.InterfaceDefStmt))
	if !ok {
		t.Fatal("InterfaceDef binding missing")
	}
	got, ok := New(bindings).Decl(decl)
	if !ok {
		t.Fatal("Decl returned ok=false")
	}
	rec, ok := got.(*typ.Recursive)
	if !ok {
		t.Fatalf("Decl = %T/%v, want recursive interface", got, got)
	}
	body, ok := rec.Body.(*typ.Interface)
	if !ok || body.Name != "Node" || len(body.Methods) != 1 {
		t.Fatalf("recursive body = %T/%v, want Node interface body", rec.Body, rec.Body)
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
