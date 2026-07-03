package body

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	luasourcevalue "github.com/wippyai/go-lua/analysis/lua/sourcevalue"
)

func objectLiteralViewEvaluator(reg *axis.Registry, typeValues *typevalue.Cache) sourcevalue.ObjectLiteralViewEvaluator {
	return func(lit factflow.ObjectLiteralView, resolver factflow.ValueSourceResolver) (product.Value, bool) {
		return objectLiteralValueFromView(reg, typeValues, lit, resolver)
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
	resolver factflow.ValueSourceResolver,
) (product.Value, bool) {
	return luasourcevalue.ObjectLiteralValueFromViewCached(reg, typeValues, lit, resolver)
}
