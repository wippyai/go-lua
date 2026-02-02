package cond

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/core"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestExtractEdgeConstraints_NilGraph(t *testing.T) {
	fc := &core.FlowContext{}
	inputs := &flow.Inputs{}
	ExtractEdgeConstraints(fc, inputs)
	if len(inputs.EdgeConditions) != 0 {
		t.Error("nil graph should produce no edge conditions")
	}
}

func TestExtractNumericConstraints_NilGraph(t *testing.T) {
	fc := &core.FlowContext{}
	inputs := &flow.Inputs{}
	ExtractNumericConstraints(fc, inputs)
	if len(inputs.EdgeNumericConstraints) != 0 {
		t.Error("nil graph should produce no numeric constraints")
	}
}

func TestFindBranchEdges_EmptySuccessors(t *testing.T) {
	trueEdge, falseEdge := FindBranchEdges(nil, 0, nil)
	if trueEdge != 0 || falseEdge != 0 {
		t.Error("empty successors should return zero edges")
	}
}

func TestComputeDeadPoints_EmptyGraph(t *testing.T) {
	result := ComputeDeadPoints(nil, nil, nil, nil)
	if result == nil {
		t.Error("should return non-nil map")
	}
}

func TestExtractLenOfPath_NilExpr(t *testing.T) {
	result := ExtractLenOfPath(nil, 0, nil)
	if !result.IsEmpty() {
		t.Error("nil expr should return empty path")
	}
}

func TestExtractLenOfPath_NonLenOp(t *testing.T) {
	expr := &ast.NumberExpr{Value: "42"}
	result := ExtractLenOfPath(expr, 0, nil)
	if !result.IsEmpty() {
		t.Error("non-len expr should return empty path")
	}
}

func TestExtractCallOnReturnConstraints_NilGraph(t *testing.T) {
	fc := &core.FlowContext{}
	inputs := &flow.Inputs{}
	result := ExtractCallOnReturnConstraints(fc, inputs)
	if len(result) != 0 {
		t.Error("nil graph should return empty map")
	}
}

func TestConstraintsFromCallOnReturn_NilInfo(t *testing.T) {
	result := ConstraintsFromCallOnReturn(nil, 0, nil, nil, nil, nil, nil, nil, nil, nil)
	if result.HasConstraints() {
		t.Error("nil info should produce no constraints")
	}
}

func TestConstraintsFromAssignOnReturn_NilInfo(t *testing.T) {
	result := ConstraintsFromAssignOnReturn(nil, 0, nil, nil, nil, nil, nil, nil, nil, nil)
	if result.HasConstraints() {
		t.Error("nil info should produce no constraints")
	}
}

func TestExtractEffectFromType_NilType(t *testing.T) {
	result := ExtractEffectFromType(nil)
	if result != nil {
		t.Error("nil type should return nil effect")
	}
}

func TestExtractEffectFromType_NonFunction(t *testing.T) {
	result := ExtractEffectFromType(typ.String)
	if result != nil {
		t.Error("non-function type should return nil effect")
	}
}

func TestExtractEffectFromType_FunctionNoRefinement(t *testing.T) {
	fn := typ.Func().Returns(typ.String).Build()
	result := ExtractEffectFromType(fn)
	if result != nil {
		t.Error("function without refinement should return nil effect")
	}
}

func TestResolveCalleeToFunctionLiteral_NilCallee(t *testing.T) {
	result := ResolveCalleeToFunctionLiteral(nil, nil)
	if result != nil {
		t.Error("nil callee should return nil")
	}
}

func TestResolveCalleeToFunctionLiteral_FunctionExpr(t *testing.T) {
	fn := &ast.FunctionExpr{}
	result := ResolveCalleeToFunctionLiteral(fn, nil)
	if result != fn {
		t.Error("FunctionExpr should be returned as-is")
	}
}

func TestResolveSymbolToFunctionLiteral_NilGraph(t *testing.T) {
	result := ResolveSymbolToFunctionLiteral(nil, 1)
	if result != nil {
		t.Error("nil graph should return nil")
	}
}

func TestResolveSymbolToFunctionLiteral_ZeroSymbol(t *testing.T) {
	result := ResolveSymbolToFunctionLiteral(nil, 0)
	if result != nil {
		t.Error("zero symbol should return nil")
	}
}

func TestResolveExprToTableLiteral_NilExpr(t *testing.T) {
	result := ResolveExprToTableLiteral(nil, nil)
	if result != nil {
		t.Error("nil expr should return nil")
	}
}

func TestResolveExprToTableLiteral_NilGraph(t *testing.T) {
	tbl := &ast.TableExpr{}
	result := ResolveExprToTableLiteral(tbl, nil)
	if result != nil {
		t.Error("nil graph should return nil")
	}
}

func TestCallTerminates_NilInfo(t *testing.T) {
	result := CallTerminates(nil, 0, nil, nil, nil, nil)
	if result {
		t.Error("nil info should return false")
	}
}

func TestExtractPredicateLinkFromCallInfo_NilInfo(t *testing.T) {
	result := ExtractPredicateLinkFromCallInfo(nil, 0, nil, nil, nil, nil, nil, nil, nil)
	if result != nil {
		t.Error("nil info should return nil")
	}
}

func TestComputeDeadPoints_NilGraph(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Error("should not panic with nil graph")
		}
	}()
	_ = ComputeDeadPoints(nil, nil, nil, nil)
}
