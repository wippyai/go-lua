package keyscoll_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/keyscoll"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestKeysCollectorInfo_ParamIndex(t *testing.T) {
	info := &keyscoll.KeysCollectorInfo{ParamIndex: 2}
	if info.ParamIndex != 2 {
		t.Errorf("expected ParamIndex 2, got %d", info.ParamIndex)
	}
}

func TestDetectKeysCollector_NilFunction(t *testing.T) {
	result := keyscoll.DetectKeysCollector(nil)
	if result != nil {
		t.Error("expected nil for nil function")
	}
}

func TestDetectKeysCollector_NilStmts(t *testing.T) {
	fn := &ast.FunctionExpr{Stmts: nil}
	result := keyscoll.DetectKeysCollector(fn)
	if result != nil {
		t.Error("expected nil for nil statements")
	}
}

func TestDetectKeysCollector_EmptyStmts(t *testing.T) {
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{}}
	result := keyscoll.DetectKeysCollector(fn)
	if result != nil {
		t.Error("expected nil for empty statements")
	}
}

func TestDetectKeysCollector_SimpleReturn(t *testing.T) {
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.ReturnStmt{Exprs: []ast.Expr{&ast.NilExpr{}}},
		},
	}
	result := keyscoll.DetectKeysCollector(fn)
	if result != nil {
		t.Error("expected nil for simple return function")
	}
}

func TestDetectKeysCollector_NoKeysPattern(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"tbl"}},
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{
				Names: []string{"result"},
				Exprs: []ast.Expr{&ast.TableExpr{}},
			},
			&ast.ReturnStmt{
				Exprs: []ast.Expr{&ast.IdentExpr{Value: "result"}},
			},
		},
	}
	result := keyscoll.DetectKeysCollector(fn)
	if result != nil {
		t.Error("expected nil for function without keys pattern")
	}
}

func TestBuildKeysCollectorDetector_NilCallInfo(t *testing.T) {
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{&ast.ReturnStmt{}},
	}
	graph := cfg.Build(fn)
	detector := keyscoll.BuildKeysCollectorDetector(graph)
	if detector == nil {
		t.Fatal("expected non-nil detector")
	}
	result := detector(nil, 0)
	if result != 0 {
		t.Error("expected 0 for nil call info")
	}
}

func TestBuildKeysCollectorDetector_MethodCall(t *testing.T) {
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{&ast.ReturnStmt{}},
	}
	graph := cfg.Build(fn)
	detector := keyscoll.BuildKeysCollectorDetector(graph)
	callInfo := &cfg.CallInfo{
		Method:   "someMethod",
		Receiver: &ast.IdentExpr{Value: "obj"},
	}
	result := detector(callInfo, 0)
	if result != 0 {
		t.Error("expected 0 for method call")
	}
}

func TestBuildKeysCollectorDetector_NoCalleeSymbol(t *testing.T) {
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{&ast.ReturnStmt{}},
	}
	graph := cfg.Build(fn)
	detector := keyscoll.BuildKeysCollectorDetector(graph)
	callInfo := &cfg.CallInfo{
		Callee:       &ast.IdentExpr{Value: "fn"},
		CalleeSymbol: 0,
	}
	result := detector(callInfo, 0)
	if result != 0 {
		t.Error("expected 0 for no callee symbol")
	}
}

func TestDetectKeysCollector_TableInsertAsAssignmentCallSite(t *testing.T) {
	body, err := parse.ParseString(`
		local keys = {}
		for k in pairs(tbl) do
			local _ = table.insert(keys, k)
		end
		return keys
	`, "test.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"tbl"}},
		Stmts:   body,
	}
	info := keyscoll.DetectKeysCollector(fn)
	if info == nil {
		t.Fatal("expected keys collector to be detected when insert call is in assignment expression")
	}
	if info.ParamIndex != 0 {
		t.Fatalf("expected param index 0, got %d", info.ParamIndex)
	}
}

func TestBuildKeysCollectorDetector_NestedFieldArgument(t *testing.T) {
	body, err := parse.ParseString(`
		local function sorted_keys(tbl)
			local keys = {}
			for k in pairs(tbl) do
				local _ = table.insert(keys, k)
			end
			return keys
		end
		local state = {}
		local keys = sorted_keys(state.users)
	`, "test.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{HasVargs: true},
		Stmts:   body,
	}
	graph := cfg.Build(fn, "pairs", "table")
	if graph == nil {
		t.Fatal("expected graph")
	}
	bindings := graph.Bindings()
	if bindings == nil {
		t.Fatal("expected bindings")
	}

	stateSym, ok := graph.SymbolAt(graph.Exit(), "state")
	if !ok || stateSym == 0 {
		t.Fatalf("expected symbol for state, got %d", stateSym)
	}
	want := bindings.GetOrCreateFieldSymbol(stateSym, "users")

	detector := keyscoll.BuildKeysCollectorDetector(graph)
	found := false
	graph.EachCallSite(func(p cfg.Point, info *cfg.CallInfo) {
		if info == nil || info.CalleeName != "sorted_keys" {
			return
		}
		found = true
		if got := detector(info, p); got != want {
			t.Fatalf("detector(sorted_keys(state.users)) = %d, want %d", got, want)
		}
	})
	if !found {
		t.Fatal("expected sorted_keys call site")
	}
}
