package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/castsem"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (l *lowerer) addAssignmentAssertionRefinements(input *factflow.FactsInput, point cfg.Point, target path.Path, loweredSource factflow.ValueSource, source sourceprovenance.ASTSource) {
	if expressionSourceHasRefinement(input, loweredSource) {
		return
	}
	if l != nil && l.wir != nil {
		return
	}
	l.addAssertionRefinementsForSource(input, source)
}

func (l *lowerer) addCallArgumentAssertionRefinements(input *factflow.FactsInput, point cfg.Point, index int, loweredSource factflow.ValueSource, source sourceprovenance.ASTSource) {
	if expressionSourceHasRefinement(input, loweredSource) {
		return
	}
	if l != nil && l.wir != nil {
		return
	}
	l.addAssertionRefinementsForSource(input, source)
}

func (l *lowerer) addAssertionRefinementsForLoweredSource(input *factflow.FactsInput, loweredSource factflow.ValueSource, source sourceprovenance.ASTSource) {
	if expressionSourceHasRefinement(input, loweredSource) {
		return
	}
	if l != nil && l.wir != nil {
		return
	}
	l.addAssertionRefinementsForSource(input, source)
}

func expressionSourceHasRefinement(input *factflow.FactsInput, source factflow.ValueSource) bool {
	if input == nil || !source.HasExpr || source.ExprRef == 0 {
		return false
	}
	_, ok := input.ExpressionRefinements[source.ExprRef]
	return ok
}

func (l *lowerer) recordExpressionRefinementFromWIRClaim(outerSource, innerSource factflow.ValueSource, inst wir.Instruction) bool {
	if !outerSource.HasExpr || outerSource.ExprRef == 0 {
		return false
	}
	refinement, ok := l.expressionRefinementFromWIRClaim(innerSource, inst)
	if !ok {
		return false
	}
	if l.expressionRefinements == nil {
		l.expressionRefinements = make(map[factflow.ExprRef]factflow.ExpressionRefinement)
	}
	l.expressionRefinements[outerSource.ExprRef] = refinement
	return true
}

func (l *lowerer) expressionRefinementFromWIRClaim(innerSource factflow.ValueSource, inst wir.Instruction) (factflow.ExpressionRefinement, bool) {
	refinement, mode, ok := l.claimRefinementFromWIR(inst)
	if !ok {
		return factflow.ExpressionRefinement{}, false
	}
	switch mode {
	case factflow.ExpressionRefinementRuntimeValidation:
		return factflow.NewExpressionRuntimeValidation(innerSource, refinement), true
	case factflow.ExpressionRefinementDeclaredContract:
		return factflow.NewExpressionDeclaredContract(innerSource, refinement), true
	default:
		return factflow.NewExpressionRefinement(innerSource, refinement), true
	}
}

func claimKindForAssertionSource(expr ast.Expr) (wir.ClaimKind, bool) {
	switch expr.(type) {
	case *ast.CastExpr:
		return wir.ClaimCast, true
	case *ast.NonNilAssertExpr:
		return wir.ClaimAssert, true
	default:
		return wir.ClaimNone, false
	}
}

func (l *lowerer) claimRefinementFromWIR(inst wir.Instruction) (product.Value, factflow.ExpressionRefinementMode, bool) {
	switch inst.Claim {
	case wir.ClaimAssert:
		return l.assertionRefinement(assertion.NonNil()), factflow.ExpressionRefinementMeet, true
	case wir.ClaimCast:
		t := l.wir.Type(inst.Type)
		if t == nil {
			return product.Value{}, factflow.ExpressionRefinementMeet, false
		}
		if typ.IsAny(t) || typ.IsUnknown(t) {
			return l.assertionRefinement(assertion.Any()), factflow.ExpressionRefinementMeet, true
		}
		refinement := product.Set(l.registry, l.valueFromTypeWithWitness(t), assertion.Key, assertion.Of(assertion.TypeClaim, assertion.RuntimeClaim))
		return refinement, factflow.ExpressionRefinementRuntimeValidation, true
	default:
		return product.Value{}, factflow.ExpressionRefinementMeet, false
	}
}

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
	} else if castSyntaxIsRuntimeValidation(outer.Expr) {
		input.ExpressionRefinements[outerSource.ExprRef] = factflow.NewExpressionRuntimeValidation(innerSource, refinement)
	} else {
		input.ExpressionRefinements[outerSource.ExprRef] = factflow.NewExpressionDeclaredContract(innerSource, refinement)
	}
	l.addAssertionRefinementsForSource(input, inner)
}

func castSyntaxIsRuntimeValidation(expr ast.Expr) bool {
	cast, ok := expr.(*ast.CastExpr)
	return ok && cast != nil && (cast.Syntax == ast.CastSyntaxAs || cast.Syntax == ast.CastSyntaxColonColon)
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
