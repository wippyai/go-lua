package sourceprovenance

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
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
	adjusted = CanProduceMultipleValues(expr) && !expanded
	shapedOpenTail = openTail && expanded
	return expanded, adjusted, shapedOpenTail
}

func ConditionSource(expr ast.Expr, resolver CallPointResolver) ASTSource {
	shape := mustSourceShape(NewSourceShape(true, false, CanProduceMultipleValues(expr), false))
	return sourceForExprShape(expr, 0, NoSourceIndex, 0, shape, resolver)
}

func SourceForExpr(expr ast.Expr, exprIndex, targetIndex, resultIndex int, final, openTail bool, resolver CallPointResolver) ASTSource {
	expanded, adjusted, shapedOpenTail := ValueShape(expr, final, true, openTail)
	shape := mustSourceShape(NewSourceShape(final, expanded, adjusted, shapedOpenTail))
	return sourceForExprShape(expr, exprIndex, targetIndex, resultIndex, shape, resolver)
}

func assignmentSource(exprs []ast.Expr, targetIndex int, resolver CallPointResolver) ASTSource {
	if len(exprs) == 0 {
		return NewNilSource(targetIndex)
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
	return NewNilSource(targetIndex)
}

func sourceForExprShape(expr ast.Expr, exprIndex, targetIndex, resultIndex int, shape SourceShape, resolver CallPointResolver) ASTSource {
	if missingAssertionInner(expr) {
		return NewUnknownSource(targetIndex)
	}
	switch valueSourceKind(expr) {
	case SourceCall:
		point, ok := resolveCallPoint(resolver, exprIndex, expr)
		if !ok || point == 0 {
			return NewUnknownSource(targetIndex)
		}
		return mustASTSource(NewCallSource(expr, exprIndex, targetIndex, resultIndex, point, shape))
	case SourceVararg:
		return mustASTSource(NewVarargSource(expr, exprIndex, targetIndex, resultIndex, shape))
	default:
		return mustASTSource(NewExpressionSource(expr, exprIndex, targetIndex, resultIndex, shape))
	}
}

func mustSourceShape(shape SourceShape, ok bool) SourceShape {
	if !ok {
		panic("sourceprovenance: invalid source shape")
	}
	return shape
}

func mustASTSource(source ASTSource, ok bool) ASTSource {
	if !ok {
		panic("sourceprovenance: invalid AST source")
	}
	return source
}

func resolveCallPoint(resolver CallPointResolver, exprIndex int, expr ast.Expr) (cfg.Point, bool) {
	if resolver == nil {
		return 0, false
	}
	if call, ok := Call(expr); ok {
		return resolver(exprIndex, call)
	}
	return 0, false
}

func valueSourceKind(expr ast.Expr) SourceKind {
	switch TopLevelProducer(expr).Kind {
	case ProducerCall:
		return SourceCall
	case ProducerVararg:
		return SourceVararg
	default:
		return SourceExpression
	}
}

func canExpandFinal(expr ast.Expr) bool {
	return CanProduceMultipleValues(expr) && !AdjustRet(expr)
}
