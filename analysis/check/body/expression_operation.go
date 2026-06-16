package body

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	luasourcevalue "github.com/wippyai/go-lua/analysis/lua/sourcevalue"
)

func expressionOperationEvaluator(reg *axis.Registry, typeValues *typevalue.Cache) sourcevalue.ExpressionOperationEvaluator {
	return func(op factflow.ExpressionOperation, left product.Value, right product.Value) (product.Value, bool) {
		return luasourcevalue.ExpressionOperationValue(reg, typeValues, op, left, right)
	}
}
