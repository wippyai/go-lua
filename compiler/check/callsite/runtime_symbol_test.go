package callsite

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
)

func TestSymbolOrCreateFieldFromExpr_StaticFieldPath(t *testing.T) {
	bindings := bind.NewBindingTable()
	base := &ast.IdentExpr{Value: "state"}
	baseSym := cfg.SymbolID(11)
	bindings.Bind(base, baseSym)

	expr := &ast.AttrGetExpr{
		Object: &ast.AttrGetExpr{
			Object: base,
			Key:    &ast.IdentExpr{Value: "users"},
		},
		Key: &ast.IdentExpr{Value: "active"},
	}

	got := SymbolOrCreateFieldFromExpr(expr, bindings)
	if got == 0 {
		t.Fatal("expected field symbol")
	}
	want := bindings.GetOrCreateFieldSymbol(baseSym, "users.active")
	if got != want {
		t.Fatalf("SymbolOrCreateFieldFromExpr(...) = %d, want %d", got, want)
	}
}

func TestRuntimeArgSymbolAt_MethodCallUsesReceiver(t *testing.T) {
	bindings := bind.NewBindingTable()
	recv := &ast.IdentExpr{Value: "state"}
	recvSym := cfg.SymbolID(22)
	bindings.Bind(recv, recvSym)
	arg := &ast.IdentExpr{Value: "x"}
	argSym := cfg.SymbolID(23)
	bindings.Bind(arg, argSym)

	info := &cfg.CallInfo{
		Method:   "sorted_keys",
		Receiver: recv,
		Args:     []ast.Expr{arg},
	}

	if got := RuntimeArgSymbolAt(info, 0, bindings); got != recvSym {
		t.Fatalf("RuntimeArgSymbolAt(method,0) = %d, want receiver %d", got, recvSym)
	}
	if got := RuntimeArgSymbolAt(info, 1, bindings); got != argSym {
		t.Fatalf("RuntimeArgSymbolAt(method,1) = %d, want arg %d", got, argSym)
	}
}
