package readexpr

import (
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
)

func normalizeDynamicReadIndexForm(config Config, dynamic factflow.DynamicIndexExpression) (factflow.DynamicReadIndexForm, bool) {
	return config.Facts.NormalizeDynamicReadIndexForm(dynamic, func(source factflow.ValueSource) (int64, bool) {
		if config.Registry == nil || source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
			return 0, false
		}
		value, ok := config.Facts.ExpressionValue(source.ExprRef)
		if !ok {
			return 0, false
		}
		return typevalue.IntegerLiteralValue(config.Registry, value)
	})
}
