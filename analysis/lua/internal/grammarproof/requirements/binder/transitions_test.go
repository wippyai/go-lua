package binder

import (
	"testing"

	bind "github.com/wippyai/go-lua/analysis/lua/bind"
	lualower "github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program/target/typeindex"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestPublicBinderTransitions(t *testing.T) {
	statements, err := parse.ParseString(`
type Shape = number
type Box<T> = T
type Remote = stream.Shape
type Missing = Unknown
M.Shape = Shape
local value = {}
local function shadow(Shape)
	return Shape()
end
return number(value), Shape(value)
`, "binder_requirements.lua")
	if err != nil {
		t.Fatal(err)
	}
	result := bind.BindChunk(statements, typeindex.Table{})

	shape := statements[0].(*ast.TypeDefStmt)
	box := statements[1].(*ast.TypeDefStmt)
	remote := statements[2].(*ast.TypeDefStmt)
	missing := statements[3].(*ast.TypeDefStmt)
	publication := statements[4].(*ast.AssignStmt)
	shadow := statements[6].(*ast.LocalAssignStmt).Exprs[0].(*ast.FunctionExpr)
	returns := statements[7].(*ast.ReturnStmt).Exprs

	shapeDecl, ok := result.TypeDef(shape)
	if !ok || shapeDecl.Kind != bind.TypeDeclAlias || shapeDecl.Name != "Shape" {
		t.Fatalf("Shape declaration = %#v/%v", shapeDecl, ok)
	}
	parameter := box.Type.(*ast.PrimitiveTypeExpr)
	parameterDecl, ok := result.PrimitiveTypeRef(parameter)
	if !ok || parameterDecl.Kind != bind.TypeDeclParam || parameterDecl.Name != "T" {
		t.Fatalf("Box parameter reference = %#v/%v", parameterDecl, ok)
	}
	if params := result.TypeDefParams(box); len(params) != 1 || params[0].ID != parameterDecl.ID {
		t.Fatalf("Box parameters = %#v", params)
	}
	unknown := missing.Type.(*ast.PrimitiveTypeExpr)
	if declaration, resolved := result.PrimitiveTypeRef(unknown); resolved || declaration.ID != 0 {
		t.Fatalf("Unknown type reference = %#v/%v, want typed unresolved disposition", declaration, resolved)
	}
	qualified := remote.Type.(*ast.TypeRefExpr)
	if root, found := result.QualifiedTypeRootSymbol(qualified); !found || root == 0 {
		t.Fatal("qualified type root has no binder-owned value identity")
	}
	publications := result.StaticTypePublications(publication)
	if len(publications) != 1 || publications[0].Index != 0 || len(publications[0].Source) != 1 || publications[0].Source[0] != "Shape" {
		t.Fatalf("static publication = %#v", publications)
	}
	primitive := callBase(t, returns[0].(*ast.FuncCallExpr))
	primitiveEvidence, ok := result.RuntimeTypeValue(primitive)
	if !ok || primitiveEvidence.Kind != bind.RuntimeTypeValuePrimitive || primitiveEvidence.Name != "number" {
		t.Fatalf("primitive runtime type = %#v/%v", primitiveEvidence, ok)
	}
	declaration := callBase(t, returns[1].(*ast.FuncCallExpr))
	declarationEvidence, ok := result.RuntimeTypeValue(declaration)
	if !ok || declarationEvidence.Kind != bind.RuntimeTypeValueDeclaration || declarationEvidence.Decl.ID != shapeDecl.ID {
		t.Fatalf("declaration runtime type = %#v/%v", declarationEvidence, ok)
	}
	shadowCall := shadow.Stmts[0].(*ast.ReturnStmt).Exprs[0].(*ast.FuncCallExpr)
	if shadowEvidence, found := result.RuntimeTypeValue(callBase(t, shadowCall)); found || shadowEvidence.Kind != 0 {
		t.Fatalf("shadowed runtime type = %#v/%v, want rejected", shadowEvidence, found)
	}

	if len(Required()) != 9 {
		t.Fatalf("binder requirement count = %d, want 9", len(Required()))
	}
}

func TestDirectRequirePreservesOneGlobalIdentityIntoProgram(t *testing.T) {
	const source = "local module = require(\"pkg.core\")\n"
	statements, err := parse.ParseString(source, "require_identity.lua")
	if err != nil {
		t.Fatal(err)
	}
	call := statements[0].(*ast.LocalAssignStmt).Exprs[0].(*ast.FuncCallExpr)
	ident := call.Func.(*ast.IdentExpr)
	binding := bind.BindChunk(statements, typeindex.Table{})
	if !binding.IsImplicitGlobalUse(ident) {
		t.Fatal("direct require has no binder implicit-global evidence")
	}
	identity, ok := binding.GlobalIdentity(ident)
	if !ok || !identity.Matches("require") {
		t.Fatalf("direct require global identity = %#v/%v", identity, ok)
	}
	sealed, err := lualower.Lower(lualower.Source{Name: "require_identity.lua", Text: []byte(source)})
	if err != nil {
		t.Fatal(err)
	}
	imports := sealed.Flow().Authored().Imports()
	if imports.Count() != 1 {
		t.Fatalf("direct require ImportCount = %d, want 1", imports.Count())
	}
	importRow, ok := imports.ImportAt(0)
	if !ok {
		t.Fatal("direct require Import missing")
	}
	row, ok := imports.Get(importRow.Term)
	if !ok || row.Call == 0 {
		t.Fatalf("direct require Import = %#v/%v", row, ok)
	}

	const shadowed = "local require = function(value) return value end\nlocal module = require(\"pkg.core\")\n"
	shadowStatements, err := parse.ParseString(shadowed, "require_shadow.lua")
	if err != nil {
		t.Fatal(err)
	}
	shadowCall := shadowStatements[1].(*ast.LocalAssignStmt).Exprs[0].(*ast.FuncCallExpr)
	shadowIdent := shadowCall.Func.(*ast.IdentExpr)
	shadowBinding := bind.BindChunk(shadowStatements, typeindex.Table{})
	if shadowBinding.IsImplicitGlobalUse(shadowIdent) {
		t.Fatal("shadowed require fabricated implicit-global evidence")
	}
	if identity, ok := shadowBinding.GlobalIdentity(shadowIdent); ok || identity.Matches("require") {
		t.Fatalf("shadowed require global identity = %#v/%v", identity, ok)
	}
	shadowProgram, err := lualower.Lower(lualower.Source{Name: "require_shadow.lua", Text: []byte(shadowed)})
	if err != nil {
		t.Fatal(err)
	}
	shadowImports := shadowProgram.Flow().Authored().Imports()
	if shadowImports.Count() != 0 {
		t.Fatalf("shadowed require ImportCount = %d, want 0", shadowImports.Count())
	}
}

func callBase(t testing.TB, call *ast.FuncCallExpr) *ast.IdentExpr {
	t.Helper()
	base, ok := call.Func.(*ast.IdentExpr)
	if !ok {
		t.Fatalf("call base = %T, want identifier", call.Func)
	}
	return base
}
