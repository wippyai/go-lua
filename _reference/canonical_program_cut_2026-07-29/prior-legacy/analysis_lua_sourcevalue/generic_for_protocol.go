package sourcevalue

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// GenericForProtocolResult projects one result of the Lua iterator protocol.
// State and control select the invocation, while the iterator function's
// return tuple determines the value type of each loop variable. The first
// result is non-nil on the body edge by the protocol's continuation rule.
func GenericForProtocolResult(reg *axis.Registry, types *typevalue.Cache, variableIndex int, iterator product.Value) (product.Value, bool) {
	iteratorType, ok := typevalue.WitnessOf(reg, iterator)
	if !ok {
		return product.Value{}, false
	}
	function, ok := iteratorType.(*typ.Function)
	if !ok || typ.IsAny(iteratorType) || typ.IsUnknown(iteratorType) || variableIndex < 0 || variableIndex >= len(function.Returns) {
		return product.Value{}, false
	}
	resultType := function.Returns[variableIndex]
	if resultType == nil || typ.IsAny(resultType) || typ.IsUnknown(resultType) {
		return product.Value{}, false
	}
	if variableIndex == 0 {
		if optional, ok := resultType.(*typ.Optional); ok && optional.Inner != nil {
			resultType = optional.Inner
		}
	}
	return types.FromTypeWithWitness(reg, resultType), true
}
