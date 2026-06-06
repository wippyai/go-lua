package transfer

import (
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
)

type callerEffectReducer struct{}

var callerEffects = callerEffectReducer{}

// Caller effects are path-local summaries of writes made by callees. Transfer
// decides which effects are eligible for the current frame; this reducer owns
// composition into the cell store and caller-visible effect carriers.
func (callerEffectReducer) applyCellStore(out *flow.PointState, effects flow.CaptureEffects) bool {
	if out == nil || flow.CaptureEffectsDomain.Equal(effects, flow.CaptureEffectsDomain.Bottom()) {
		return false
	}
	before := out.Cells
	out.Cells = effects.Apply(out.Cells)
	return !flow.CaptureCellsDomain.Equal(before, out.Cells)
}

func (callerEffectReducer) recordCells(out *flow.PointState, effects flow.CaptureEffects) bool {
	if out == nil || flow.CaptureEffectsDomain.Equal(effects, flow.CaptureEffectsDomain.Bottom()) {
		return false
	}
	before := out.CellEffects
	out.CellEffects = out.CellEffects.Then(effects)
	return !flow.CaptureEffectsDomain.Equal(before, out.CellEffects)
}

func (callerEffectReducer) recordReceiverWrite(
	out *flow.PointState,
	slot int,
	value product.AbstractValue,
	mutations ...flow.ReceiverMutation,
) bool {
	if out == nil || slot < 0 || value.IsZero() {
		return false
	}
	before := out.ReceiverEffects
	out.ReceiverEffects = out.ReceiverEffects.Then(flow.ReceiverMustWriteWithMutations(slot, value, mutations))
	return !flow.ReceiverEffectsDomain.Equal(before, out.ReceiverEffects)
}

func (callerEffectReducer) recordReceiverMutation(
	out *flow.PointState,
	slot int,
	mutations ...flow.ReceiverMutation,
) bool {
	if out == nil || slot < 0 || len(mutations) == 0 {
		return false
	}
	before := out.ReceiverEffects
	out.ReceiverEffects = out.ReceiverEffects.Then(flow.ReceiverMutations(slot, mutations))
	return !flow.ReceiverEffectsDomain.Equal(before, out.ReceiverEffects)
}
