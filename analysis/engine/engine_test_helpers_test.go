// engine_test_helpers_test.go holds the shared generic and semantic helpers the engine law tests build on.

package engine

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lattice"
)

func testRuleProjector[O any](_ O) (uint64, bool) { return 1, true }

// coldKey produces deterministic, versioned semantic identities for engine
// laws that exercise the callback-free Schema/receipt boundary.
func coldKey[N ~int | ~int8 | ~int16 | ~int32 | ~int64 | ~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64](n N) identity.SemanticKey {
	var digest [32]byte
	value := uint64(n)
	for index := 0; index < 8; index++ {
		digest[24+index] = byte(value >> uint((7-index)*8))
	}
	key, ok := identity.NewSemanticKey(digest, 1)
	if !ok {
		panic("engine law semantic key")
	}
	return key
}

func coldUintLattice() lattice.Lattice[uint64] {
	return lattice.Lattice[uint64]{
		Bottom:   func() uint64 { return 0 },
		Top:      func() uint64 { return ^uint64(0) },
		Equal:    func(left, right uint64) bool { return left == right },
		LessOrEq: func(left, right uint64) bool { return left <= right },
		Join: func(left, right uint64) uint64 {
			if left > right {
				return left
			}
			return right
		},
		Widen: func(left, right uint64) uint64 {
			if left > right {
				return left
			}
			return right
		},
	}
}
