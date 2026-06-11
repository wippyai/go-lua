package semantics

import (
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
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
	if targetCount <= 0 {
		return nil
	}
	sources := make([]sourceprovenance.ASTSource, targetCount)
	for targetIndex := range sources {
		sources[targetIndex] = assignmentValueSource(exprs, targetIndex, callPoints)
	}
	return sources
}

func assignmentValueSource(exprs []ast.Expr, targetIndex int, callPoints map[int]cfg.Point) sourceprovenance.ASTSource {
	if len(exprs) == 0 {
		return nilFillSource(targetIndex)
	}

	finalExprIndex := len(exprs) - 1
	if targetIndex < finalExprIndex {
		return valueSourceForExpr(exprs[targetIndex], targetIndex, targetIndex, 0, false, false, callPoints)
	}

	finalExpr := exprs[finalExprIndex]
	finalExpands := canExpandFinal(finalExpr)
	if targetIndex == finalExprIndex {
		return valueSourceForExpr(finalExpr, finalExprIndex, targetIndex, 0, true, false, callPoints)
	}
	if finalExpands {
		return valueSourceForExpr(finalExpr, finalExprIndex, targetIndex, targetIndex-finalExprIndex, true, false, callPoints)
	}
	return nilFillSource(targetIndex)
}

func returnValueSources(exprs []ast.Expr, callPoints map[int]cfg.Point) []sourceprovenance.ASTSource {
	return valueListSources(exprs, callPoints, true)
}

func iteratorValueSources(exprs []ast.Expr, callPoints map[int]cfg.Point) []sourceprovenance.ASTSource {
	return valueListSources(exprs, callPoints, false)
}

func valueListSources(exprs []ast.Expr, callPoints map[int]cfg.Point, openTailFinal bool) []sourceprovenance.ASTSource {
	if len(exprs) == 0 {
		return nil
	}
	sources := make([]sourceprovenance.ASTSource, len(exprs))
	for i, expr := range exprs {
		final := i == len(exprs)-1
		openTail := openTailFinal && final && canExpandFinal(expr)
		sources[i] = valueSourceForExpr(expr, i, i, 0, final, openTail, callPoints)
	}
	return sources
}

func conditionValueSource(expr ast.Expr, callPoints map[int]cfg.Point) sourceprovenance.ASTSource {
	source := sourceprovenance.ASTSource{
		Kind:        valueSourceKind(expr),
		Expr:        expr,
		ExprIndex:   0,
		TargetIndex: factflow.NoValueSourceIndex,
		ResultIndex: 0,
		Final:       true,
		Adjusted:    canProduceMultipleValues(expr),
	}
	if source.Kind == factflow.ValueSourceCall {
		if point, ok := callPoints[0]; ok {
			source.CallPoint = point
			source.HasCallPoint = point != 0
		}
	}
	return source
}

func valueSourceForExpr(expr ast.Expr, exprIndex, targetIndex, resultIndex int, final bool, openTail bool, callPoints map[int]cfg.Point) sourceprovenance.ASTSource {
	expanded := final && canExpandFinal(expr)
	adjusted := canProduceMultipleValues(expr) && !expanded
	source := sourceprovenance.ASTSource{
		Kind:        valueSourceKind(expr),
		Expr:        expr,
		ExprIndex:   exprIndex,
		TargetIndex: targetIndex,
		ResultIndex: resultIndex,
		Final:       final,
		Expanded:    expanded,
		Adjusted:    adjusted,
		OpenTail:    openTail && expanded,
	}
	if source.Kind == factflow.ValueSourceCall {
		if point, ok := callPoints[exprIndex]; ok {
			source.CallPoint = point
			source.HasCallPoint = point != 0
		}
	}
	return source
}

func nilFillSource(targetIndex int) sourceprovenance.ASTSource {
	return sourceprovenance.ASTSource{
		Kind:        factflow.ValueSourceNil,
		ExprIndex:   factflow.NoValueSourceIndex,
		TargetIndex: targetIndex,
		ResultIndex: factflow.NoValueSourceIndex,
	}
}

func valueSourceKind(expr ast.Expr) factflow.ValueSourceKind {
	switch valueexpr.TopLevelProducer(expr).Kind {
	case valueexpr.ProducerCall:
		return factflow.ValueSourceCall
	case valueexpr.ProducerVararg:
		return factflow.ValueSourceVararg
	default:
		return factflow.ValueSourceExpression
	}
}

func canExpandFinal(expr ast.Expr) bool {
	return canProduceMultipleValues(expr) && !adjustRet(expr)
}

func canProduceMultipleValues(expr ast.Expr) bool {
	return valueexpr.CanProduceMultipleValues(expr)
}

func adjustRet(expr ast.Expr) bool {
	return valueexpr.AdjustRet(expr)
}
