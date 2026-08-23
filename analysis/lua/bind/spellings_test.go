package bind_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/program/target/typeindex"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestCallSpellingIsBoundAtTheCallOccurrence(t *testing.T) {
	statements, err := parse.ParseString(`direct(); object.field(); object[key](); object:method()`, "call-spellings.lua")
	if err != nil {
		t.Fatal(err)
	}
	binding := bind.BindChunk(statements, typeindex.Table{})
	want := []string{"direct", "object.field", "", "method"}
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

func TestMethodParamSlotOwnsImplicitSelfSelectorPosition(t *testing.T) {
	statements, err := parse.ParseString(`function object:run(value) return value end`, "method-slots.lua")
	if err != nil {
		t.Fatal(err)
	}
	statement, ok := statements[0].(*ast.FuncDefStmt)
	if !ok || statement.Name == nil || statement.Func == nil {
		t.Fatal("method declaration was not parsed")
	}
	binding := bind.BindChunk(statements, typeindex.Table{})
	slots := binding.ParamSlots(statement.Func)
	if len(slots) != 2 || !slots[0].ImplicitSelf || slots[0].Name != "self" {
		t.Fatalf("method ParamSlots = %#v, want implicit self followed by value", slots)
	}
	if slots[0].Position != statement.Name.MethodPosition {
		t.Fatalf("implicit self Position = %#v, want selector %#v", slots[0].Position, statement.Name.MethodPosition)
	}
	if name := binding.Name(slots[0].Symbol); name != "self" {
		t.Fatalf("implicit self spelling = %q, want self", name)
	}
}
