package solve

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/lattice"
)

func TestWTOPlanNestedLoopsVisitsEveryCellOnce(t *testing.T) {
	edges := map[int][]int{0: {1}, 1: {2, 5}, 2: {3, 4}, 3: {1}, 4: {2}}
	plan := NewWTOPlan([]int{0, 1, 2, 4, 3, 5}, func(cell int) []int { return edges[cell] })
	seen := map[int]int{}
	maxDepth := 0
	var walk func([]WTOElement[int], int)
	walk = func(items []WTOElement[int], depth int) {
		if depth > maxDepth {
			maxDepth = depth
		}
		for _, item := range items {
			seen[item.Vertex]++
			walk(item.Body, depth+1)
		}
	}
	walk(plan.elements, 0)
	for cell := 0; cell <= 5; cell++ {
		if seen[cell] != 1 {
			t.Fatalf("cell %d appears %d times in %#v", cell, seen[cell], plan.elements)
		}
	}
	if maxDepth < 2 {
		t.Fatalf("nesting depth = %d, want at least 2", maxDepth)
	}
}

func TestSolveWTOMatchesFIFOOnFiniteLattice(t *testing.T) {
	edges := map[int][]int{0: {1}, 1: {2, 3}, 2: {1}, 3: {4}}
	sys := EquationSystem[int, int]{
		Lattice: capLattice{top: 8}.joinOnly(),
		Cells:   []int{0, 1, 2, 3, 4},
		InitialSparse: func(cell int) (int, bool) {
			return 1, cell == 0
		},
		Transfer: func(cell int, read func(int) int, emit func(int, int)) {
			value := read(cell)
			for _, successor := range edges[cell] {
				emit(successor, min(4, value+1))
			}
		},
	}
	want := Solve(sys)
	plan := NewWTOPlan(sys.Cells, func(cell int) []int { return edges[cell] })
	got, err := SolveWTO(sys, plan)
	if err != nil {
		t.Fatalf("SolveWTO: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SolveWTO = %#v, FIFO = %#v", got, want)
	}
}

func TestSolveWTOAcceptsForwardDynamicRead(t *testing.T) {
	edges := map[int][]int{0: {1}, 1: {2}}
	sys := EquationSystem[int, int]{
		Lattice: capLattice{top: 8}.joinOnly(),
		Cells:   []int{0, 1, 2},
		InitialSparse: func(cell int) (int, bool) {
			return 3, cell == 0
		},
		Transfer: func(cell int, read func(int) int, emit func(int, int)) {
			for _, to := range edges[cell] {
				emit(to, read(cell))
			}
			if cell == 2 {
				emit(2, read(0))
			}
		},
	}
	got, err := SolveWTO(sys, NewWTOPlan(sys.Cells, func(cell int) []int { return edges[cell] }))
	if err != nil {
		t.Fatalf("SolveWTO: %v", err)
	}
	if got[2] != 3 {
		t.Fatalf("cell 2 = %d, want 3", got[2])
	}
}

func TestSolveWTORejectsUnplannedBackwardDynamicRead(t *testing.T) {
	edges := map[int][]int{0: {1}}
	sys := EquationSystem[int, int]{
		Lattice: capLattice{top: 8}.joinOnly(),
		Cells:   []int{0, 1},
		Transfer: func(cell int, read func(int) int, emit func(int, int)) {
			for _, to := range edges[cell] {
				emit(to, read(cell))
			}
			if cell == 0 {
				emit(0, read(1))
			}
		},
	}
	result, err := SolveWTO(sys, NewWTOPlan(sys.Cells, func(cell int) []int { return edges[cell] }))
	if !errors.Is(err, ErrWTOPlanUncovered) {
		t.Fatalf("error = %v, want ErrWTOPlanUncovered", err)
	}
	if result != nil {
		t.Fatalf("result = %#v, want nil", result)
	}
}

func TestSolveWTOCanceledPublishesNothing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	sys := EquationSystem[int, int]{
		Lattice:  capLattice{top: 8}.joinOnly(),
		Cells:    []int{0},
		Transfer: func(int, func(int) int, func(int, int)) {},
	}
	result, versions, err := SolveWTOContextWithVersions(ctx, sys, NewWTOPlan(sys.Cells, nil))
	if !errors.Is(err, ErrCanceled) || !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want cancellation", err)
	}
	if result != nil || versions != nil {
		t.Fatalf("canceled result = %#v/%#v, want nil/nil", result, versions)
	}
}

func TestSolveWTOMatchesFIFOAcrossSmallDirectedGraphs(t *testing.T) {
	setDomain := lattice.Lattice[uint16]{
		Bottom:   func() uint16 { return 0 },
		Equal:    func(a, b uint16) bool { return a == b },
		LessOrEq: func(a, b uint16) bool { return a&b == a },
		Join:     func(a, b uint16) uint16 { return a | b },
	}
	check := func(n int, mask uint32, duplicate bool) {
		cells := make([]int, n)
		edges := make(map[int][]int, n)
		for from := 0; from < n; from++ {
			cells[from] = from
			for to := 0; to < n; to++ {
				if mask&(1<<uint(from*n+to)) == 0 {
					continue
				}
				edges[from] = append(edges[from], to)
				if duplicate {
					edges[from] = append(edges[from], to)
				}
			}
		}
		sys := EquationSystem[int, uint16]{
			Lattice:       setDomain,
			Cells:         cells,
			InitialSparse: func(cell int) (uint16, bool) { return 1 << uint(cell), true },
			Transfer: func(cell int, read func(int) uint16, emit func(int, uint16)) {
				value := read(cell)
				for _, successor := range edges[cell] {
					emit(successor, value)
				}
			},
		}
		fifo := Solve(sys)
		plan := NewWTOPlan(cells, func(cell int) []int { return edges[cell] })
		wto, err := SolveWTO(sys, plan)
		if err != nil {
			t.Fatalf("n=%d mask=%x duplicate=%v: %v", n, mask, duplicate, err)
		}
		if !reflect.DeepEqual(wto, fifo) {
			t.Fatalf("n=%d mask=%x duplicate=%v: WTO=%v FIFO=%v", n, mask, duplicate, wto, fifo)
		}
		secondPlan := NewWTOPlan(cells, func(cell int) []int { return edges[cell] })
		second, err := SolveWTO(sys, secondPlan)
		if err != nil || !reflect.DeepEqual(plan.elements, secondPlan.elements) || !reflect.DeepEqual(wto, second) {
			t.Fatalf("n=%d mask=%x duplicate=%v: nondeterministic plan/result", n, mask, duplicate)
		}
	}
	for n := 1; n <= 3; n++ {
		limit := uint32(1) << uint(n*n)
		for mask := uint32(0); mask < limit; mask++ {
			check(n, mask, false)
		}
	}
	var sample uint32 = 0x9e3779b9
	for i := 0; i < 1024; i++ {
		sample = sample*1664525 + 1013904223
		check(4, sample&0xffff, i%17 == 0)
	}
	// Duplicate declared cells are normalized by both solvers.
	cells := []int{0, 1, 0, 2, 1}
	edges := map[int][]int{0: {1, 1}, 1: {2}, 2: {0}}
	sys := EquationSystem[int, uint16]{Lattice: setDomain, Cells: cells,
		InitialSparse: func(cell int) (uint16, bool) { return 1 << uint(cell), true },
		Transfer: func(cell int, read func(int) uint16, emit func(int, uint16)) {
			for _, to := range edges[cell] {
				emit(to, read(cell))
			}
		},
	}
	wto, err := SolveWTO(sys, NewWTOPlan(cells, func(cell int) []int { return edges[cell] }))
	if err != nil || !reflect.DeepEqual(wto, Solve(sys)) {
		t.Fatalf("duplicate cells: WTO=%v FIFO=%v err=%v", wto, Solve(sys), err)
	}
}

func TestSolveWTOTerminatesWhenWideningPointDiffersFromWTOHead(t *testing.T) {
	edges := map[int][]int{0: {1}, 1: {2}, 2: {0}}
	stats := &Stats{}
	sys := EquationSystem[int, int]{
		Lattice: capLattice{top: 32}.lattice(), Cells: []int{0, 1, 2}, Stats: stats,
		InitialSparse: func(cell int) (int, bool) { return 1, cell == 0 },
		Transfer: func(cell int, read func(int) int, emit func(int, int)) {
			for _, to := range edges[cell] {
				emit(to, min(32, read(cell)+1))
			}
		},
		WidenAt: func(cell int) bool { return cell == 1 },
	}
	plan := NewWTOPlan(sys.Cells, func(cell int) []int { return edges[cell] })
	if len(plan.elements) != 1 || !plan.elements[0].IsComponent() || plan.elements[0].Vertex == 1 {
		t.Fatalf("plan = %#v, want component headed away from widening cell 1", plan.elements)
	}
	result, err := SolveWTO(sys, plan)
	if err != nil {
		t.Fatalf("SolveWTO: %v", err)
	}
	if stats.TransferCalls > 20 || result[1] != 32 {
		t.Fatalf("result=%v transfers=%d, want widened termination", result, stats.TransferCalls)
	}
}

func TestSolveWTOForwardDynamicInfluencesMatchFIFO(t *testing.T) {
	setDomain := lattice.Lattice[uint16]{
		Bottom: func() uint16 { return 0 }, Equal: func(a, b uint16) bool { return a == b },
		LessOrEq: func(a, b uint16) bool { return a&b == a }, Join: func(a, b uint16) uint16 { return a | b },
	}
	cells := []int{0, 1, 2, 3}
	var sample uint32 = 0x243f6a88
	for iteration := 0; iteration < 256; iteration++ {
		sample = sample*1103515245 + 12345
		edges := make(map[int][]int)
		for from := range cells {
			for to := range cells {
				if sample&(1<<uint(from*4+to)) != 0 {
					edges[from] = append(edges[from], to)
				}
			}
		}
		plan := NewWTOPlan(cells, func(cell int) []int { return edges[cell] })
		sys := EquationSystem[int, uint16]{
			Lattice: setDomain, Cells: cells,
			InitialSparse: func(cell int) (uint16, bool) { return 1 << uint(cell), true },
			Transfer: func(cell int, read func(int) uint16, emit func(int, uint16)) {
				value := read(cell)
				for _, other := range cells {
					if plan.rank[other] < plan.rank[cell] {
						value |= read(other)
					}
				}
				for _, to := range edges[cell] {
					emit(to, value)
				}
			},
		}
		fifo := Solve(sys)
		wto, err := SolveWTO(sys, plan)
		if err != nil || !reflect.DeepEqual(wto, fifo) {
			t.Fatalf("iteration=%d mask=%x: WTO=%v FIFO=%v err=%v", iteration, sample&0xffff, wto, fifo, err)
		}
	}
}

func BenchmarkSolveSchedulesLinear128(b *testing.B) {
	const count = 128
	cells := make([]int, count)
	edges := make(map[int][]int, count-1)
	for cell := range cells {
		cells[cell] = cell
		if cell+1 < count {
			edges[cell] = []int{cell + 1}
		}
	}
	sys := EquationSystem[int, int]{
		Lattice: capLattice{top: count}.joinOnly(), Cells: cells,
		InitialSparse: func(cell int) (int, bool) { return 1, cell == 0 },
		Transfer: func(cell int, read func(int) int, emit func(int, int)) {
			for _, to := range edges[cell] {
				emit(to, read(cell))
			}
		},
	}
	plan := NewWTOPlan(cells, func(cell int) []int { return edges[cell] })
	b.Run("FIFO", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = Solve(sys)
		}
	})
	b.Run("WTO", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if _, err := SolveWTO(sys, plan); err != nil {
				b.Fatal(err)
			}
		}
	})
}
