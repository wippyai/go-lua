package sourceprovenance

import (
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/valueexpr"
	"github.com/wippyai/go-lua/compiler/ast"
)

// CallPointResolver resolves a call expression to its CFG point.
type CallPointResolver func(exprIndex int, call *ast.FuncCallExpr) (cfg.Point, bool)

func AssignmentSources(exprs []ast.Expr, targetCount int, resolver CallPointResolver) []ASTSource {
	if targetCount <= 0 {
		return nil
	}
	sources := make([]ASTSource, targetCount)
	for targetIndex := range sources {
		sources[targetIndex] = assignmentSource(exprs, targetIndex, resolver)
	}
	return sources
}

func ValueListSources(exprs []ast.Expr, openTailFinal bool, resolver CallPointResolver) []ASTSource {
	if len(exprs) == 0 {
		return nil
	}
	sources := make([]ASTSource, len(exprs))
	for i, expr := range exprs {
		final := i == len(exprs)-1
		openTail := openTailFinal && final && canExpandFinal(expr)
		sources[i] = SourceForExpr(expr, i, i, 0, final, openTail, resolver)
	}
	return sources
}

func ValueShape(expr ast.Expr, final, allowExpansion, openTail bool) (expanded, adjusted, shapedOpenTail bool) {
	expanded = final && allowExpansion && canExpandFinal(expr)
	adjusted = valueexpr.CanProduceMultipleValues(expr) && !expanded
	shapedOpenTail = openTail && expanded
	return expanded, adjusted, shapedOpenTail
}

func ConditionSource(expr ast.Expr, resolver CallPointResolver) ASTSource {
	source := ASTSource{
		Kind:        valueSourceKind(expr),
		Expr:        expr,
		ExprIndex:   0,
		TargetIndex: factflow.NoValueSourceIndex,
		ResultIndex: 0,
		Final:       true,
		Adjusted:    valueexpr.CanProduceMultipleValues(expr),
	}
	if source.Kind == factflow.ValueSourceCall {
		if point, ok := resolveCallPoint(resolver, 0, expr); ok {
			source.CallPoint = point
			source.HasCallPoint = point != 0
		}
	}
	return source
}

func SourceForExpr(expr ast.Expr, exprIndex, targetIndex, resultIndex int, final, openTail bool, resolver CallPointResolver) ASTSource {
	expanded, adjusted, shapedOpenTail := ValueShape(expr, final, true, openTail)
	source := ASTSource{
		Kind:        valueSourceKind(expr),
		Expr:        expr,
		ExprIndex:   exprIndex,
		TargetIndex: targetIndex,
		ResultIndex: resultIndex,
		Final:       final,
		Expanded:    expanded,
		Adjusted:    adjusted,
		OpenTail:    shapedOpenTail,
	}
	if source.Kind == factflow.ValueSourceCall {
		if point, ok := resolveCallPoint(resolver, exprIndex, expr); ok {
			source.CallPoint = point
			source.HasCallPoint = point != 0
		}
	}
	return source
}

func assignmentSource(exprs []ast.Expr, targetIndex int, resolver CallPointResolver) ASTSource {
	if len(exprs) == 0 {
		return nilFillSource(targetIndex)
	}

	finalExprIndex := len(exprs) - 1
	if targetIndex < finalExprIndex {
		return SourceForExpr(exprs[targetIndex], targetIndex, targetIndex, 0, false, false, resolver)
	}

	finalExpr := exprs[finalExprIndex]
	finalExpands := canExpandFinal(finalExpr)
	if targetIndex == finalExprIndex {
		return SourceForExpr(finalExpr, finalExprIndex, targetIndex, 0, true, false, resolver)
	}
	if finalExpands {
		return SourceForExpr(finalExpr, finalExprIndex, targetIndex, targetIndex-finalExprIndex, true, false, resolver)
	}
	return nilFillSource(targetIndex)
}

func nilFillSource(targetIndex int) ASTSource {
	return ASTSource{
		Kind:        factflow.ValueSourceNil,
		ExprIndex:   factflow.NoValueSourceIndex,
		TargetIndex: targetIndex,
		ResultIndex: factflow.NoValueSourceIndex,
	}
}

func resolveCallPoint(resolver CallPointResolver, exprIndex int, expr ast.Expr) (cfg.Point, bool) {
	if resolver == nil {
		return 0, false
	}
	if call, ok := valueexpr.Call(expr); ok {
		return resolver(exprIndex, call)
	}
	return 0, false
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
	return valueexpr.CanProduceMultipleValues(expr) && !valueexpr.AdjustRet(expr)
}
