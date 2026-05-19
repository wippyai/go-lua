package pipeline

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/diag"
)

func TestSortedResultFunctions_Empty(t *testing.T) {
	result := SortedResultFunctions(nil)
	if result != nil {
		t.Error("expected nil for empty map")
	}
	result = SortedResultFunctions(map[*ast.FunctionExpr]*api.FuncResult{})
	if result != nil {
		t.Error("expected nil for empty map")
	}
}

func TestSortedResultFunctions_SortByLine(t *testing.T) {
	fn1 := &ast.FunctionExpr{}
	fn1.SetLine(10)
	fn2 := &ast.FunctionExpr{}
	fn2.SetLine(5)
	fn3 := &ast.FunctionExpr{}
	fn3.SetLine(15)

	results := map[*ast.FunctionExpr]*api.FuncResult{
		fn1: {},
		fn2: {},
		fn3: {},
	}

	sorted := SortedResultFunctions(results)
	if len(sorted) != 3 {
		t.Fatalf("expected 3 functions, got %d", len(sorted))
	}
	if sorted[0].Line() != 5 {
		t.Errorf("expected first function at line 5, got %d", sorted[0].Line())
	}
	if sorted[1].Line() != 10 {
		t.Errorf("expected second function at line 10, got %d", sorted[1].Line())
	}
	if sorted[2].Line() != 15 {
		t.Errorf("expected third function at line 15, got %d", sorted[2].Line())
	}
}

func TestSortedResultFunctions_SortByColumn(t *testing.T) {
	fn1 := &ast.FunctionExpr{}
	fn1.SetLine(10)
	fn1.SetColumn(20)
	fn2 := &ast.FunctionExpr{}
	fn2.SetLine(10)
	fn2.SetColumn(5)

	results := map[*ast.FunctionExpr]*api.FuncResult{
		fn1: {},
		fn2: {},
	}

	sorted := SortedResultFunctions(results)
	if sorted[0].Column() != 5 {
		t.Errorf("expected first function at column 5, got %d", sorted[0].Column())
	}
	if sorted[1].Column() != 20 {
		t.Errorf("expected second function at column 20, got %d", sorted[1].Column())
	}
}

func TestSortedResultFunctions_NilFunction(t *testing.T) {
	fn1 := &ast.FunctionExpr{}
	fn1.SetLine(10)

	results := map[*ast.FunctionExpr]*api.FuncResult{
		fn1: {},
		nil: {},
	}

	sorted := SortedResultFunctions(results)
	if len(sorted) != 2 {
		t.Fatalf("expected 2 functions, got %d", len(sorted))
	}
	if sorted[0] != fn1 {
		t.Error("non-nil function should come first")
	}
}

func TestSortDiagnostics_Empty(t *testing.T) {
	SortDiagnostics(nil)
	SortDiagnostics([]diag.Diagnostic{})
	SortDiagnostics([]diag.Diagnostic{{Message: "single"}})
}

func TestSortDiagnostics_ByLine(t *testing.T) {
	diags := []diag.Diagnostic{
		{Position: diag.Position{Line: 20}, Message: "second"},
		{Position: diag.Position{Line: 10}, Message: "first"},
		{Position: diag.Position{Line: 30}, Message: "third"},
	}

	SortDiagnostics(diags)

	if diags[0].Position.Line != 10 {
		t.Errorf("expected first at line 10, got %d", diags[0].Position.Line)
	}
	if diags[1].Position.Line != 20 {
		t.Errorf("expected second at line 20, got %d", diags[1].Position.Line)
	}
	if diags[2].Position.Line != 30 {
		t.Errorf("expected third at line 30, got %d", diags[2].Position.Line)
	}
}

func TestSortDiagnostics_ByColumn(t *testing.T) {
	diags := []diag.Diagnostic{
		{Position: diag.Position{Line: 10, Column: 20}},
		{Position: diag.Position{Line: 10, Column: 5}},
	}

	SortDiagnostics(diags)

	if diags[0].Position.Column != 5 {
		t.Errorf("expected first at column 5, got %d", diags[0].Position.Column)
	}
}

func TestSortDiagnostics_ByFile(t *testing.T) {
	diags := []diag.Diagnostic{
		{Position: diag.Position{File: "b.lua"}},
		{Position: diag.Position{File: "a.lua"}},
	}

	SortDiagnostics(diags)

	if diags[0].Position.File != "a.lua" {
		t.Errorf("expected a.lua first, got %s", diags[0].Position.File)
	}
}

func TestSortDiagnostics_ByMessage(t *testing.T) {
	diags := []diag.Diagnostic{
		{Position: diag.Position{Line: 1}, Message: "beta"},
		{Position: diag.Position{Line: 1}, Message: "alpha"},
	}

	SortDiagnostics(diags)

	if diags[0].Message != "alpha" {
		t.Errorf("expected alpha first, got %s", diags[0].Message)
	}
}

func TestResolveSymbolName_NilGraph(t *testing.T) {
	name := ResolveSymbolName(nil, 1)
	if name != "" {
		t.Error("expected empty string for nil graph")
	}
}

func TestResolveSymbolName_WithGraph(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"x"}},
	}
	graph := cfg.Build(fn)
	syms := graph.ParamSymbols()
	if len(syms) == 0 {
		t.Skip("no param symbols in test graph")
	}
	name := ResolveSymbolName(graph, syms[0])
	if name != "x" {
		t.Errorf("expected 'x', got '%s'", name)
	}
}
