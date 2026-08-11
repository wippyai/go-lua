package transfer

import (
	internal "github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/lattice"
)

// Lattice exposes the one sealed transfer-membership algebra. A foreign
// value is a broken composition boundary, never a semantic Bottom value.
func (algebra Algebra) Lattice() (lattice.Lattice[Value], bool) {
	if !algebra.valid() {
		return lattice.Lattice[Value]{}, false
	}
	bottom, bottomOK := algebra.Default()
	top, topOK := algebra.Present()
	if !bottomOK || !topOK {
		return lattice.Lattice[Value]{}, false
	}
	return lattice.Lattice[Value]{
		Bottom:   func() Value { return bottom },
		Top:      func() Value { return top },
		Equal:    algebra.Equal,
		LessOrEq: algebra.LessOrEq,
		Join: func(left, right Value) Value {
			value, ok := algebra.Join(left, right)
			if !ok {
				panic("transfer: foreign arm algebra")
			}
			return value
		},
		Widen: func(previous, next Value) Value {
			value, ok := algebra.Widen(previous, next)
			if !ok {
				panic("transfer: foreign arm algebra")
			}
			return value
		},
	}, true
}

// Admits checks a Value against one exact Link-arm/isolation coordinate.
// Membership is uniformly defined over every admitted arm, but the Key
// validation prevents a caller from manufacturing a transfer coordinate.
func (algebra Algebra) Admits(key Key, value Value) bool {
	return algebra.ownsKey(key) && algebra.owns(value)
}

// Fingerprint is the deterministic hot fingerprint of a transfer membership
// fact. Persistent family identity remains Algebra.ContentID.
func (algebra Algebra) Fingerprint(value Value) uint64 {
	if !algebra.owns(value) {
		return 0
	}
	if value.present {
		return internal.MixHash(0x7472616e73666572, 1) // "transfer"
	}
	return internal.MixHash(0x7472616e73666572, 0)
}
