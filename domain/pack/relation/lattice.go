package relation

import (
	"github.com/wippyai/go-lua/analysis/lattice"
	packdomain "github.com/wippyai/go-lua/domain/pack"
)

// PackLattice is domain/pack's ascent authority. The owner spells its lattice
// as a struct of total operators that answer the owner's own top where two
// values have no closer upper bound, so the witness adopts that operator set
// once instead of rebuilding it per invocation.
type PackLattice struct {
	operators lattice.Lattice[packdomain.Value]
}

// NewPackLattice adopts one sealed pack schema's operator set.
func NewPackLattice(schema *packdomain.Schema) (PackLattice, bool) {
	if schema == nil {
		return PackLattice{}, false
	}
	operators := schema.Lattice()
	if operators.Join == nil || operators.Widen == nil || operators.LessOrEq == nil {
		return PackLattice{}, false
	}
	return PackLattice{operators: operators}, true
}

func (witness PackLattice) Join(left, right packdomain.Value) (packdomain.Value, bool) {
	return witness.operators.Join(left, right), true
}

func (witness PackLattice) Widen(previous, next packdomain.Value) (packdomain.Value, bool) {
	return witness.operators.Widen(previous, next), true
}

func (witness PackLattice) LessOrEq(left, right packdomain.Value) bool {
	return witness.operators.LessOrEq(left, right)
}
