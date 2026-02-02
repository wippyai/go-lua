package keyscoll_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/keyscoll"
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
