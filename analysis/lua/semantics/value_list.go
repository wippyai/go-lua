package semantics

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/callorder"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
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

func valueListCalls(exprs []ast.Expr, bindings *bind.Result) ([]indexedCall, bool) {
	ordered, ok := callorder.ValueList(exprs, valueCallOrderOptions(bindings))
	if !ok {
		return nil, false
	}
	return indexedCallsFromOccurrences(ordered), true
}

func exprCalls(expr ast.Expr, bindings *bind.Result) ([]indexedCall, bool) {
	ordered, ok := callorder.Expr(expr, valueCallOrderOptions(bindings))
	if !ok {
		return nil, false
	}
	return indexedCallsFromOccurrences(ordered), true
}

func indexedCallsFromOccurrences(ordered []callorder.Occurrence) []indexedCall {
	calls := make([]indexedCall, len(ordered))
	for i, call := range ordered {
		calls[i] = indexedCall{index: call.ExprIndex, call: call.Call}
	}
	return calls
}

func valueCallOrderOptions(bindings *bind.Result) callorder.Options {
	options := callorder.LuaOptions(bindings)
	options.AllowShortCircuitCalls = true
	return options
}

func callPointsByCall(calls []indexedCall, points []cfg.Point) map[*ast.FuncCallExpr]cfg.Point {
	if len(calls) == 0 {
		return nil
	}
	out := make(map[*ast.FuncCallExpr]cfg.Point, len(calls))
	for i, call := range calls {
		if i >= len(points) {
			break
		}
		out[call.call] = points[i]
	}
	return out
}

func assignmentValueSources(exprs []ast.Expr, targetCount int, resolver sourceprovenance.CallPointResolver) []sourceprovenance.ASTSource {
	return sourceprovenance.AssignmentSources(exprs, targetCount, resolver)
}

func assignmentValueSource(exprs []ast.Expr, targetIndex int, resolver sourceprovenance.CallPointResolver) sourceprovenance.ASTSource {
	return sourceprovenance.AssignmentSources(exprs, targetIndex+1, resolver)[targetIndex]
}

func returnValueSources(exprs []ast.Expr, resolver sourceprovenance.CallPointResolver) []sourceprovenance.ASTSource {
	return sourceprovenance.ValueListSources(exprs, true, resolver)
}

func argumentValueSources(exprs []ast.Expr, resolver sourceprovenance.CallPointResolver) []sourceprovenance.ASTSource {
	return sourceprovenance.ValueListSources(exprs, false, resolver)
}

func conditionValueSource(expr ast.Expr, resolver sourceprovenance.CallPointResolver) sourceprovenance.ASTSource {
	return sourceprovenance.ConditionSource(expr, resolver)
}

func callPointResolver(calls []indexedCall, points []cfg.Point) sourceprovenance.CallPointResolver {
	callPoints := callPointsByCall(calls, points)
	exprPoints := callPointsByExprIndex(calls, points)
	if len(callPoints) == 0 && len(exprPoints) == 0 {
		return nil
	}
	return func(exprIndex int, call *ast.FuncCallExpr) (cfg.Point, bool) {
		if point, ok := callPoints[call]; ok {
			return point, true
		}
		point, ok := exprPoints[exprIndex]
		return point, ok
	}
}

func topLevelValueListCall(exprs []ast.Expr, call indexedCall) bool {
	if call.index < 0 || call.index >= len(exprs) {
		return false
	}
	top, ok := sourceprovenance.Call(exprs[call.index])
	return ok && top == call.call
}
