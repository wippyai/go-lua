package call

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

func TestStaticAccessDirectFieldRequiresOneSegment(t *testing.T) {
	t.Parallel()

	base := &ast.IdentExpr{Value: "mod"}
	field := &ast.AttrGetExpr{Object: base, Key: &ast.StringExpr{Value: "run"}}
	nested := &ast.AttrGetExpr{Object: field, Key: &ast.StringExpr{Value: "again"}}
	bindings := bind.NewBindingTable()
	bindings.Bind(base, cfg.SymbolID(77))
	bindings.SetName(cfg.SymbolID(77), "mod")
	access := staticAccess{Bindings: bindings}

	sym, key, ok := access.directField(field)
	if !ok || sym != cfg.SymbolID(77) || key.Name != "run" {
		t.Fatalf("directField(field) = (%d,%+v,%v), want (77,run,true)", sym, key, ok)
	}
	if _, _, ok := access.directField(nested); ok {
		t.Fatal("directField accepted deeper-than-one-segment path")
	}
}

func TestStaticAccessMethodPathAppendsWithoutMutatingReceiverPath(t *testing.T) {
	t.Parallel()

	recv := &ast.IdentExpr{Value: "svc"}
	bindings := bind.NewBindingTable()
	bindings.Bind(recv, cfg.SymbolID(88))
	bindings.SetName(cfg.SymbolID(88), "svc")
	access := staticAccess{Bindings: bindings}

	receiverPath, ok := access.exprPath(recv)
	if !ok {
		t.Fatal("receiver path was not produced")
	}
	methodPath, ok := access.methodPath(&ast.FuncCallExpr{Receiver: recv, Method: "start"})
	if !ok {
		t.Fatal("method path was not produced")
	}
	wantMethod := constraint.NewPath(cfg.SymbolID(88), "svc").Field("start")
	if !methodPath.Equal(wantMethod) {
		t.Fatalf("methodPath = %s, want %s", methodPath.Key(), wantMethod.Key())
	}
	if !receiverPath.Equal(constraint.NewPath(cfg.SymbolID(88), "svc")) {
		t.Fatalf("receiver path mutated to %s", receiverPath.Key())
	}
}
