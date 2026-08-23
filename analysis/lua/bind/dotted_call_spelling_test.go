package bind_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/program/target/typeindex"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestDottedCallSpellingRecordsTheFullChain(t *testing.T) {
	source := `a.b.c(); a.b.c.d(); obj:m(); f(); f[k](); a["b"].c(); f().b()`
	statements, err := parse.ParseString(source, "dotted-call-spellings.lua")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a.b.c", "a.b.c.d", "m", "f", "", "", ""}
	binding := bind.BindChunk(statements, typeindex.Table{})
	for index, statement := range statements {
		callStmt, ok := statement.(*ast.FuncCallStmt)
		if !ok || callStmt.Expr == nil {
			t.Fatalf("statement %d is not a Call", index)
		}
		call, ok := callStmt.Expr.(*ast.FuncCallExpr)
		if !ok {
			t.Fatalf("statement %d expression is not a FuncCall", index)
		}
		name, named := binding.CallSpelling(call)
		if want[index] == "" {
			if named || name != "" {
				t.Fatalf("Call %d spelling = %q/%v, want absent", index, name, named)
			}
			continue
		}
		if !named || name != want[index] {
			t.Fatalf("Call %d spelling = %q/%v, want %q", index, name, named, want[index])
		}
	}
}
