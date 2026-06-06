package flow

import (
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/domain/value/product"
)

func ApplyCaptureEffectsToCellStore(out *PointState, effects CaptureEffects) bool {
	if out == nil || CaptureEffectsDomain.Equal(effects, CaptureEffectsDomain.Bottom()) {
		return false
	}
	before := out.Cells
	out.Cells = effects.Apply(out.Cells)
	return !CaptureCellsDomain.Equal(before, out.Cells)
}

func RecordCaptureEffects(out *PointState, effects CaptureEffects) bool {
	if out == nil || CaptureEffectsDomain.Equal(effects, CaptureEffectsDomain.Bottom()) {
		return false
	}
	before := out.CellEffects
	out.CellEffects = out.CellEffects.Then(effects)
	return !CaptureEffectsDomain.Equal(before, out.CellEffects)
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

func RecordPrototypeSelf(out *PointState, proto cfg.SymbolID, value product.AbstractValue) bool {
	if out == nil || proto == 0 || value.IsZero() {
		return false
	}
	before := out.PrototypeSelf
	out.PrototypeSelf = out.PrototypeSelf.JoinValue(proto, value)
	return !PrototypeSelfDomain.Equal(before, out.PrototypeSelf)
}

func BindPrototypeInstance(out *PointState, sym cfg.SymbolID, proto cfg.SymbolID) bool {
	if out == nil || sym == 0 || proto == 0 {
		return false
	}
	before := out.PrototypeInstances
	out.PrototypeInstances = out.PrototypeInstances.WithPrototype(sym, proto)
	return !PrototypeInstancesDomain.Equal(before, out.PrototypeInstances)
}

func ClearPrototypeInstance(out *PointState, sym cfg.SymbolID) bool {
	if out == nil || sym == 0 {
		return false
	}
	before := out.PrototypeInstances
	out.PrototypeInstances = out.PrototypeInstances.With(sym, nil)
	return !PrototypeInstancesDomain.Equal(before, out.PrototypeInstances)
}
