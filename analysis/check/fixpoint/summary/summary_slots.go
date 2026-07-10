package summary

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

func returnAt(reg *axis.Registry, s Summary, i int) product.Value {
	if i < len(s.Returns) {
		return s.Returns[i]
	}
	return product.Bottom(reg)
}

func normalReturnParamAt(reg *axis.Registry, s Summary, i int) product.Value {
	if i < len(s.NormalReturnParams) {
		return s.NormalReturnParams[i]
	}
	return product.Bottom(reg)
}

func paramObligationAt(reg *axis.Registry, s Summary, i int) product.Value {
	if i < len(s.ParamObligations) {
		return s.ParamObligations[i]
	}
	return product.Top()
}

// UsefulParamObligation reports whether a parameter obligation carries
// caller-checkable pre-call information.
func UsefulParamObligation(reg *axis.Registry, value product.Value) bool {
	return usefulNonExtremalValue(reg, value)
}

// UsefulNormalReturnParam reports whether a normal-return parameter lane carries
// caller-applicable information.
func UsefulNormalReturnParam(reg *axis.Registry, value product.Value) bool {
	return usefulNonExtremalValue(reg, value)
}

func usefulNonExtremalValue(reg *axis.Registry, value product.Value) bool {
	if reg == nil {
		return false
	}
	return !product.Equal(reg, value, product.Bottom(reg)) && !product.Equal(reg, value, product.Top())
}

func normalReturnParamConditionAt(reg *axis.Registry, s Summary, i int) ParamCondition {
	if i < len(s.NormalReturnParamConditions) {
		return s.NormalReturnParamConditions[i]
	}
	if i < normalReturnParamCount(reg, s) {
		return ParamConditionTop
	}
	return ParamConditionBottom
}

func normalReturnParamCount(reg *axis.Registry, s Summary) int {
	paramCount := len(s.NormalReturnParams)
	bottom := product.Bottom(reg)
	for paramCount > 0 && product.Equal(reg, s.NormalReturnParams[paramCount-1], bottom) {
		paramCount--
	}
	conditionCount := len(s.NormalReturnParamConditions)
	for conditionCount > 0 && !s.NormalReturnParamConditions[conditionCount-1].IsUseful() {
		conditionCount--
	}
	return max(paramCount, conditionCount)
}
