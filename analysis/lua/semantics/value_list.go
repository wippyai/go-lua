package semantics

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/lua/valueexpr"
	"github.com/wippyai/go-lua/compiler/ast"
)

type indexedCall struct {
	index int
	call  *ast.FuncCallExpr
}

func callPointsByExprIndex(calls []indexedCall, points []cfg.Point) map[int]cfg.Point {
	if len(calls) == 0 {
		return nil
	}
	out := make(map[int]cfg.Point, len(calls))
	for i, call := range calls {
		if i >= len(points) {
			break
		}
		out[call.index] = points[i]
	}
	return out
}

func topLevelValueListCalls(exprs []ast.Expr) []indexedCall {
	var calls []indexedCall
	for i, expr := range exprs {
		call, ok := valueexpr.Call(expr)
		if !ok {
			continue
		}
		calls = append(calls, indexedCall{index: i, call: call})
	}
	return calls
}

func assignmentValueSources(exprs []ast.Expr, targetCount int, callPoints map[int]cfg.Point) []sourceprovenance.ASTSource {
	return sourceprovenance.AssignmentSources(exprs, targetCount, callPointResolverByExprIndex(callPoints))
}

func assignmentValueSource(exprs []ast.Expr, targetIndex int, callPoints map[int]cfg.Point) sourceprovenance.ASTSource {
	return sourceprovenance.AssignmentSources(exprs, targetIndex+1, callPointResolverByExprIndex(callPoints))[targetIndex]
}

func returnValueSources(exprs []ast.Expr, callPoints map[int]cfg.Point) []sourceprovenance.ASTSource {
	return sourceprovenance.ValueListSources(exprs, true, callPointResolverByExprIndex(callPoints))
}

func iteratorValueSources(exprs []ast.Expr, callPoints map[int]cfg.Point) []sourceprovenance.ASTSource {
	return sourceprovenance.ValueListSources(exprs, false, callPointResolverByExprIndex(callPoints))
}

func argumentValueSources(exprs []ast.Expr) []sourceprovenance.ASTSource {
	return sourceprovenance.ValueListSources(exprs, false, nil)
}

func conditionValueSource(expr ast.Expr, callPoints map[int]cfg.Point) sourceprovenance.ASTSource {
	return sourceprovenance.ConditionSource(expr, callPointResolverByExprIndex(callPoints))
}

func callPointResolverByExprIndex(callPoints map[int]cfg.Point) sourceprovenance.CallPointResolver {
	if len(callPoints) == 0 {
		return nil
	}
	return func(exprIndex int, call *ast.FuncCallExpr) (cfg.Point, bool) {
		point, ok := callPoints[exprIndex]
		return point, ok
	}
}
