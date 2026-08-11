package parse

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
)

func TestParseTableFieldsRetainExactSourceSpans(t *testing.T) {
	stmts, err := ParseString(`local t = {
  name = 42,
  [key] = value,
  f(),
  readonly = 0
}`, "table_fields.lua")
	if err != nil {
		t.Fatal(err)
	}
	table := stmts[0].(*ast.LocalAssignStmt).Exprs[0].(*ast.TableExpr)
	if len(table.Fields) != 4 {
		t.Fatalf("fields = %d, want 4", len(table.Fields))
	}

	for index, want := range [][4]int{
		{2, 3, 2, 11}, // name = 42
		{3, 3, 3, 15}, // [key] = value
		{4, 3, 4, 5},  // f()
		{5, 3, 5, 14}, // readonly = 0
	} {
		requireParsedSpan(t, table.Fields[index], want[0], want[1], want[2], want[3])
	}

	nameKey, ok := table.Fields[0].Key.(*ast.StringExpr)
	if !ok || nameKey.Value != "name" {
		t.Fatalf("name key = %#v, want string name", table.Fields[0].Key)
	}
	requireParsedSpan(t, nameKey, 2, 3, 2, 6)
	contextualKey, ok := table.Fields[3].Key.(*ast.StringExpr)
	if !ok || contextualKey.Value != "readonly" {
		t.Fatalf("contextual key = %#v, want string readonly", table.Fields[3].Key)
	}
	requireParsedSpan(t, contextualKey, 5, 3, 5, 10)
}
