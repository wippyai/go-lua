package relation

import (
	effectfactor "github.com/wippyai/go-lua/domain/effect/factor"
)

// EffectLattice is domain/effect/factor's ascent authority, spelled as methods
// on a sealed algebra.
type EffectLattice struct {
	algebra *effectfactor.Algebra
}

// NewEffectLattice adopts one sealed effect algebra.
func NewEffectLattice(algebra *effectfactor.Algebra) (EffectLattice, bool) {
	if algebra == nil || !algebra.Valid() {
		return EffectLattice{}, false
	}
	return EffectLattice{algebra: algebra}, true
}

func (witness EffectLattice) Join(left, right effectfactor.Value) (effectfactor.Value, bool) {
	return witness.algebra.Join(left, right)
}

func (witness EffectLattice) Widen(previous, next effectfactor.Value) (effectfactor.Value, bool) {
	return witness.algebra.Widen(previous, next)
}

func (witness EffectLattice) LessOrEq(left, right effectfactor.Value) bool {
	return witness.algebra.LessOrEq(left, right)
}
