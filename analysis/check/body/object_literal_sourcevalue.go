package body

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/escape"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	luasourcevalue "github.com/wippyai/go-lua/analysis/lua/sourcevalue"
)

func objectLiteralViewEvaluator(reg *axis.Registry, typeValues *typevalue.Cache) sourcevalue.ObjectLiteralViewEvaluator {
	return func(lit factflow.ObjectLiteralView, resolve func(factflow.ValueSource) (product.Value, bool)) (product.Value, bool) {
		return objectLiteralValueFromView(reg, typeValues, lit, resolve)
	}
}

func expressionOperationEvaluator(reg *axis.Registry, typeValues *typevalue.Cache) sourcevalue.ExpressionOperationEvaluator {
	return func(op factflow.ExpressionOperation, left product.Value, right product.Value) (product.Value, bool) {
		return luasourcevalue.ExpressionOperationValue(reg, typeValues, op, left, right)
	}
}

func objectLiteralValueFromView(
	reg *axis.Registry,
	typeValues *typevalue.Cache,
	lit factflow.ObjectLiteralView,
	resolve func(factflow.ValueSource) (product.Value, bool),
) (product.Value, bool) {
	t, ok := luasourcevalue.ObjectLiteralTypeViewCached(reg, typeValues, lit, resolve)
	if !ok {
		return product.Value{}, false
	}
	value := typeValues.FromTypeWithWitness(reg, t)
	if id, ok := lit.Identity(); ok {
		value = product.Set(reg, value, identity.Key, identity.Singleton(id))
	}
	return product.Set(reg, value, escape.Key, escape.Fresh()), true
}
