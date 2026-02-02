package mutator

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/core"
	"github.com/wippyai/go-lua/types/flow"
)

func TestExtractTableMutatorAssignments_NilGraph(t *testing.T) {
	fc := &core.FlowContext{}
	inputs := &flow.Inputs{}

	ExtractTableMutatorAssignments(fc, inputs)

	if len(inputs.TableMutatorAssignments) != 0 {
		t.Errorf("expected no assignments for nil graph, got %d", len(inputs.TableMutatorAssignments))
	}
}

func TestExtractTableMutatorAssignments_NilInputs(t *testing.T) {
	fc := &core.FlowContext{}

	ExtractTableMutatorAssignments(fc, nil)
}

func TestTableMutatorFromCall_NilInfo(t *testing.T) {
	result := TableMutatorFromCall(nil, 0, nil, nil)
	if result != nil {
		t.Error("expected nil for nil info")
	}
}

func TestArgAtCall_EmptyArgs(t *testing.T) {
	result := ArgAtCall(nil, 0)
	if result != nil {
		t.Error("expected nil for empty args")
	}

	result = ArgAtCall([]ast.Expr{}, 0)
	if result != nil {
		t.Error("expected nil for empty slice")
	}
}

func TestArgAtCall_ValidIndex(t *testing.T) {
	arg0 := &ast.NumberExpr{Value: "1"}
	arg1 := &ast.NumberExpr{Value: "2"}
	arg2 := &ast.NumberExpr{Value: "3"}
	args := []ast.Expr{arg0, arg1, arg2}

	if ArgAtCall(args, 0) != arg0 {
		t.Error("expected arg0 at index 0")
	}
	if ArgAtCall(args, 1) != arg1 {
		t.Error("expected arg1 at index 1")
	}
	if ArgAtCall(args, 2) != arg2 {
		t.Error("expected arg2 at index 2")
	}
}

func TestArgAtCall_NegativeIndex(t *testing.T) {
	arg0 := &ast.NumberExpr{Value: "1"}
	arg1 := &ast.NumberExpr{Value: "2"}
	arg2 := &ast.NumberExpr{Value: "3"}
	args := []ast.Expr{arg0, arg1, arg2}

	if ArgAtCall(args, -1) != arg2 {
		t.Error("expected arg2 at index -1")
	}
	if ArgAtCall(args, -2) != arg1 {
		t.Error("expected arg1 at index -2")
	}
	if ArgAtCall(args, -3) != arg0 {
		t.Error("expected arg0 at index -3")
	}
}

func TestArgAtCall_OutOfBounds(t *testing.T) {
	args := []ast.Expr{&ast.NumberExpr{Value: "1"}}

	if ArgAtCall(args, 5) != nil {
		t.Error("expected nil for index out of bounds")
	}
	if ArgAtCall(args, -5) != nil {
		t.Error("expected nil for negative index out of bounds")
	}
}

func TestKeyTypeFromExpr_NilExpr(t *testing.T) {
	result := keyTypeFromExpr(nil, nil)
	if result != nil {
		t.Error("expected nil for nil expr")
	}
}
