package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

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

func (l *lowerer) assertionRefinement(value assertion.Value) product.Value {
	if value.Has(assertion.AnyClaim) {
		return product.Set(l.registry, l.valueFromType(typ.Any), assertion.Key, value)
	}
	return product.Set(l.registry, product.Top(), assertion.Key, value)
}
