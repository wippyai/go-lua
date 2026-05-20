package callsite

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestFunctionLiteralForSymbol_BindingTableLiteral(t *testing.T) {
	bindings := bind.NewBindingTable()
	fn := &ast.FunctionExpr{}
	sym := cfg.SymbolID(41)
	bindings.SetFuncLitSymbol(fn, sym)

	got := FunctionLiteralForSymbol(bindings, api.FlowEvidence{}, sym)
	if got != fn {
		t.Fatalf("FunctionLiteralForSymbol() = %v, want %v", got, fn)
	}
}

func TestFunctionLiteralForSymbol_FuncDefSymbol(t *testing.T) {
	stmts, err := parse.ParseString(`
		local function cb(x)
			return x
		end
		cb(1)
	`, "test.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	graph := cfg.Build(&ast.FunctionExpr{
		ParList: &ast.ParList{},
		Stmts:   stmts,
	})
	if graph == nil {
		t.Fatal("expected graph")
	}

	var calleeSym cfg.SymbolID
	graph.EachCallSite(func(_ cfg.Point, info *cfg.CallInfo) {
		if info != nil && info.CalleeName == "cb" {
			calleeSym = info.CalleeSymbol
		}
	})
	if calleeSym == 0 {
		t.Fatal("expected callsite callee symbol")
	}

	evidence := testFunctionEvidence(graph)
	fn := FunctionLiteralForSymbol(graph.Bindings(), evidence, calleeSym)
	if fn == nil {
		t.Fatal("expected function literal for local function symbol")
	}
}

func TestFunctionLiteralForSymbol_AssignedFunctionLiteral(t *testing.T) {
	stmts, err := parse.ParseString(`
		local cb = function(x)
			return x
		end
		cb(1)
	`, "test.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	graph := cfg.Build(&ast.FunctionExpr{
		ParList: &ast.ParList{},
		Stmts:   stmts,
	})
	if graph == nil {
		t.Fatal("expected graph")
	}

	var calleeSym cfg.SymbolID
	graph.EachCallSite(func(_ cfg.Point, info *cfg.CallInfo) {
		if info != nil && info.CalleeName == "cb" {
			calleeSym = info.CalleeSymbol
		}
	})
	if calleeSym == 0 {
		t.Fatal("expected callsite callee symbol")
	}

	evidence := testFunctionEvidence(graph)
	fn := FunctionLiteralForSymbol(graph.Bindings(), evidence, calleeSym)
	if fn == nil {
		t.Fatal("expected function literal for assigned symbol")
	}
}

func TestFunctionLiteralForGraphSymbol_FuncDefSymbol(t *testing.T) {
	stmts, err := parse.ParseString(`
		local M = {}
		function M.run()
			return 1
		end
		M.run()
	`, "test.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	graph := cfg.Build(&ast.FunctionExpr{
		ParList: &ast.ParList{},
		Stmts:   stmts,
	})
	if graph == nil {
		t.Fatal("expected graph")
	}

	var calleeSym cfg.SymbolID
	graph.EachCallSite(func(_ cfg.Point, info *cfg.CallInfo) {
		if info != nil && info.CalleeName == "run" {
			calleeSym = info.CalleeSymbol
		}
	})
	if calleeSym == 0 {
		t.Fatal("expected callsite callee symbol")
	}

	evidence := testFunctionEvidence(graph)
	fn := FunctionLiteralForGraphSymbol(evidence, calleeSym)
	if fn == nil {
		t.Fatal("expected graph-local function literal for field definition")
	}
}

func TestFunctionLiteralForGraphSymbol_IgnoresMutableFieldPathBinding(t *testing.T) {
	stmts, err := parse.ParseString(`
		local M = {
			dep = {
				get = function()
					return nil
				end,
			},
		}
		M.dep = {
			get = function()
				return 1
			end,
		}
		M.dep.get()
	`, "test.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	graph := cfg.Build(&ast.FunctionExpr{
		ParList: &ast.ParList{},
		Stmts:   stmts,
	})
	if graph == nil {
		t.Fatal("expected graph")
	}

	var calleeSym cfg.SymbolID
	graph.EachCallSite(func(_ cfg.Point, info *cfg.CallInfo) {
		if info != nil && info.CalleeName == "get" {
			calleeSym = info.CalleeSymbol
		}
	})
	if calleeSym == 0 {
		t.Fatal("expected callsite callee symbol")
	}

	evidence := testFunctionEvidence(graph)
	if fn := FunctionLiteralForGraphSymbol(evidence, calleeSym); fn != nil {
		t.Fatalf("expected mutable field-path symbol to stay unresolved in graph-local resolver, got %v", fn)
	}
	if fn := FunctionLiteralForSymbol(graph.Bindings(), evidence, calleeSym); fn == nil {
		t.Fatal("expected binder-level symbol resolver to still find a literal for the shared field symbol")
	}
}

func testFunctionEvidence(graph *cfg.Graph) api.FlowEvidence {
	if graph == nil {
		return api.FlowEvidence{}
	}
	var defs []api.FunctionDefinitionEvidence
	for _, nf := range graph.NestedFunctions() {
		if nf.Func == nil {
			continue
		}
		info := graph.FuncDef(nf.Point)
		sym := nf.Symbol
		if info != nil && info.Symbol != 0 {
			sym = info.Symbol
		}
		defs = append(defs, api.FunctionDefinitionEvidence{
			Nested:  nf,
			FuncDef: info,
			Symbol:  sym,
		})
	}
	return api.FlowEvidence{FunctionDefinitions: defs}
}
