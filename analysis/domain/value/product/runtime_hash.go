package product

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	internal "github.com/wippyai/go-lua/analysis/internal/hash"
)

func (rt *registryRuntime) stableHash(shape Shape, p presence.Value, slots []slot) uint64 {
	h := internal.FnvString("value.product")
	h = internal.MixHash(h, uint64(shape)+1)
	h = internal.MixHash(h, presence.Value.Hash(p))
	for _, slot := range slots {
		info := rt.axisOrdinal(slot.ordinal)
		h = internal.MixHash(h, info.keyHash)
		h = internal.MixHash(h, info.spec.HashAny(slot.value))
	}
	return h
}
