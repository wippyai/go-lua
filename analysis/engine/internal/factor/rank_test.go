package factor

import (
	"testing"
)

func TestWidenRankAcceptsScalarTupleAndEqual(t *testing.T) {
	t.Run("scalar", func(t *testing.T) {
		arena := mustArena(t, factorConfig(0))
		left := set(t, arena, arena.Empty(), 1, 2)
		right := set(t, arena, arena.Empty(), 1, 3)
		work := arena.Begin()
		widened, ok := work.Widen(left, right, nil)
		if !ok || !publish(work, &widened) {
			t.Fatal("scalar ranked widening rejected")
		}
		if got, _, _ := arena.Get(widened, 1); got != 3 {
			t.Fatalf("scalar widened value = %d, want 3", got)
		}
	})

	t.Run("lexicographic", func(t *testing.T) {
		config := factorConfig(0)
		config.WidenRank = Measure[uint64, uint8]{
			Width: 2,
			At: func(_ uint64, value uint8, component int) uint64 {
				if component == 0 {
					return 17
				}
				return ^uint64(value)
			},
		}
		arena := mustArena(t, config)
		left := set(t, arena, arena.Empty(), 1, 2)
		right := set(t, arena, arena.Empty(), 1, 3)
		work := arena.Begin()
		next, ok := work.Widen(left, right, nil)
		if !ok || !publish(work, &next) {
			t.Fatal("lexicographic ranked widening rejected")
		}
	})

	t.Run("equal", func(t *testing.T) {
		arena := mustArena(t, factorConfig(0))
		root := set(t, arena, arena.Empty(), 1, 3)
		work := arena.Begin()
		next, ok := work.Widen(root, root, nil)
		if !ok || !publish(work, &next) {
			t.Fatal("equal widening was not accepted exactly")
		}
		if equal, valid := arena.Equal(root, next); !valid || !equal {
			t.Fatal("equal widening changed the factor")
		}
	})
}

func TestArenaRankConfiguration(t *testing.T) {
	config := factorConfig(0)
	config.WidenRank = Measure[uint64, uint8]{}
	arena, ok := New(config)
	if !ok || arena == nil {
		t.Fatal("Arena rejected an absent Widen rank for an acyclic Factor")
	}
	left := set(t, arena, arena.Empty(), 1, 1)
	right := set(t, arena, arena.Empty(), 1, 2)
	work := arena.Begin()
	if _, ok := work.Widen(left, right, nil); ok {
		t.Fatal("unranked Factor widened without a termination proof")
	}

	config = factorConfig(0)
	config.WidenRank.Width = 0
	if arena, ok := New(config); ok || arena != nil {
		t.Fatal("Arena accepted zero-width Widen rank")
	}

	config = factorConfig(0)
	config.WidenRank.Width = -1
	if arena, ok := New(config); ok || arena != nil {
		t.Fatal("Arena accepted negative-width Widen rank")
	}

	config = factorConfig(0)
	config.Lattice.Narrow = func(left, right uint8) uint8 { return left + right }
	if arena, ok := New(config); ok || arena != nil {
		t.Fatal("Factor accepted Narrow without its mandatory NarrowRank")
	}
}

func TestWidenRejectsDishonestBoundsAndRanksWithoutPublishingChanges(t *testing.T) {
	cases := []struct {
		name  string
		widen func(uint8, uint8) uint8
		rank  func(uint64, uint8, int) uint64
		left  uint8
		right uint8
		width int
	}{
		{
			name:  "equal rank",
			widen: maxUint8,
			rank:  func(uint64, uint8, int) uint64 { return 0 },
			left:  2,
			right: 3,
			width: 1,
		},
		{
			name:  "increasing rank",
			widen: maxUint8,
			rank:  func(_ uint64, value uint8, _ int) uint64 { return uint64(value) },
			left:  2,
			right: 3,
			width: 1,
		},
		{
			name: "not upper bound",
			widen: func(left, right uint8) uint8 {
				if left == 0 && right == 0 {
					return 0
				}
				return left
			},
			rank:  func(_ uint64, value uint8, _ int) uint64 { return ^uint64(value) },
			left:  2,
			right: 3,
			width: 1,
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			config := factorConfig(0)
			config.Lattice.Widen = test.widen
			config.WidenRank = Measure[uint64, uint8]{Width: test.width, At: test.rank}
			arena := mustArena(t, config)
			left := set(t, arena, arena.Empty(), 1, test.left)
			right := set(t, arena, arena.Empty(), 1, test.right)
			work := arena.Begin()
			var changes Changes[uint64]
			if _, ok := work.Widen(left, right, &changes); ok {
				t.Fatal("dishonest widening returned a candidate")
			}
			if len(changes.Drain()) != 0 {
				t.Fatalf("failed widening leaked %d changed locations", len(changes.Drain()))
			}
		})
	}
}

func TestWidenChecksDefaultOperand(t *testing.T) {
	config := factorConfig(0)
	config.Lattice.Widen = func(left, right uint8) uint8 {
		if left == 0 && right == 0 {
			return 0
		}
		return left + 1
	}
	arena := mustArena(t, config)
	left := set(t, arena, arena.Empty(), 1, 2)
	work := arena.Begin()
	widened, ok := work.Widen(left, arena.Empty(), nil)
	if !ok || !publish(work, &widened) {
		t.Fatal("widening against Default was rejected")
	}
	if got, _, _ := arena.Get(widened, 1); got != 3 {
		t.Fatalf("widening against Default = %d, want 3", got)
	}

	config = factorConfig(0)
	config.Lattice.Widen = func(left, right uint8) uint8 {
		if left == 0 && right == 0 {
			return 0
		}
		if right == 0 {
			return 0
		}
		return maxUint8(left, right)
	}
	arena = mustArena(t, config)
	left = set(t, arena, arena.Empty(), 1, 2)
	work = arena.Begin()
	if _, ok := work.Widen(left, arena.Empty(), nil); ok {
		t.Fatal("widening below the left/default operand was accepted")
	}
}

func TestWidenRankChecksAbsentToExplicitCoordinate(t *testing.T) {
	config := factorConfigWithKeyRange(KeyRange{End: 2}, 0)
	config.WidenRank = Measure[uint64, uint8]{
		Width: 1,
		At: func(_ uint64, value uint8, _ int) uint64 {
			return ^uint64(value)
		},
	}
	arena := mustArena(t, config)
	right := set(t, arena, arena.Empty(), 1, 3)
	work := arena.Begin()
	widened, valid := work.Widen(arena.Empty(), right, nil)
	if !valid || !publish(work, &widened) {
		t.Fatal("ranked absent-to-explicit widening rejected")
	}
	if value, present, valid := arena.Get(widened, 1); !valid || !present || value != 3 {
		t.Fatalf("absent-to-explicit widening = %d/%t/%t", value, present, valid)
	}
}

func TestNarrowRankAcceptsAWellFoundedRefinement(t *testing.T) {
	config := factorConfig(0)
	config.Lattice.Narrow = func(left, right uint8) uint8 {
		if left < right {
			return left
		}
		return right
	}
	config.NarrowRank = Measure[uint64, uint8]{
		Width: 1,
		At: func(_ uint64, value uint8, _ int) uint64 {
			return uint64(value)
		},
	}
	arena := mustArena(t, config)
	left := set(t, arena, arena.Empty(), 1, 3)
	right := set(t, arena, arena.Empty(), 1, 2)
	work := arena.Begin()
	next, ok := work.Narrow(left, right, nil)
	if !ok || !publish(work, &next) {
		t.Fatal("well-founded Narrow rejected")
	}
	if got, _, valid := arena.Get(next, 1); !valid || got != 2 {
		t.Fatalf("narrowed value = %d/%v, want 2/true", got, valid)
	}
}

func TestNarrowRejectsDishonestBoundsAndRanksWithoutPublishingChanges(t *testing.T) {
	cases := []struct {
		name   string
		narrow func(uint8, uint8) uint8
		rank   func(uint64, uint8, int) uint64
	}{
		{
			name: "below body result",
			narrow: func(_, right uint8) uint8 {
				if right == 0 {
					return 0
				}
				return right - 1
			},
			rank: func(_ uint64, value uint8, _ int) uint64 { return uint64(value) },
		},
		{
			name:   "non-descending rank",
			narrow: func(_, right uint8) uint8 { return right },
			rank:   func(_ uint64, value uint8, _ int) uint64 { return ^uint64(value) },
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			config := factorConfig(0)
			config.Lattice.Narrow = test.narrow
			config.NarrowRank = Measure[uint64, uint8]{Width: 1, At: test.rank}
			arena := mustArena(t, config)
			left := set(t, arena, arena.Empty(), 1, 3)
			right := set(t, arena, arena.Empty(), 1, 2)
			work := arena.Begin()
			var changes Changes[uint64]
			if _, ok := work.Narrow(left, right, &changes); ok {
				t.Fatal("dishonest Narrow returned a candidate")
			}
			if got := len(changes.Drain()); got != 0 {
				t.Fatalf("failed Narrow retained %d changed locations", got)
			}
		})
	}
}

func TestWidenFailureRewindsEarlierChangedCoordinates(t *testing.T) {
	keys := []uint64{0, 1, 2, 3}
	for _, badKey := range keys {
		t.Run("bad-key", func(t *testing.T) {
			config := factorConfigWithKeyRange(KeyRange{End: uint64(len(keys))}, 0)
			config.WidenRank = Measure[uint64, uint8]{
				Width: 1,
				At: func(key uint64, value uint8, _ int) uint64 {
					if key == badKey {
						return uint64(value) // dishonest at this coordinate
					}
					return ^uint64(value)
				},
			}
			arena := mustArena(t, config)
			left, right := arena.Empty(), arena.Empty()
			for _, key := range keys {
				left = set(t, arena, left, key, 2)
				right = set(t, arena, right, key, 3)
			}
			work := arena.Begin()
			var changes Changes[uint64]
			if _, valid := work.Widen(left, right, &changes); valid {
				t.Fatal("partially dishonest widening returned a candidate")
			}
			if got := len(changes.Drain()); got != 0 {
				t.Fatalf("failed widening retained %d earlier changes", got)
			}
		})
	}
}

func TestRankComparisonDoesNotAllocate(t *testing.T) {
	measure := Measure[uint64, uint8]{
		Width: 3,
		At: func(_ uint64, value uint8, component int) uint64 {
			if component < 2 {
				return 7
			}
			return ^uint64(value)
		},
	}
	if !measure.descends(1, 2, 3) {
		t.Fatal("rank setup did not descend")
	}
	allocations := testing.AllocsPerRun(1000, func() {
		if !measure.descends(1, 2, 3) {
			t.Fatal("rank descent changed")
		}
	})
	if allocations != 0 {
		t.Fatalf("rank comparison allocated %f times, want 0", allocations)
	}
}

func maxUint8(left, right uint8) uint8 {
	if left > right {
		return left
	}
	return right
}
