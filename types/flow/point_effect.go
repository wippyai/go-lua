package flow

import (
	"github.com/wippyai/go-lua/types/domain/value/product"
)

func ApplyCaptureEffectsToCellStore(out *PointState, effects CaptureEffects) bool {
	return ApplyCellEffectsToCaptureCells(out, effects)
}

func RecordCaptureEffects(out *PointState, effects CaptureEffects) bool {
	return RecordCellEffects(out, effects)
}

func RecordReceiverWrite(out *PointState, slot int, value product.AbstractValue, mutations ...ReceiverMutation) bool {
	if out == nil || slot < 0 || value.IsZero() {
		return false
	}
	before := out.ReceiverEffects
	out.ReceiverEffects = out.ReceiverEffects.Then(ReceiverMustWriteWithMutations(slot, value, mutations))
	return !ReceiverEffectsDomain.Equal(before, out.ReceiverEffects)
}

func RecordReceiverMutation(out *PointState, slot int, mutations ...ReceiverMutation) bool {
	if out == nil || slot < 0 || len(mutations) == 0 {
		return false
	}
	before := out.ReceiverEffects
	out.ReceiverEffects = out.ReceiverEffects.Then(ReceiverMutations(slot, mutations))
	return !ReceiverEffectsDomain.Equal(before, out.ReceiverEffects)
}
