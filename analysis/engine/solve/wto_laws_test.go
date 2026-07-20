package solve

import (
	"context"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/lattice"
)

type capLattice struct{ top int }

func (c capLattice) lattice() lattice.Lattice[int] {
	return lattice.Lattice[int]{
		Bottom: func() int { return 0 }, Top: func() int { return c.top },
		Equal: func(a, b int) bool { return a == b }, LessOrEq: func(a, b int) bool { return a <= b },
		Join: maxi, Meet: mini,
		Widen: func(previous, next int) int {
			if previous == next {
				return previous
			}
			return c.top
		},
	}
}

func (c capLattice) joinOnly() lattice.Lattice[int] {
	domain := c.lattice()
	domain.Widen = maxi
	return domain
}

func maxi(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func mini(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestSolveWTONarrowingRunsToEqualityWithoutIterationCap(t *testing.T) {
	const depth = 96
	domain := capLattice{top: depth}.lattice()
	domain.Narrow = func(previous, candidate int) int {
		if candidate < previous {
			return previous - 1
		}
		return previous
	}
	narrowCalls := 0
	system := EquationSystem[string, int]{
		Lattice: domain, Cells: []string{"loop"},
		Transfer: func(cell string, _ func(string) int, emit func(string, int)) {
			narrowCalls++
			emit(cell, 0)
		},
		TransferVersioned: func(cell string, _ func(string) (int, uint64), emit func(string, int)) {
			emit(cell, depth)
		},
		WidenAt: func(string) bool { return true },
	}
	result, err := SolveWTOContext(context.Background(), system, NewWTOPlan(system.Cells, func(string) []string { return []string{"loop"} }))
	if err != nil {
		t.Fatal(err)
	}
	if result["loop"] != 0 || narrowCalls != depth+1 {
		t.Fatalf("result=%v narrowing calls=%d, want zero after %d strict rounds plus equality", result, narrowCalls, depth)
	}
}

func TestSolveWTONarrowingCoversExistingAndCandidateOnlyEmissions(t *testing.T) {
	domain := capLattice{top: 10}.lattice()
	domain.Narrow = mini
	abstractCalls := make(map[string]int)
	system := EquationSystem[string, int]{
		Lattice: domain, Cells: []string{"declared"},
		Transfer: func(_ string, _ func(string) int, emit func(string, int)) {
			emit("emitted", 1)
			emit("candidate", 1)
		},
		TransferVersioned: func(_ string, _ func(string) (int, uint64), emit func(string, int)) {
			emit("emitted", 10)
		},
		WidenAt: func(string) bool { return true },
		Abstract: func(cell string, value int) int {
			abstractCalls[cell]++
			return value
		},
	}
	_, err := SolveWTOContext(context.Background(), system, NewWTOPlan(system.Cells, nil))
	if err != nil {
		t.Fatal(err)
	}
	// Ascent accounts for one "emitted" call. Each narrowing round accounts
	// for construction and application; candidate-only must participate in the
	// first round rather than being deferred.
	if abstractCalls["emitted"] != 5 || abstractCalls["candidate"] != 4 {
		t.Fatalf("abstract calls=%v, want emitted=5 candidate=4", abstractCalls)
	}
}

func TestSolveWTOSparseInitialsPublishOnlyDeclaredCells(t *testing.T) {
	domain := capLattice{top: 100}.joinOnly()
	system := EquationSystem[string, int]{
		Lattice: domain, Cells: []string{"a", "b"},
		InitialSparse: func(cell string) (int, bool) { return 3, cell == "a" },
		Transfer: func(cell string, _ func(string) int, emit func(string, int)) {
			if cell == "a" {
				emit("ghost", 9)
			}
		},
	}
	result, err := SolveWTOContext(context.Background(), system, NewWTOPlan(system.Cells, nil))
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]int{"a": 3, "b": 0}; !reflect.DeepEqual(result, want) {
		t.Fatalf("result=%v, want %v", result, want)
	}
}
