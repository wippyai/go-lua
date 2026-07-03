package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/castsem"
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
		l.addCastAssertion(input, source, expr.Expr, expr.Type)
	case *ast.NonNilAssertExpr:
		l.addAssertion(input, source, expr.Expr, l.assertionRefinement(assertion.NonNil()))
	}
}

func (l *lowerer) addAssertion(input *factflow.FactsInput, outer sourceprovenance.ASTSource, innerExpr ast.Expr, refinement product.Value) {
	outerSource := l.valueSource(outer)
	if !outerSource.HasExpr || innerExpr == nil {
		return
	}
	inner := outer
	inner.Expr = innerExpr
	if input.ExpressionRefinements == nil {
		input.ExpressionRefinements = make(map[factflow.ExprRef]factflow.ExpressionRefinement)
	}
	input.ExpressionRefinements[outerSource.ExprRef] = factflow.NewExpressionRefinement(l.valueSource(inner), refinement)
	l.addAssertionRefinementsForSource(input, inner)
}

func (l *lowerer) addCastAssertion(input *factflow.FactsInput, outer sourceprovenance.ASTSource, innerExpr ast.Expr, expr ast.TypeExpr) {
	refinement := l.castAssertionRefinement(expr)
	outerSource := l.valueSource(outer)
	if !outerSource.HasExpr || innerExpr == nil {
		return
	}
	inner := outer
	inner.Expr = innerExpr
	if input.ExpressionRefinements == nil {
		input.ExpressionRefinements = make(map[factflow.ExprRef]factflow.ExpressionRefinement)
	}
	innerSource := l.valueSource(inner)
	if product.Get(l.registry, refinement, assertion.Key).Has(assertion.AnyClaim) {
		input.ExpressionRefinements[outerSource.ExprRef] = factflow.NewExpressionRefinement(innerSource, refinement)
	} else {
		input.ExpressionRefinements[outerSource.ExprRef] = factflow.NewExpressionDeclaredContract(innerSource, refinement)
	}
	l.addAssertionRefinementsForSource(input, inner)
}

func (l *lowerer) assertionRefinement(value assertion.Value) product.Value {
	if value.Has(assertion.AnyClaim) {
		return product.Set(l.registry, l.valueFromType(typ.Any), assertion.Key, value)
	}
	return product.Set(l.registry, product.Top(), assertion.Key, value)
}

func (l *lowerer) castAssertionRefinement(expr ast.TypeExpr) product.Value {
	value := castAssertionValue(expr)
	if value.Has(assertion.AnyClaim) {
		return l.assertionRefinement(value)
	}
	t, ok := l.resolveType(expr)
	if !ok || t == nil {
		return l.assertionRefinement(value)
	}
	if typ.IsAny(t) || typ.IsUnknown(t) {
		return l.assertionRefinement(assertion.Any())
	}
	return product.Set(l.registry, l.valueFromTypeWithWitness(t), assertion.Key, value)
}

func castAssertionValue(typ ast.TypeExpr) assertion.Value {
	if primitive, ok := typ.(*ast.PrimitiveTypeExpr); ok {
		if castsem.IsTopLikeTarget(primitive.Name) {
			return assertion.Any()
		}
	}
	return assertion.Of(assertion.TypeClaim, assertion.RuntimeClaim)
}
