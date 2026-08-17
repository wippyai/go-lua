package lower_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func applicationFunctionSourceCases() []sourceCase {
	return []sourceCase{
		{"application.case.function.inferred-returns", "FunctionExpr", "local f =\nfunction(value)\n  return value\nend\nreturn f", 2},
		{"application.case.function.declared-empty-returns", "FunctionExpr", "local f =\nfunction(): ()\n  return\nend\nreturn f", 2},
		{"application.case.function.declared-returns", "FunctionExpr", "local f =\nfunction(value: number): number\n  return value\nend\nreturn f", 2},
		{"application.case.function.declared-multiple-returns", "FunctionExpr", "local f =\nfunction(value: number): (number, string)\n  return value, \"ok\"\nend\nreturn f", 2},
	}
}

func TestApplicationFunctionSourceCasesHaveExactProgramWitnesses(t *testing.T) {
	for _, sourceCase := range applicationFunctionSourceCases() {
		t.Run(string(sourceCase.ID), func(t *testing.T) {
			stmts, err := parse.ParseString(sourceCase.Source, "fixture.lua")
			if err != nil {
				t.Fatal(err)
			}
			anchor := applicationAnchor(t, stmts, sourceCase)
			if anchor.Form != sourceCase.Form || anchor.Line != sourceCase.Line || anchor.Span.StartLine == 0 || anchor.Span.File != "fixture.lua" {
				t.Fatalf("parsed application anchor = %#v for %s/%d", anchor, sourceCase.Form, sourceCase.Line)
			}
			binding := bind.BindChunk(stmts)
			p := parseBindLower(t, sourceCase.Source)
			switch node := anchor.Node.(type) {
			case *ast.FunctionExpr:
				term := applicationFunctionAt(t, p, node)
				applicationFunction(t, p, term, node, binding)
				applicationBoundFunction(t, binding, stmts, node)
			default:
				t.Fatalf("unhandled application anchor %T", anchor)
			}
		})
	}
}

func applicationFunction(t *testing.T, p *program.Program, term keyspace.Term, node *ast.FunctionExpr, binding *bind.Result) {
	t.Helper()
	flow := p.Flow()
	owner, body, vararg, ok := flow.Authored().Functions().Get(term)
	if !ok || owner == 0 || body == 0 {
		t.Fatalf("Function = owner %v body %v vararg %v ok %v", owner, body, vararg, ok)
	}
	slots := binding.ParamSlots(node)
	wantFormals := len(slots)
	if node.ParList != nil && node.ParList.HasVargs {
		wantFormals--
	}
	if got, ok := p.Source().Formals().Len(term); !ok || got != wantFormals || (node.ParList != nil && node.ParList.HasVargs) != (vararg != 0) {
		t.Fatalf("Function formals/vararg = %d/%v/%v, binder non-vararg slots %d parsed-vararg %v", got, ok, vararg, wantFormals, node.ParList != nil && node.ParList.HasVargs)
	}
	if known, ok := p.Static().Contracts().Functions().Get(term); !ok || known != node.ReturnsKnown {
		t.Fatalf("Function returns-known = %v/%v, want parsed %v", known, ok, node.ReturnsKnown)
	}
	if count, ok := p.Static().Contracts().Functions().ReturnCount(term); !ok || count != len(node.ReturnTypes) {
		t.Fatalf("Function return count = %d/%v, want parsed %d", count, ok, len(node.ReturnTypes))
	}
	if next, ok := flow.Ports().Finish(term); !ok || next == 0 {
		t.Fatalf("Function(%v) has no normal successor", term)
	}
}

func applicationBoundFunction(t *testing.T, result *bind.Result, stmts []ast.Stmt, node *ast.FunctionExpr) {
	t.Helper()
	origin, ok := result.FunctionOrigin(node)
	if !ok || origin.Func != node {
		t.Fatal("binder lost exact FunctionExpr origin")
	}
	if want := applicationFunctionOrigin(stmts, node); origin.Kind != want {
		t.Fatalf("binder function origin = %v, want parsed origin %v", origin.Kind, want)
	}
	if node.ParList == nil {
		return
	}
	slots := result.ParamSlots(node)
	implicitSelf := origin.Kind == bind.FunctionOriginMethod && (len(node.ParList.Names) == 0 || node.ParList.Names[0] != "self")
	want := len(node.ParList.Names)
	if implicitSelf {
		want++
	}
	if node.ParList.HasVargs {
		want++
	}
	if len(slots) != want {
		t.Fatalf("binder parameter slots = %d, want exact authored layout %d", len(slots), want)
	}
	offset := 0
	if implicitSelf {
		first := slots[0]
		if first.Name != "self" || !first.ImplicitSelf || first.Vararg || first.SourceIndex != -1 {
			t.Fatalf("binder implicit receiver = %#v", first)
		}
		offset = 1
	}
	for index, name := range node.ParList.Names {
		slot := slots[offset+index]
		if slot.Name != name || slot.ImplicitSelf || slot.Vararg || slot.SourceIndex != index || slot.Position != applicationNamePosition(node.ParList, index) || slot.Type != applicationNameType(node.ParList, index) {
			t.Fatalf("binder formal %d = %#v, want parsed name/type/position", index, slot)
		}
	}
	if node.ParList.HasVargs {
		slot := slots[len(slots)-1]
		if slot.Name != "..." || slot.ImplicitSelf || !slot.Vararg || slot.SourceIndex != len(node.ParList.Names) || slot.Position != node.ParList.VarargPosition || slot.Type != node.ParList.VarargType {
			t.Fatalf("binder vararg = %#v, want parsed vararg", slot)
		}
	}
}

func applicationNamePosition(list *ast.ParList, index int) ast.Position {
	if list == nil || index < 0 || index >= len(list.NamePositions) {
		return ast.Position{}
	}
	return list.NamePositions[index]
}

func applicationNameType(list *ast.ParList, index int) ast.TypeExpr {
	if list == nil || index < 0 || index >= len(list.Types) {
		return nil
	}
	return list.Types[index]
}

func applicationFunctionOrigin(stmts []ast.Stmt, function *ast.FunctionExpr) bind.FunctionOriginKind {
	for _, stmt := range stmts {
		switch node := stmt.(type) {
		case *ast.FuncDefStmt:
			if node.Func == function {
				if node.Name != nil && node.Name.Receiver != nil {
					return bind.FunctionOriginMethod
				}
				return bind.FunctionOriginDeclaration
			}
		case *ast.LocalAssignStmt:
			for _, expr := range node.Exprs {
				if expr == function {
					return bind.FunctionOriginLocalAssignment
				}
			}
		}
	}
	return bind.FunctionOriginLiteral
}

func applicationFunctionAt(t *testing.T, p *program.Program, node ast.PositionHolder) keyspace.Term {
	t.Helper()
	functions := p.Flow().Authored().Functions()
	return applicationTermAt(t, p, node, functions.Count, functions.At, "Function")
}
