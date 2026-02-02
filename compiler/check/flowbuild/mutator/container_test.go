package mutator

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/core"
	"github.com/wippyai/go-lua/types/flow"
)

func TestExtractContainerMutatorAssignments_NilGraph(t *testing.T) {
	fc := &core.FlowContext{}
	inputs := &flow.Inputs{}

	ExtractContainerMutatorAssignments(fc, inputs)

	if len(inputs.ContainerMutatorAssignments) != 0 {
		t.Errorf("expected no assignments for nil graph, got %d", len(inputs.ContainerMutatorAssignments))
	}
}

func TestExtractContainerMutatorAssignments_NilInputs(t *testing.T) {
	fc := &core.FlowContext{}

	ExtractContainerMutatorAssignments(fc, nil)
}

func TestContainerElementReturnInfo(t *testing.T) {
	info := ContainerElementReturnInfo{
		ReturnIndex: 1,
	}
	if info.ReturnIndex != 1 {
		t.Errorf("expected ReturnIndex 1, got %d", info.ReturnIndex)
	}
}

func TestContainerElementReturnFromCall_NilInfo(t *testing.T) {
	result := ContainerElementReturnFromCall(nil, 0, nil, nil, nil)
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

	result := containerArgAtMethod(receiver, args, 0)
	if result != receiver {
		t.Error("expected receiver at index 0")
	}
}

func TestContainerArgAtMethod_ArgsAfterReceiver(t *testing.T) {
	receiver := &ast.IdentExpr{Value: "self"}
	arg1 := &ast.NumberExpr{Value: "1"}
	arg2 := &ast.NumberExpr{Value: "2"}
	args := []ast.Expr{arg1, arg2}

	result := containerArgAtMethod(receiver, args, 1)
	if result != arg1 {
		t.Error("expected first arg at index 1")
	}

	result = containerArgAtMethod(receiver, args, 2)
	if result != arg2 {
		t.Error("expected second arg at index 2")
	}
}

func TestContainerArgAtMethod_OutOfBounds(t *testing.T) {
	receiver := &ast.IdentExpr{Value: "self"}
	args := []ast.Expr{&ast.NumberExpr{Value: "1"}}

	result := containerArgAtMethod(receiver, args, 5)
	if result != nil {
		t.Error("expected nil for out of bounds index")
	}
}
