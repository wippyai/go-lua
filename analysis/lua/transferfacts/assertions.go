package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (l *lowerer) addAssertionOverlaysForSource(input *factflow.FactsInput, source sourceprovenance.ASTSource) {
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
	outerRef, hasOuter := l.exprRef(outer.Expr)
	if !hasOuter || innerExpr == nil {
		return
	}
	inner := outer
	inner.Expr = innerExpr
	if input.ValueOverlays == nil {
		input.ValueOverlays = make(map[factflow.ExprRef]factflow.ValueOverlay)
	}
	input.ValueOverlays[outerRef] = factflow.NewValueOverlay(l.valueSource(inner), l.assertionOverlay(value))
	l.addAssertionOverlaysForSource(input, inner)
}

func (l *lowerer) assertionOverlay(value assertion.Value) product.Value {
	return product.Set(l.registry, product.Top(), assertion.Key, value)
}

func castAssertionValue(typ ast.TypeExpr) assertion.Value {
	if primitive, ok := typ.(*ast.PrimitiveTypeExpr); ok && primitive.Name == "any" {
		return assertion.Any()
	}
	return assertion.Type()
}
