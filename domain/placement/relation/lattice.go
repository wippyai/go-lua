package relation

import (
	"github.com/wippyai/go-lua/analysis/lattice"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
)

// PlacementLattice is domain/placement's ascent authority for its rule fact.
// The owner spells the product of its placement chain and its control-flow
// proof lattice as a struct of total operators, and states its own widening
// there, so the witness adopts that operator set once rather than rebuilding
// it per invocation and never decides how a placement widens.
type PlacementLattice struct {
	operators lattice.Lattice[placementdomain.Fact]
}

// NewPlacementLattice adopts the sealed placement fact operator set.
func NewPlacementLattice() (PlacementLattice, bool) {
	operators := placementdomain.FactLattice()
	if operators.Join == nil || operators.Widen == nil || operators.LessOrEq == nil {
		return PlacementLattice{}, false
	}
	return PlacementLattice{operators: operators}, true
}

func (witness PlacementLattice) Join(left, right placementdomain.Fact) (placementdomain.Fact, bool) {
	return witness.operators.Join(left, right), true
}

func (witness PlacementLattice) Widen(previous, next placementdomain.Fact) (placementdomain.Fact, bool) {
	return witness.operators.Widen(previous, next), true
}

func (witness PlacementLattice) LessOrEq(left, right placementdomain.Fact) bool {
	return witness.operators.LessOrEq(left, right)
}
