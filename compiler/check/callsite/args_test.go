package callsite

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
)

func TestPositionalArgAt(t *testing.T) {
	a := &ast.NumberExpr{Value: "1"}
	b := &ast.NumberExpr{Value: "2"}
	args := []ast.Expr{a, b}

	if got := PositionalArgAt(args, 0); got != a {
		t.Fatal("expected first arg at index 0")
	}
	if got := PositionalArgAt(args, -1); got != b {
		t.Fatal("expected last arg at index -1")
	}
	if got := PositionalArgAt(args, 5); got != nil {
		t.Fatal("expected nil for out-of-range index")
	}
	if got := PositionalArgAt(nil, 0); got != nil {
		t.Fatal("expected nil for empty args")
	}
}

func TestRuntimeArgAt_DirectCall(t *testing.T) {
	a := &ast.NumberExpr{Value: "1"}
	b := &ast.NumberExpr{Value: "2"}
	call := &ast.FuncCallExpr{Args: []ast.Expr{a, b}}
	info := &cfg.CallInfo{Call: call, Args: call.Args}
	if got := RuntimeArgCount(info); got != 2 {
		t.Fatalf("RuntimeArgCount(direct) = %d, want 2", got)
	}
	if got := RuntimeArgExprCount(call); got != 2 {
		t.Fatalf("RuntimeArgExprCount(direct) = %d, want 2", got)
	}

	if got := RuntimeArgAt(info, 0); got != a {
		t.Fatal("expected first positional arg for direct call")
	}
	if got := RuntimeArgExprAt(call, 0); got != a {
		t.Fatal("expected first positional arg for direct AST call")
	}
	if got := RuntimeArgAt(info, -1); got != b {
		t.Fatal("expected last positional arg for direct call")
	}
	if got := RuntimeArgExprAt(call, -1); got != b {
		t.Fatal("expected last positional arg for direct AST call")
	}
}

func TestRuntimeArgAt_MethodCall(t *testing.T) {
	recv := &ast.IdentExpr{Value: "self"}
	a := &ast.NumberExpr{Value: "1"}
	b := &ast.NumberExpr{Value: "2"}
	call := &ast.FuncCallExpr{
		Receiver: recv,
		Method:   "send",
		Args:     []ast.Expr{a, b},
	}
	info := &cfg.CallInfo{
		Call:     call,
		Method:   "send",
		Receiver: recv,
		Args:     []ast.Expr{a, b},
	}
	if got := RuntimeArgCount(info); got != 3 {
		t.Fatalf("RuntimeArgCount(method) = %d, want 3", got)
	}
	if got := RuntimeArgExprCount(call); got != 3 {
		t.Fatalf("RuntimeArgExprCount(method) = %d, want 3", got)
	}

	if got := RuntimeArgAt(info, 0); got != recv {
		t.Fatal("expected receiver at runtime index 0")
	}
	if got := RuntimeArgExprAt(call, 0); got != recv {
		t.Fatal("expected receiver at runtime index 0 for AST call")
	}
	if got := RuntimeArgAt(info, 1); got != a {
		t.Fatal("expected first positional arg at runtime index 1")
	}
	if got := RuntimeArgExprAt(call, 1); got != a {
		t.Fatal("expected first positional arg at runtime index 1 for AST call")
	}
	if got := RuntimeArgAt(info, -1); got != b {
		t.Fatal("expected last runtime arg for method call")
	}
	if got := RuntimeArgExprAt(call, -1); got != b {
		t.Fatal("expected last runtime arg for AST method call")
	}
	if got := RuntimeArgAt(info, -3); got != recv {
		t.Fatal("expected receiver for runtime index -3 with 2 args")
	}
	if got := RuntimeArgExprAt(call, -3); got != recv {
		t.Fatal("expected receiver for AST runtime index -3 with 2 args")
	}
}

func TestRuntimeArgCount_Nil(t *testing.T) {
	if got := RuntimeArgCount(nil); got != 0 {
		t.Fatalf("RuntimeArgCount(nil) = %d, want 0", got)
	}
}
