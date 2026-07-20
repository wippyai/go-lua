package factapply

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// NewResolvedCallResultTransaction seals one already-resolved result value as
// an immutable N0 transaction. It is the callback-free handoff for dynamic
// producers whose argument is evaluated by a compiled ValueTerm at execution.
func NewResolvedCallResultTransaction(reg *axis.Registry, point cfg.Point, resultIndex int, value product.Value) (CallResultTransaction, bool) {
	if reg == nil || resultIndex < 0 || !product.RetentionSafe(reg, value) {
		return CallResultTransaction{}, false
	}
	return CallResultTransaction{
		point: point,
		steps: []CallResultStep{{
			kind:  CallResultStepValue,
			value: factflow.NewCallResultValue(resultIndex, value),
		}},
	}, true
}
