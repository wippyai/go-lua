package readexpr

import (
	"math"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
)

func TestProveArrayIndexInBoundsRejectsOverflowedLowerAffineBound(t *testing.T) {
	proof := ArrayIndexProof{
		HasTerm: true,
		Term:    ArrayIndexTerm{Path: pathdom.Path{Root: "index"}, Coeff: 2},
		LengthAtLeast: func(int64) bool {
			return true
		},
		NumericFloor: func(pathdom.Path) (int64, bool) {
			return math.MaxInt64, true
		},
	}
	if ProveArrayIndexInBounds(proof) {
		t.Fatal("overflowed affine lower bound proved index in bounds")
	}
}
