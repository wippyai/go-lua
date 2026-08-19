package acceptance_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/program"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func applicationCallSourceCases() []sourceCase {
	return []sourceCase{
		{"application.case.call.plain.scalar", "FuncCallExpr", "local function f()\n  return 1, 2\nend\nreturn (f())", 4},
		{"application.case.call.plain.open", "FuncCallExpr", "local function f()\n  return 1, 2\nend\nreturn f()", 4},
		{"application.case.call.method.scalar", "FuncCallExpr", "local object = {\n  f = function(self)\n    return 1, 2\n  end,\n}\nreturn (object:f())", 6},
		{"application.case.call.method.open", "FuncCallExpr", "local object = {\n  f = function(self)\n    return 1, 2\n  end,\n}\nreturn object:f()", 6},
	}
}

func TestApplicationCallSourceCasesHaveExactProgramWitnesses(t *testing.T) {
	for _, sourceCase := range applicationCallSourceCases() {
		t.Run(string(sourceCase.ID), func(t *testing.T) {
			stmts, err := parse.ParseString(sourceCase.Source, "fixture.lua")
			if err != nil {
				t.Fatal(err)
			}
			anchor := applicationAnchor(t, stmts, sourceCase)
			if anchor.Form != sourceCase.Form || anchor.Line != sourceCase.Line || anchor.Span.StartLine == 0 || anchor.Span.File != "fixture.lua" {
				t.Fatalf("parsed application anchor = %#v for %s/%d", anchor, sourceCase.Form, sourceCase.Line)
			}
			bind.BindChunk(stmts)
			p := parseBindLower(t, sourceCase.Source)
			switch node := anchor.Node.(type) {
			case *ast.FuncCallExpr:
				applicationCall(t, p, applicationCallAt(t, p, node), node, stmts)
			default:
				t.Fatalf("unhandled application anchor %T", anchor)
			}
		})
	}
}

func applicationCall(t *testing.T, p *program.Program, term keyspace.Term, node *ast.FuncCallExpr, stmts []ast.Stmt) {
	t.Helper()
	flow := p.Flow()
	owner, callee, receiver, actuals, ok := flow.Authored().Calls().Get(term)
	if !ok || owner == 0 || callee == 0 || actuals == 0 {
		t.Fatalf("Call = owner %v callee %v receiver %v actuals %v ok %v", owner, callee, receiver, actuals, ok)
	}
	direct, _ := flow.DirectFunctions().Call(term)
	if (node.Receiver != nil) != (receiver != 0) {
		t.Fatalf("Call receiver = %v for parsed receiver %T", receiver, node.Receiver)
	}
	if fixed, ok := flow.Authored().Values().Len(actuals); !ok || fixed != len(node.Args) {
		t.Fatalf("Call actual fixed count = %d/%v, want parsed %d", fixed, ok, len(node.Args))
	}
	if node.Receiver != nil {
		// A Lua table member is mutable.  Even this literal-table method call
		// has no flow proof that the member remains that closure at the call.
		if direct != 0 {
			t.Fatalf("mutable method Call direct candidate = %v, want absent", direct)
		}
	} else {
		function := applicationOnlyFunctionExpr(t, stmts)
		wantDirect := applicationFunctionAt(t, p, function)
		if direct != wantDirect {
			t.Fatalf("plain local Call direct candidate = %v, want source Function %v", direct, wantDirect)
		}
	}
	if types, ok := p.Static().Contracts().Calls().TypeArgumentCount(term); !ok || types != len(node.TypeArgs) {
		t.Fatalf("Call type-argument count = %d/%v, want parsed %d", types, ok, len(node.TypeArgs))
	}
	if entry, ok := flow.Ports().Entry(callee); !ok || entry == 0 {
		t.Fatalf("Call(%v) has no callee entry", term)
	}
	if next, ok := flow.Ports().Finish(term); !ok || next == 0 {
		t.Fatalf("Call(%v) has no normal successor", term)
	}
	// The atomic call sources return their anchor directly.  Parentheses are
	// already reflected by the parser's AdjustRet bit, so this proves the
	// Program's fixed-versus-final-open result relation from syntax rather than
	// from a prose label.
	returned := returnOwnedBy(t, p, owner)
	_, values, ok := flow.Authored().Control().Returns().Get(returned)
	if !ok {
		t.Fatalf("Call(%v) owner has no Return Values", term)
	}
	fixed, fixedOK := flow.Authored().Values().Len(values)
	tail := valuesTail(t, p, values)
	if node.AdjustRet {
		if !fixedOK || fixed != 1 || valueAt(t, p, values, 0) != term || tail != 0 {
			t.Fatalf("scalar Call result Values = fixed %d/%v value %v tail %v", fixed, fixedOK, valueAt(t, p, values, 0), tail)
		}
	} else if !fixedOK || fixed != 0 || tail != term {
		t.Fatalf("open Call result Values = fixed %d/%v tail %v, want authored Call %v", fixed, fixedOK, tail, term)
	}
	boundary, boundaryOK := flow.Causal().Boundaries().For(term)
	if !boundaryOK || boundary.Throw == 0 || boundary.Yield == 0 || boundary.Cancel == 0 {
		t.Fatal("Call lacks a shared non-normal outcome")
	}
}

func applicationOnlyFunctionExpr(t *testing.T, stmts []ast.Stmt) *ast.FunctionExpr {
	t.Helper()
	var functions []*ast.FunctionExpr
	applicationWalk(stmts, func(node ast.PositionHolder) {
		if function, ok := node.(*ast.FunctionExpr); ok {
			functions = append(functions, function)
		}
	})
	if len(functions) != 1 {
		t.Fatalf("plain call source Functions = %d, want exactly one", len(functions))
	}
	return functions[0]
}

func applicationCallAt(t *testing.T, p *program.Program, node ast.PositionHolder) keyspace.Term {
	t.Helper()
	calls := p.Flow().Authored().Calls()
	return applicationTermAt(t, p, node, calls.Count, calls.At, "Call")
}

func applicationOutcome(
	t *testing.T,
	p *program.Program,
	exit, body keyspace.Term,
	kind flowkind.OutcomeKind,
	next keyspace.Term,
) {
	t.Helper()
	got, ok := p.Flow().Outcomes().Get(exit)
	gotBody, gotKind, target := got.Body, got.Kind, got.Target
	if !ok || gotBody != body || gotKind != kind || target != 0 {
		t.Fatalf("Outcome(%v) = %v/%v/%v/%v, want %v/%v/0/true", exit, gotBody, gotKind, target, ok, body, kind)
	}
	gotNext, nextOK := p.Flow().Outcomes().Propagation(exit)
	if next == 0 {
		if nextOK {
			t.Fatalf("OutcomeSuccessor(%v) = %v/%v, want terminal", exit, gotNext, nextOK)
		}
		return
	}
	if !nextOK || gotNext != next {
		t.Fatalf("OutcomeSuccessor(%v) = %v/%v, want %v", exit, gotNext, nextOK, next)
	}
}
