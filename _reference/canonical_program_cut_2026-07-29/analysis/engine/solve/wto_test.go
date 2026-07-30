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
	got, err := SolveWTOContext(context.Background(), sys, plan)
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
	got, err := SolveWTOContext(context.Background(), EquationSystem[int, int]{
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
	got, err := SolveWTOContext(context.Background(), EquationSystem[int, int]{
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

func TestRestrictWTOPlanClosesDemandOverEnclosingSCCWithoutReplanning(t *testing.T) {
	plan, err := FreezeWTOPlan(
		[]int{0, 1, 2, 3, 4},
		[]WTOElement[int]{
			{Vertex: 0},
			{Vertex: 1, Body: []WTOElement[int]{{Vertex: 2, Body: []WTOElement[int]{{Vertex: 3}}}, {Vertex: 4}}},
		},
		[]WTOInfluence[int]{{From: 3, To: 2}, {From: 4, To: 1}},
	)
	if err != nil {
		t.Fatal(err)
	}
	restricted, err := RestrictWTOPlan(plan, []int{3})
	if err != nil {
		t.Fatal(err)
	}
	if !restricted.Matches([]int{1, 2, 3, 4}) || !reflect.DeepEqual(restricted.Elements(), plan.Elements()[1:]) {
		t.Fatalf("restriction changed frozen component schedule: %#v", restricted.Elements())
	}
	if restricted.ComponentCount() != 2 || !restricted.CoversInfluence(3, 2) || !restricted.CoversInfluence(4, 1) {
		t.Fatalf("restriction lost nested SCC schedule: %#v", restricted)
	}
	if _, err := RestrictWTOPlan(plan, []int{9}); !errors.Is(err, ErrWTOPlanRestrictionUncovered) {
		t.Fatalf("unknown demand error = %v", err)
	}
}

func TestSolveWTOFiniteLatticeExactSolution(t *testing.T) {
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
	want := map[int]int{0: 1, 1: 4, 2: 4, 3: 4, 4: 4}
	plan := NewWTOPlan(sys.Cells, func(cell int) []int { return edges[cell] })
	got, err := SolveWTOContext(context.Background(), sys, plan)
	if err != nil {
		t.Fatalf("SolveWTO: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SolveWTO = %#v, want %#v", got, want)
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
	got, err := SolveWTOContext(context.Background(), sys, NewWTOPlan(sys.Cells, func(cell int) []int {
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
	result, err := SolveWTOContext(context.Background(), sys, NewWTOPlan(sys.Cells, func(cell int) []int { return edges[cell] }))
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
	result, err := SolveWTOContext(context.Background(), sys, plan)
	if !errors.Is(err, ErrWTOPlanUncovered) || result != nil {
		t.Fatalf("duplicate equations result=%v error=%v", result, err)
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
	result, err := SolveWTOContext(ctx, sys, NewWTOPlan(sys.Cells, nil))
	if !errors.Is(err, ErrCanceled) || !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want cancellation", err)
	}
	if result != nil {
		t.Fatalf("canceled result = %#v, want nil", result)
	}
}

func TestSolveWTOComputesReachabilityAcrossSmallDirectedGraphs(t *testing.T) {
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
		want := make(map[int]uint16, n)
		for destination := 0; destination < n; destination++ {
			for source := 0; source < n; source++ {
				seen := make([]bool, n)
				queue := []int{source}
				seen[source] = true
				for len(queue) > 0 {
					from := queue[0]
					queue = queue[1:]
					for _, to := range edges[from] {
						if !seen[to] {
							seen[to] = true
							queue = append(queue, to)
						}
					}
				}
				if seen[destination] {
					want[destination] |= 1 << uint(source)
				}
			}
		}
		plan := NewWTOPlan(cells, func(cell int) []int { return edges[cell] })
		wto, err := SolveWTOContext(context.Background(), sys, plan)
		if err != nil {
			t.Fatalf("n=%d mask=%x duplicate=%v: %v", n, mask, duplicate, err)
		}
		if !reflect.DeepEqual(wto, want) {
			t.Fatalf("n=%d mask=%x duplicate=%v: WTO=%v want=%v", n, mask, duplicate, wto, want)
		}
		secondPlan := NewWTOPlan(cells, func(cell int) []int { return edges[cell] })
		second, err := SolveWTOContext(context.Background(), sys, secondPlan)
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
	wto, err := SolveWTOContext(context.Background(), sys, NewWTOPlan(cells, func(cell int) []int { return edges[cell] }))
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
	result, err := SolveWTOContext(context.Background(), sys, plan)
	if err != nil {
		t.Fatalf("SolveWTO: %v", err)
	}
	if stats.TransferCalls > 20 || result[1] != 32 {
		t.Fatalf("result=%v transfers=%d, want widened termination", result, stats.TransferCalls)
	}
}

func TestSolveWTOForwardDynamicInfluencesReachPostFixedPoint(t *testing.T) {
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
		wto, err := SolveWTOContext(context.Background(), sys, plan)
		if err != nil {
			t.Fatalf("iteration=%d mask=%x: WTO=%v err=%v", iteration, sample&0xffff, wto, err)
		}
		for cell := range cells {
			if wto[cell]&(1<<uint(cell)) == 0 {
				t.Fatalf("iteration=%d cell=%d lost initial bit: %v", iteration, cell, wto)
			}
			value := wto[cell]
			for _, other := range cells {
				if plan.rank[other] < plan.rank[cell] {
					value |= wto[other]
				}
			}
			for _, to := range edges[cell] {
				if value&^wto[to] != 0 {
					t.Fatalf("iteration=%d edge=%d->%d is not post-fixed: %v", iteration, cell, to, wto)
				}
			}
		}
	}
}

func BenchmarkSolveWTOLinear128(b *testing.B) {
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
	for i := 0; i < b.N; i++ {
		if _, err := SolveWTOContext(context.Background(), sys, plan); err != nil {
			b.Fatal(err)
		}
	}
}

// TestSolveWTOAcceptsReadInsideOneComponent pins the read admission a cyclic
// dataflow client needs. Inside a stabilization component every cell is
// revisited until the component's head stops changing, so a cell may read the
// current value of a later-ranked cell of that same component: the read follows
// declared edges and adds no vertex the frozen decomposition did not schedule.
// The same read between cells of different components stays uncovered.
func TestSolveWTOAcceptsReadInsideOneComponent(t *testing.T) {
	elements := []WTOElement[int]{{Vertex: 0, Body: []WTOElement[int]{{Vertex: 1}, {Vertex: 2}}}, {Vertex: 3}}
	influences := []WTOInfluence[int]{{From: 0, To: 1}, {From: 1, To: 2}, {From: 2, To: 0}, {From: 0, To: 3}}
	plan, err := FreezeWTOPlan([]int{0, 1, 2, 3}, elements, influences)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.CoversInfluence(2, 1) {
		t.Fatalf("a read from a later cell of the same component must be covered")
	}
	if plan.CoversInfluence(3, 1) {
		t.Fatalf("a read from a cell outside the component must stay uncovered")
	}
	if !plan.InComponent(1) || plan.InComponent(3) {
		t.Fatalf("component membership = %v/%v, want inside/outside", plan.InComponent(1), plan.InComponent(3))
	}
	// A self read inside a component is the recurrence the component iterates,
	// so it resolves instead of failing the plan.
	got, err := SolveWTOContext(context.Background(), EquationSystem[int, int]{
		Lattice: capLattice{top: 8}.lattice(), Cells: []int{0, 1, 2, 3},
		WidenAt: func(cell int) bool { return cell != 3 },
		Evaluate: func(cell int, read func(int) int) int {
			switch cell {
			case 0:
				return read(2) + 1
			case 1:
				return read(0)
			case 2:
				return read(1)
			default:
				return read(0)
			}
		},
	}, plan)
	if err != nil {
		t.Fatalf("SolveWTO: %v", err)
	}
	if got[0] != 8 || got[3] != 8 {
		t.Fatalf("solution = %v, want the component to ascend to the lattice cap", got)
	}
}
