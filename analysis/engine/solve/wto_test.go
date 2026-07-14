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

func TestCondensationWTOPlanDirectEquationsDependencyFirst(t *testing.T) {
	plan, err := FreezeWTOPlan(
		[]int{0, 1},
		[]WTOElement[int]{{Vertex: 0}, {Vertex: 1}},
		[]WTOInfluence[int]{{From: 0, To: 1}},
	)
	if err != nil {
		t.Fatal(err)
	}
	calls := [2]int{}
	sys := EquationSystem[int, int]{
		Lattice: capLattice{top: 8}.joinOnly(), Cells: []int{0, 1},
		Evaluate: func(cell int, read func(int) int) int {
			calls[cell]++
			if cell == 0 {
				return 1
			}
			return read(0) + 1
		},
	}
	got, err := SolveWTO(sys, plan)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, map[int]int{0: 1, 1: 2}) || calls != [2]int{1, 1} {
		t.Fatalf("result=%v calls=%v, want map[0:1 1:2] and one call each", got, calls)
	}
}

func TestCondensationWTOPlanDirectCyclicEquationStabilizes(t *testing.T) {
	plan, err := FreezeWTOPlan(
		[]int{0},
		[]WTOElement[int]{{Vertex: 0, Body: []WTOElement[int]{}}},
		[]WTOInfluence[int]{{From: 0, To: 0}},
	)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	got, err := SolveWTO(EquationSystem[int, int]{
		Lattice: capLattice{top: 8}.joinOnly(), Cells: []int{0},
		Evaluate: func(cell int, read func(int) int) int {
			calls++
			return min(3, read(cell)+1)
		},
	}, plan)
	if err != nil {
		t.Fatal(err)
	}
	if got[0] != 3 || calls != 4 {
		t.Fatalf("result=%v calls=%d, want value 3 after four confirmation visits", got, calls)
	}
}

func TestCondensationWTOPlanRejectsUnplannedDirectSelfRead(t *testing.T) {
	plan, err := FreezeWTOPlan([]int{0}, []WTOElement[int]{{Vertex: 0}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := SolveWTO(EquationSystem[int, int]{
		Lattice: capLattice{top: 8}.joinOnly(), Cells: []int{0},
		Evaluate: func(cell int, read func(int) int) int {
			return read(cell) + 1
		},
	}, plan)
	if !errors.Is(err, ErrWTOPlanUncovered) || got != nil {
		t.Fatalf("result=%v error=%v, want fail-closed uncovered plan", got, err)
	}
}

func TestCondensationWTOPlanValidatesCanonicalDAG(t *testing.T) {
	if _, err := FreezeWTOPlan([]int{0, 0}, []WTOElement[int]{{Vertex: 0}}, nil); !errors.Is(err, ErrWTOInvalidFrozenPlan) {
		t.Fatalf("duplicate error=%v", err)
	}
	if _, err := FreezeWTOPlan(
		[]int{0, 1},
		[]WTOElement[int]{{Vertex: 0}, {Vertex: 1}},
		[]WTOInfluence[int]{{From: 1, To: 0}},
	); !errors.Is(err, ErrWTOInvalidFrozenPlan) {
		t.Fatalf("backward-edge error=%v", err)
	}
}

func TestFreezeWTOPlanOwnsExactNestedSchedule(t *testing.T) {
	elements := []WTOElement[int]{{Vertex: 0, Body: []WTOElement[int]{{Vertex: 1}}}}
	influences := []WTOInfluence[int]{{From: 1, To: 0}}
	plan, err := FreezeWTOPlan([]int{0, 1}, elements, influences)
	if err != nil {
		t.Fatal(err)
	}
	if plan.ComponentCount() != 1 || !plan.CoversInfluence(1, 0) {
		t.Fatalf("components=%d covers-backedge=%v", plan.ComponentCount(), plan.CoversInfluence(1, 0))
	}
	// The frozen plan owns both structural and influence inputs.
	elements[0].Body[0].Vertex = 9
	influences[0] = WTOInfluence[int]{From: 0, To: 9}
	got := plan.Elements()
	if len(got) != 1 || len(got[0].Body) != 1 || got[0].Body[0].Vertex != 1 || !plan.CoversInfluence(1, 0) {
		t.Fatalf("caller mutation changed frozen plan: %#v", got)
	}
}

func TestFreezeWTOPlanRejectsInexactCoverageAndNonHeadBackedge(t *testing.T) {
	if _, err := FreezeWTOPlan([]int{0, 1}, []WTOElement[int]{{Vertex: 0}}, nil); !errors.Is(err, ErrWTOInvalidFrozenPlan) {
		t.Fatalf("missing-cell error=%v", err)
	}
	if _, err := FreezeWTOPlan(
		[]int{0, 1, 2},
		[]WTOElement[int]{{Vertex: 0, Body: []WTOElement[int]{{Vertex: 1}, {Vertex: 2}}}},
		[]WTOInfluence[int]{{From: 2, To: 1}},
	); !errors.Is(err, ErrWTOInvalidFrozenPlan) {
		t.Fatalf("non-head backedge error=%v", err)
	}
}

func TestBuildRetainedWTORejectsDirectEquations(t *testing.T) {
	sys := EquationSystem[int, int]{
		Lattice: capLattice{top: 8}.joinOnly(), Cells: []int{0},
		Evaluate: func(int, func(int) int) int { return 1 },
	}
	values, versions, retained, err := BuildRetainedWTO(context.Background(), sys, NewWTOPlan(sys.Cells, nil), RetainedBudget{})
	if !errors.Is(err, ErrRetainedEvaluateUnsupported) || values != nil || versions != nil || retained != nil {
		t.Fatalf("values=%v versions=%v retained=%v error=%v", values, versions, retained, err)
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
	got, err := SolveWTO(sys, NewWTOPlan(sys.Cells, func(cell int) []int {
		if cell == 2 {
			// The dynamic read is forward, while the declared self emission is
			// conservatively represented so the plan can iterate it.
			return []int{2}
		}
		return edges[cell]
	}))
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

func TestSolveWTORejectsDuplicateEquationCellsAgainstExactPlan(t *testing.T) {
	sys := EquationSystem[int, int]{
		Lattice:  capLattice{top: 8}.joinOnly(),
		Cells:    []int{0, 0},
		Transfer: func(int, func(int) int, func(int, int)) {},
	}
	plan := NewWTOPlan([]int{0}, nil)
	result, versions, err := SolveWTOWithVersions(sys, plan)
	if !errors.Is(err, ErrWTOPlanUncovered) || result != nil || versions != nil {
		t.Fatalf("duplicate equations result=%v versions=%v error=%v", result, versions, err)
	}
	result, versions, retained, err := BuildRetainedWTO(context.Background(), sys, plan, RetainedBudget{})
	if !errors.Is(err, ErrWTOPlanUncovered) || result != nil || versions != nil || retained != nil {
		t.Fatalf("duplicate retained result=%v versions=%v retained=%v error=%v", result, versions, retained, err)
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
	// A prepared WTO owns an exact equation sequence; duplicate owners are not
	// silently normalized against its canonical plan.
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
	if !errors.Is(err, ErrWTOPlanUncovered) || wto != nil {
		t.Fatalf("duplicate cells: WTO=%v err=%v", wto, err)
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

func TestSolveWTORetainedCheckpointOwnsPreNarrowHistory(t *testing.T) {
	edges := map[int][]int{0: {1}, 1: {0, 2}}
	sys := EquationSystem[int, int]{
		Lattice: capLattice{top: 16}.lattice(), Cells: []int{0, 1, 2},
		InitialSparse: func(cell int) (int, bool) { return 1, cell == 0 },
		Transfer: func(cell int, read func(int) int, emit func(int, int)) {
			for _, destination := range edges[cell] {
				emit(destination, min(16, read(cell)+1))
			}
		},
		WidenAt: func(cell int) bool { return cell == 0 },
	}
	result, versions, retained, err := BuildRetainedWTO(context.Background(), sys, NewWTOPlan(sys.Cells, func(cell int) []int { return edges[cell] }), RetainedBudget{})
	if err != nil {
		t.Fatal(err)
	}
	if retained == nil || len(result) != len(sys.Cells) || len(versions) != len(sys.Cells) {
		t.Fatalf("retained result/checkpoint missing: retained=%#v", retained)
	}
	if retained.visits[0] == 0 || retained.nextVersion == 0 || len(retained.values) != len(sys.Cells) || len(retained.versions) != len(sys.Cells) {
		t.Fatalf("incomplete pre-narrow history: %#v", retained)
	}
	if len(retained.owners) != len(sys.Cells) || len(retained.readers) == 0 || len(retained.outputOwners) == 0 {
		t.Fatalf("incomplete owner provenance: %#v", retained)
	}
	retained.Release()
	if retained.values != nil || retained.visits != nil || retained.nextVersion != 0 || retained.owners != nil {
		t.Fatalf("checkpoint retained after release: %#v", retained)
	}
}

func TestRetainedRecorderAtomicallyReplacesAndJoinsOwnerBag(t *testing.T) {
	domain := capLattice{top: 32}.lattice()
	recorder := newRetainedRecorder([]int{0, 1, 2}, domain, RetainedBudget{})
	recorder.begin(0)
	recorder.read(0, 3, 7)
	recorder.emit(1, 2)
	recorder.emit(1, 4)
	recorder.emit(2, 8)
	if err := recorder.commit(); err != nil {
		t.Fatal(err)
	}
	owners, _, reverse := recorder.compact([]int{0, 1, 2})
	if len(owners) != 1 || len(owners[0].outputs) != 2 || owners[0].outputs[0].contribution != 4 {
		t.Fatalf("joined owner bag = %#v", owners)
	}
	if len(reverse) != 2 {
		t.Fatalf("reverse outputs = %#v", reverse)
	}

	// A failed replacement cannot clear the previously committed bag.
	recorder.budget.MaxOutputs = 1
	recorder.begin(0)
	recorder.emit(1, 16)
	recorder.emit(2, 16)
	if !errors.Is(recorder.commit(), ErrRetainedBudget) {
		t.Fatal("replacement unexpectedly fit budget")
	}
	recorder.discard()
	owners, _, reverse = recorder.compact([]int{0, 1, 2})
	if len(owners) != 1 || len(owners[0].outputs) != 2 || len(reverse) != 2 {
		t.Fatalf("failed replacement mutated committed bag: owners=%#v reverse=%#v", owners, reverse)
	}

	// A successful later visit is replacement semantics, not accumulation.
	recorder.budget.MaxOutputs = 0
	recorder.begin(0)
	recorder.emit(1, 1)
	if err := recorder.commit(); err != nil {
		t.Fatal(err)
	}
	owners, _, reverse = recorder.compact([]int{0, 1, 2})
	if len(owners[0].outputs) != 1 || owners[0].outputs[0].destination != 1 || len(reverse) != 1 {
		t.Fatalf("replacement retained removed output: owners=%#v reverse=%#v", owners, reverse)
	}
}

func TestBuildRetainedWTOIncludesEmittedOnlyCells(t *testing.T) {
	sys := EquationSystem[int, int]{
		Lattice: capLattice{top: 32}.lattice(), Cells: []int{0},
		Transfer: func(cell int, read func(int) int, emit func(int, int)) { emit(9, 7) },
	}
	result, versions, retained, err := BuildRetainedWTO(context.Background(), sys, NewWTOPlan(sys.Cells, nil), RetainedBudget{})
	if err != nil {
		t.Fatal(err)
	}
	defer retained.Release()
	value, ok := retained.Value(9)
	if !ok || value != 7 || retained.Version(9) == 0 {
		t.Fatalf("emitted-only value missing: value=%d ok=%v retainedVersion=%d result=%v versions=%v", value, ok, retained.Version(9), result, versions)
	}
	if len(retained.outputOwners) != 1 || retained.outputOwners[0].cell != 9 {
		t.Fatalf("emitted-only reverse edge missing: %#v", retained.outputOwners)
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
