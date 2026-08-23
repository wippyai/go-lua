package bind

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/target/typeindex"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestTypeTraversalVisitsNestedRecordAndFunctionTypes(t *testing.T) {
	statements, err := parse.ParseString(`
type Numeric = number
type Flag = boolean
local value: { item: Numeric, nested: { flag: Flag } }
local fn: (Numeric, string) -> Flag
`, "type_traversal.lua")
	if err != nil {
		t.Fatal(err)
	}
	result := BindChunk(statements, typeindex.Table{})
	record := statements[2].(*ast.LocalAssignStmt).Types[0].(*ast.RecordTypeExpr)
	item, ok := result.PrimitiveTypeRef(record.Fields[0].Type.(*ast.PrimitiveTypeExpr))
	if !ok || item.Name != "Numeric" {
		t.Fatalf("record item type = %#v/%v, want Numeric", item, ok)
	}
	nested := record.Fields[1].Type.(*ast.RecordTypeExpr)
	flag, ok := result.PrimitiveTypeRef(nested.Fields[0].Type.(*ast.PrimitiveTypeExpr))
	if !ok || flag.Name != "Flag" {
		t.Fatalf("nested flag type = %#v/%v, want Flag", flag, ok)
	}
	function := statements[3].(*ast.LocalAssignStmt).Types[0].(*ast.FunctionTypeExpr)
	params := result.FunctionTypeParams(function)
	if len(params) != 0 {
		t.Fatalf("non-generic function type params = %#v, want no type declarations", params)
	}
	if _, ok := result.PrimitiveTypeRef(function.Returns[0].(*ast.PrimitiveTypeExpr)); !ok {
		t.Fatal("function return type was not traversed")
	}
}
