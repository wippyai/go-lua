package relation

import (
	"github.com/wippyai/go-lua/analysis/lattice"
	staticdomain "github.com/wippyai/go-lua/domain/static"
)

// StaticLattice is domain/static's type-fact ascent authority, spelled by its
// class set as a struct of total operators.
type StaticLattice struct {
	operators lattice.Lattice[staticdomain.TypeFact]
}

// NewStaticLattice adopts one sealed class set's type-fact operator set.
func NewStaticLattice(classes *staticdomain.ClassSet) (StaticLattice, bool) {
	if classes == nil {
		return StaticLattice{}, false
	}
	operators := classes.TypeFactLattice()
	if operators.Join == nil || operators.Widen == nil || operators.LessOrEq == nil {
		return StaticLattice{}, false
	}
	return StaticLattice{operators: operators}, true
}

func (witness StaticLattice) Join(left, right staticdomain.TypeFact) (staticdomain.TypeFact, bool) {
	return witness.operators.Join(left, right), true
}

func (witness StaticLattice) Widen(previous, next staticdomain.TypeFact) (staticdomain.TypeFact, bool) {
	return witness.operators.Widen(previous, next), true
}

func (witness StaticLattice) LessOrEq(left, right staticdomain.TypeFact) bool {
	return witness.operators.LessOrEq(left, right)
}
