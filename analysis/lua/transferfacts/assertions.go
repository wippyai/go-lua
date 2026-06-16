package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (l *lowerer) addAssertionRefinementsForSource(input *factflow.FactsInput, source sourceprovenance.ASTSource) {
	if input == nil || source.Expr == nil {
		return
	}
	switch expr := source.Expr.(type) {
	case *ast.CastExpr:
		l.addAssertion(input, source, expr.Expr, castAssertionValue(expr.Type))
	case *ast.NonNilAssertExpr:
		l.addAssertion(input, source, expr.Expr, assertion.NonNil())
	}
}

func (l *lowerer) addAssertion(input *factflow.FactsInput, outer sourceprovenance.ASTSource, innerExpr ast.Expr, value assertion.Value) {
	outerSource := l.valueSource(outer)
	if !outerSource.HasExpr || innerExpr == nil {
		return
	}
	inner := outer
	inner.Expr = innerExpr
	if input.ExpressionRefinements == nil {
		input.ExpressionRefinements = make(map[factflow.ExprRef]factflow.ExpressionRefinement)
	}
	input.ExpressionRefinements[outerSource.ExprRef] = factflow.NewExpressionRefinement(l.valueSource(inner), l.assertionRefinement(value))
	l.addAssertionRefinementsForSource(input, inner)
}

func (l *lowerer) assertionRefinement(value assertion.Value) product.Value {
	if value.Has(assertion.AnyClaim) {
		return product.Set(l.registry, l.valueFromType(typ.Any), assertion.Key, value)
	}
	return product.Set(l.registry, product.Top(), assertion.Key, value)
}

func castAssertionValue(typ ast.TypeExpr) assertion.Value {
	if primitive, ok := typ.(*ast.PrimitiveTypeExpr); ok && primitive.Name == "any" {
		return assertion.Any()
	}
	return assertion.Type()
}
