package mutator

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/domain/calleffect"
)

func TestContainerElementReturnInfo(t *testing.T) {
	info := calleffect.ContainerElementReturnInfo{
		ReturnIndex: 1,
	}
	if info.ReturnIndex != 1 {
		t.Errorf("expected ReturnIndex 1, got %d", info.ReturnIndex)
	}
}

func TestContainerElementReturnFromCall_NilInfo(t *testing.T) {
	result := calleffect.ContainerElementReturnFromCall(nil, 0, nil, nil, nil, nil, nil, nil)
	if result != nil {
		t.Error("expected nil for nil info")
	}
}

func TestContainerArgAtMethod_ReceiverAtIndex0(t *testing.T) {
	receiver := &ast.IdentExpr{Value: "self"}
	args := []ast.Expr{
		&ast.NumberExpr{Value: "1"},
		&ast.NumberExpr{Value: "2"},
	}

	result := callsite.RuntimeArgAt(&cfg.CallInfo{
		Method:   "method",
		Receiver: receiver,
		Args:     args,
	}, 0)
	if result != receiver {
		t.Error("expected receiver at index 0")
	}
}

func TestContainerArgAtMethod_ArgsAfterReceiver(t *testing.T) {
	receiver := &ast.IdentExpr{Value: "self"}
	arg1 := &ast.NumberExpr{Value: "1"}
	arg2 := &ast.NumberExpr{Value: "2"}
	args := []ast.Expr{arg1, arg2}

	result := callsite.RuntimeArgAt(&cfg.CallInfo{
		Method:   "method",
		Receiver: receiver,
		Args:     args,
	}, 1)
	if result != arg1 {
		t.Error("expected first arg at index 1")
	}

	result = callsite.RuntimeArgAt(&cfg.CallInfo{
		Method:   "method",
		Receiver: receiver,
		Args:     args,
	}, 2)
	if result != arg2 {
		t.Error("expected second arg at index 2")
	}
}

func TestContainerArgAtMethod_OutOfBounds(t *testing.T) {
	receiver := &ast.IdentExpr{Value: "self"}
	args := []ast.Expr{&ast.NumberExpr{Value: "1"}}

	result := callsite.RuntimeArgAt(&cfg.CallInfo{
		Method:   "method",
		Receiver: receiver,
		Args:     args,
	}, 5)
	if result != nil {
		t.Error("expected nil for out of bounds index")
	}
}
