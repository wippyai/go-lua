package sourceprovenance

import (
	"testing"

	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/compiler/ast"
)

func ident(name string) *ast.IdentExpr {
	return &ast.IdentExpr{Value: name}
}

func call(name string) *ast.FuncCallExpr {
	return &ast.FuncCallExpr{Func: ident(name)}
}

func wrappedCall(name string) ast.Expr {
	return &ast.CastExpr{
		Expr: &ast.NonNilAssertExpr{
			Expr: call(name),
		},
	}
}

func TestAssignmentSourcesShape(t *testing.T) {
	t.Run("fixed targets and expanding final call", func(t *testing.T) {
		exprs := []ast.Expr{ident("x"), call("pack")}
		sources := AssignmentSources(exprs, 4, nil)
		if len(sources) != 4 {
			t.Fatalf("len(sources) = %d, want 4", len(sources))
		}

		if got := sources[0]; got.Kind != factflow.ValueSourceExpression || got.Expr != exprs[0] || got.ExprIndex != 0 || got.TargetIndex != 0 || got.ResultIndex != 0 || got.Final || got.Expanded || got.Adjusted {
			t.Fatalf("fixed target source = %#v", got)
		}
		if got := sources[1]; got.Kind != factflow.ValueSourceCall || got.Expr != exprs[1] || got.ExprIndex != 1 || got.TargetIndex != 1 || got.ResultIndex != 0 || !got.Final || !got.Expanded || got.Adjusted {
			t.Fatalf("final call source = %#v", got)
		}
		if got := sources[2]; got.TargetIndex != 2 || got.ResultIndex != 1 || !got.Final || !got.Expanded || got.Kind != factflow.ValueSourceCall {
			t.Fatalf("expanded target source = %#v", got)
		}
		if got := sources[3]; got.TargetIndex != 3 || got.ResultIndex != 2 || !got.Final || !got.Expanded || got.Kind != factflow.ValueSourceCall {
			t.Fatalf("later expanded target source = %#v", got)
		}
	})

	t.Run("non-expanding final fills nil", func(t *testing.T) {
		sources := AssignmentSources([]ast.Expr{ident("x"), ident("y")}, 3, nil)
		if len(sources) != 3 {
			t.Fatalf("len(sources) = %d, want 3", len(sources))
		}

		if got := sources[2]; got.Kind != factflow.ValueSourceNil || got.Expr != nil || got.ExprIndex != factflow.NoValueSourceIndex || got.TargetIndex != 2 || got.ResultIndex != factflow.NoValueSourceIndex {
			t.Fatalf("nil fill source = %#v", got)
		}
	})
}

func TestValueListSourcesShape(t *testing.T) {
	exprs := []ast.Expr{call("left"), call("tail")}

	t.Run("return open tail final call expands and keeps open tail", func(t *testing.T) {
		sources := ValueListSources(exprs, true, nil)
		if len(sources) != 2 {
			t.Fatalf("len(sources) = %d, want 2", len(sources))
		}

		if got := sources[1]; got.Kind != factflow.ValueSourceCall || !got.Final || !got.Expanded || !got.OpenTail || got.Adjusted {
			t.Fatalf("return tail source = %#v", got)
		}
	})

	t.Run("closed tail final call expands without open tail", func(t *testing.T) {
		sources := ValueListSources(exprs, false, nil)
		if len(sources) != 2 {
			t.Fatalf("len(sources) = %d, want 2", len(sources))
		}

		if got := sources[1]; got.Kind != factflow.ValueSourceCall || !got.Final || !got.Expanded || got.OpenTail || got.Adjusted {
			t.Fatalf("closed tail source = %#v", got)
		}
	})
}

func TestConditionSourceShape(t *testing.T) {
	source := ConditionSource(wrappedCall("pred"), nil)
	if source.Kind != factflow.ValueSourceCall {
		t.Fatalf("kind = %v, want call", source.Kind)
	}
	if source.ExprIndex != 0 || source.TargetIndex != factflow.NoValueSourceIndex || source.ResultIndex != 0 {
		t.Fatalf("indexes = %#v, want condition source shape", source)
	}
	if !source.Final || !source.Adjusted || source.Expanded || source.OpenTail {
		t.Fatalf("condition flags = %#v, want adjusted multi-value without expansion", source)
	}
}

func TestSourceForExprResolvesTopLevelProducerCallPointThroughWrappers(t *testing.T) {
	inner := call("resolve")
	expr := &ast.CastExpr{
		Expr: &ast.NonNilAssertExpr{
			Expr: inner,
		},
	}
	var gotIndex int
	var gotCall *ast.FuncCallExpr

	source := SourceForExpr(expr, 7, 2, 3, true, false, func(exprIndex int, call *ast.FuncCallExpr) (cfg.Point, bool) {
		gotIndex = exprIndex
		gotCall = call
		return cfg.Point(42), true
	})

	if gotIndex != 7 {
		t.Fatalf("resolver exprIndex = %d, want 7", gotIndex)
	}
	if gotCall != inner {
		t.Fatalf("resolver call = %p, want inner call", gotCall)
	}
	if !source.HasCallPoint || source.CallPoint != cfg.Point(42) {
		t.Fatalf("call point = %#v, want 42", source)
	}
	if source.Expr != expr {
		t.Fatalf("expr = %T %p, want wrapped expr %p", source.Expr, source.Expr, expr)
	}
}
