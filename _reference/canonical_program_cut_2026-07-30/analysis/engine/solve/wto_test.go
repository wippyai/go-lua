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
	elements := plan.Elements()
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
	walk(elements, 0)
	for cell := 0; cell <= 5; cell++ {
		if seen[cell] != 1 {
			t.Fatalf("cell %d appears %d times in %#v", cell, seen[cell], elements)
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
	influences := []WTOInfluence[int]{{From: 0, To: 1}, {From: 1, To: 0}}
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

func TestWTOPlanComponentHeadsAreExactAndAllocationFree(t *testing.T) {
	acyclic := NewWTOPlan([]int{0, 1}, func(cell int) []int {
		if cell == 0 {
			return []int{1}
		}
		return nil
	})
	for _, cell := range []int{0, 1} {
		if acyclic.IsComponentHead(cell) {
			t.Fatalf("acyclic cell %d was marked as a component head", cell)
		}
	}

	selfLoop := NewWTOPlan([]int{7}, func(int) []int { return []int{7} })
	if !selfLoop.IsComponentHead(7) {
		t.Fatal("self-loop head was not selected")
	}
	if allocations := testing.AllocsPerRun(1000, func() { _ = selfLoop.IsComponentHead(7) }); allocations != 0 {
		t.Fatalf("IsComponentHead allocations = %f, want zero", allocations)
	}
}

func TestFreezeWTOPlanPreservesEveryNestedComponentHead(t *testing.T) {
	cells := []int{0, 1, 2, 3, 4}
	elements := []WTOElement[int]{
		{Vertex: 0, Body: []WTOElement[int]{
			{Vertex: 1, Body: []WTOElement[int]{{Vertex: 2}}},
			{Vertex: 3},
		}},
		{Vertex: 4},
	}
	plan, err := FreezeWTOPlan(cells, elements, []WTOInfluence[int]{
		{From: 0, To: 1}, {From: 1, To: 2}, {From: 2, To: 1}, {From: 1, To: 3}, {From: 3, To: 0},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, cell := range []int{0, 1} {
		if !plan.IsComponentHead(cell) {
			t.Fatalf("nested component head %d was not selected", cell)
		}
	}
	for _, cell := range []int{2, 3, 4} {
		if plan.IsComponentHead(cell) {
			t.Fatalf("body-only or acyclic cell %d was marked as a component head", cell)
		}
	}

	// Freeze owns a clone of the schedule; later caller mutation cannot alter
	// the exact widening cut set it derived at construction time.
	elements[0].Vertex = 9
	for _, cell := range []int{0, 1} {
		if !plan.IsComponentHead(cell) {
			t.Fatalf("caller mutation changed frozen component head %d", cell)
		}
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

func TestFreezeWTOPlanRejectsAcyclicFakeComponent(t *testing.T) {
	_, err := FreezeWTOPlan(
		[]int{0, 1},
		[]WTOElement[int]{{Vertex: 0, Body: []WTOElement[int]{{Vertex: 1}}}},
		[]WTOInfluence[int]{{From: 0, To: 1}},
	)
	if !errors.Is(err, ErrWTOInvalidFrozenPlan) {
		t.Fatalf("acyclic fake component error = %v, want %v", err, ErrWTOInvalidFrozenPlan)
	}
}

func TestFreezeWTOPlanRejectsAlternateInvalidNesting(t *testing.T) {
	cells := []int{0, 1, 2, 3}
	influences := []WTOInfluence[int]{
		{From: 0, To: 1},
		{From: 1, To: 2},
		{From: 2, To: 1},
		{From: 1, To: 3},
		{From: 3, To: 0},
	}
	canonical := []WTOElement[int]{{Vertex: 0, Body: []WTOElement[int]{
		{Vertex: 1, Body: []WTOElement[int]{{Vertex: 2}}},
		{Vertex: 3},
	}}}
	if _, err := FreezeWTOPlan(cells, canonical, influences); err != nil {
		t.Fatalf("canonical nesting was rejected: %v", err)
	}
	alternate := []WTOElement[int]{{Vertex: 0, Body: []WTOElement[int]{
		{Vertex: 1},
		{Vertex: 2, Body: []WTOElement[int]{{Vertex: 3}}},
	}}}
	if _, err := FreezeWTOPlan(cells, alternate, influences); !errors.Is(err, ErrWTOInvalidFrozenPlan) {
		t.Fatalf("alternate nesting error = %v, want %v", err, ErrWTOInvalidFrozenPlan)
	}
}

func TestWTOPlanningIsStackSafeAndReadsEachSuccessorRowOnce(t *testing.T) {
	const count = 100_000
	cells := make([]int, count)
	calls := make([]int, count)
	for index := range cells {
		cells[index] = index
	}
	plan := NewWTOPlan(cells, func(cell int) []int {
		calls[cell]++
		if cell+1 == count {
			return nil
		}
		return []int{cell + 1, cell + 1}
	})
	if plan.ComponentCount() != 0 || len(plan.Elements()) != count {
		t.Fatalf("deep acyclic plan has components=%d elements=%d", plan.ComponentCount(), len(plan.Elements()))
	}
	for cell, callCount := range calls {
		if callCount != 1 {
			t.Fatalf("successors(%d) called %d times, want exactly once", cell, callCount)
		}
	}
	frozen, err := FreezeWTOPlan(cells, plan.Elements(), plan.Influences())
	if err != nil {
		t.Fatalf("deep acyclic canonical freeze: %v", err)
	}
	if frozen.ComponentCount() != 0 || len(frozen.Elements()) != count {
		t.Fatalf("deep frozen plan has components=%d elements=%d", frozen.ComponentCount(), len(frozen.Elements()))
	}
}

func TestWTOPlanningAndFreezeAreStackSafeForDeepNesting(t *testing.T) {
	const count = 4_096
	cells := make([]int, count)
	for index := range cells {
		cells[index] = index
	}
	plan := NewWTOPlan(cells, func(cell int) []int {
		if cell+1 < count {
			return []int{cell + 1}
		}
		backedges := make([]int, count-1)
		for index := range backedges {
			backedges[index] = index
		}
		return backedges
	})
	if plan.ComponentCount() != count-1 {
		t.Fatalf("deep plan components = %d, want %d", plan.ComponentCount(), count-1)
	}
	frozen, err := FreezeWTOPlan(cells, plan.Elements(), plan.Influences())
	if err != nil {
		t.Fatalf("deep canonical freeze: %v", err)
	}
	if frozen.ComponentCount() != plan.ComponentCount() {
		t.Fatalf("frozen components = %d, want %d", frozen.ComponentCount(), plan.ComponentCount())
	}

	calls := 0
	result, err := SolveWTOContext(context.Background(), EquationSystem[int, int]{
		Lattice: capLattice{top: 1}.joinOnly(),
		Cells:   cells,
		Initial: func(int) int { return 0 },
		Evaluate: func(int, func(int) int) int {
			calls++
			return 0
		},
	}, frozen)
	if err != nil {
		t.Fatalf("deep iterative solve: %v", err)
	}
	if len(result) != count || calls != count {
		t.Fatalf("deep iterative solve cells=%d calls=%d, want %d/%d", len(result), calls, count, count)
	}
}

func TestWTOPlanIgnoresDuplicateAndPermutedSuccessorSpellings(t *testing.T) {
	cells := []int{0, 1, 2, 3, 4}
	first := map[int][]int{0: {3, 1, 3, 2}, 1: {4}, 2: {4}, 3: {4}, 4: {0}}
	second := map[int][]int{0: {2, 1, 3}, 1: {4, 4}, 2: {4}, 3: {4}, 4: {0, 0}}
	left := NewWTOPlan(cells, func(cell int) []int { return first[cell] })
	right := NewWTOPlan(cells, func(cell int) []int { return second[cell] })
	if !reflect.DeepEqual(left.Elements(), right.Elements()) || !reflect.DeepEqual(left.Influences(), right.Influences()) {
		t.Fatalf("equivalent graphs produced different canonical schedules")
	}
}

func TestRestrictWTOPlanClosesDemandOverEnclosingSCCWithoutReplanning(t *testing.T) {
	plan, err := FreezeWTOPlan(
		[]int{0, 1, 2, 3, 4},
		[]WTOElement[int]{
			{Vertex: 0},
			{Vertex: 1, Body: []WTOElement[int]{{Vertex: 2, Body: []WTOElement[int]{{Vertex: 3}}}, {Vertex: 4}}},
		},
		[]WTOInfluence[int]{{From: 1, To: 2}, {From: 2, To: 3}, {From: 3, To: 2}, {From: 2, To: 4}, {From: 4, To: 1}},
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

func TestRestrictedWTOPlanFailsClosedForExcludedCells(t *testing.T) {
	plan := NewWTOPlan([]int{0, 1}, nil)
	restricted, err := RestrictWTOPlan(plan, []int{1})
	if err != nil {
		t.Fatal(err)
	}
	if restricted.CoversInfluence(0, 1) || restricted.CoversEmission(1, 0) {
		t.Fatal("restricted plan admitted an influence crossing its retained membership")
	}
	if got := restricted.AppendCells(nil); !reflect.DeepEqual(got, []int{1}) {
		t.Fatalf("restricted execution cells = %v, want [1]", got)
	}
}

func TestRestrictWTOPlanNoncontiguousRootsOwnDenseMembership(t *testing.T) {
	plan := NewWTOPlan([]int{0, 1, 2, 3}, nil)
	restricted, err := RestrictWTOPlan(plan, []int{3, 1})
	if err != nil {
		t.Fatal(err)
	}
	if got := restricted.AppendCells(nil); !reflect.DeepEqual(got, []int{1, 3}) {
		t.Fatalf("restricted execution cells = %v, want [1 3]", got)
	}
	if !restricted.Matches([]int{1, 3}) {
		t.Fatal("restricted plan does not match its exact local cell order")
	}
	for index, cell := range []int{1, 3} {
		got, ok := restricted.CanonicalIndex(cell)
		if !ok || got != index {
			t.Fatalf("CanonicalIndex(%d) = %d/%v, want %d/true", cell, got, ok, index)
		}
	}
	for _, excluded := range []int{0, 2} {
		if _, ok := restricted.CanonicalIndex(excluded); ok ||
			restricted.CoversInfluence(excluded, excluded) ||
			restricted.CoversEmission(excluded, excluded) ||
			restricted.CoversInfluence(excluded, 3) ||
			restricted.CoversEmission(3, excluded) {
			t.Fatalf("excluded cell %d retained authority in restricted plan", excluded)
		}
	}
	var calls []int
	result, solveErr := SolveWTOContext(context.Background(), EquationSystem[int, int]{
		Lattice: capLattice{top: 8}.joinOnly(), Cells: []int{1, 3},
		Evaluate: func(cell int, _ func(int) int) int {
			calls = append(calls, cell)
			return cell
		},
	}, restricted)
	if solveErr != nil || !reflect.DeepEqual(calls, []int{1, 3}) || !reflect.DeepEqual(result, map[int]int{1: 1, 3: 3}) {
		t.Fatalf("noncontiguous execution calls=%v result=%v error=%v", calls, result, solveErr)
	}
}

func TestRestrictWTOPlanRejectsExcludedDemandFromCurrentView(t *testing.T) {
	plan := NewWTOPlan([]int{0, 1, 2}, nil)
	restricted, err := RestrictWTOPlan(plan, []int{2})
	if err != nil {
		t.Fatal(err)
	}
	for _, demand := range [][]int{{0}, {2, 0}} {
		got, restrictErr := RestrictWTOPlan(restricted, demand)
		if !errors.Is(restrictErr, ErrWTOPlanRestrictionUncovered) || got != nil {
			t.Fatalf("demand %v produced plan=%v error=%v, want nil/uncovered", demand, got, restrictErr)
		}
	}
}

func BenchmarkRestrictWTOPlanNested(b *testing.B) {
	const count = 256
	cells := make([]int, count)
	for index := range cells {
		cells[index] = index
	}
	plan := NewWTOPlan(cells, func(cell int) []int {
		if cell+1 < count {
			return []int{cell + 1}
		}
		backedges := make([]int, count-1)
		for index := range backedges {
			backedges[index] = index
		}
		return backedges
	})
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		if _, err := RestrictWTOPlan(plan, []int{count - 1}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRestrictedWTOPlanMembershipManyRoots(b *testing.B) {
	const count = 4_096
	cells := make([]int, count)
	for index := range cells {
		cells[index] = index
	}
	plan := NewWTOPlan(cells, nil)
	restricted, err := RestrictWTOPlan(plan, cells)
	if err != nil {
		b.Fatal(err)
	}
	cell := count / 2
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		if index, ok := restricted.CanonicalIndex(cell); !ok || index < 0 {
			b.Fatal("retained cell is absent")
		}
		if !restricted.CoversInfluence(cell, cell) {
			b.Fatal("retained self read is absent")
		}
	}
}

func TestRestrictWTOPlanAllocationCountDoesNotGrowWithNestingDepth(t *testing.T) {
	makePlan := func(count int) *WTOPlan[int] {
		cells := make([]int, count)
		for index := range cells {
			cells[index] = index
		}
		return NewWTOPlan(cells, func(cell int) []int {
			if cell+1 < count {
				return []int{cell + 1}
			}
			backedges := make([]int, count-1)
			for index := range backedges {
				backedges[index] = index
			}
			return backedges
		})
	}
	small, large := makePlan(64), makePlan(1_024)
	smallAllocs := testing.AllocsPerRun(20, func() {
		if _, err := RestrictWTOPlan(small, []int{63}); err != nil {
			panic(err)
		}
	})
	largeAllocs := testing.AllocsPerRun(20, func() {
		if _, err := RestrictWTOPlan(large, []int{1_023}); err != nil {
			panic(err)
		}
	})
	if largeAllocs > smallAllocs*2 {
		t.Fatalf("restriction allocations grew with nesting: small=%f large=%f", smallAllocs, largeAllocs)
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

func TestSolveWTOFlatScheduleReplaysNestedComponentsExactly(t *testing.T) {
	edges := map[int][]int{0: {1}, 1: {2}, 2: {1, 0}}
	plan := NewWTOPlan([]int{0, 1, 2}, func(cell int) []int { return edges[cell] })
	calls := make([]int, 0)
	result, err := SolveWTOContext(context.Background(), EquationSystem[int, int]{
		Lattice: capLattice{top: 2}.joinOnly(),
		Cells:   []int{0, 1, 2},
		Initial: func(int) int { return 0 },
		Evaluate: func(cell int, read func(int) int) int {
			calls = append(calls, cell)
			switch cell {
			case 0:
				return min(2, read(2)+1)
			case 1:
				return min(2, read(1)+1)
			default:
				return read(1)
			}
		},
	}, plan)
	if err != nil {
		t.Fatal(err)
	}
	wantCalls := []int{0, 1, 2, 1, 2, 1, 2, 0, 1, 2, 0, 1, 2}
	if !reflect.DeepEqual(calls, wantCalls) || !reflect.DeepEqual(result, map[int]int{0: 2, 1: 2, 2: 2}) {
		t.Fatalf("calls=%v result=%v, want calls=%v and all cells at 2", calls, result, wantCalls)
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

func TestSolveWTONarrowingUsesFrozenReadAudit(t *testing.T) {
	t.Run("excluded", func(t *testing.T) {
		full := NewWTOPlan([]int{0, 1}, func(cell int) []int {
			if cell == 0 {
				return []int{0}
			}
			return nil
		})
		plan, err := RestrictWTOPlan(full, []int{0})
		if err != nil {
			t.Fatal(err)
		}
		domain := capLattice{top: 8}.lattice()
		domain.Narrow = mini
		calls := 0
		result, solveErr := SolveWTOContext(context.Background(), EquationSystem[int, int]{
			Lattice: domain, Cells: []int{0}, WidenAt: plan.IsComponentHead,
			Evaluate: func(_ int, read func(int) int) int {
				calls++
				if calls >= 3 {
					return read(1)
				}
				return 1
			},
		}, plan)
		if !errors.Is(solveErr, ErrWTOPlanUncovered) || result != nil {
			t.Fatalf("result=%v error=%v calls=%d, want nil/uncovered during narrowing", result, solveErr, calls)
		}
	})

	t.Run("backward", func(t *testing.T) {
		plan := NewWTOPlan([]int{0, 1}, nil)
		domain := capLattice{top: 8}.lattice()
		domain.Narrow = mini
		calls := [2]int{}
		result, err := SolveWTOContext(context.Background(), EquationSystem[int, int]{
			Lattice: domain, Cells: []int{0, 1}, WidenAt: func(cell int) bool { return cell == 0 },
			Evaluate: func(cell int, read func(int) int) int {
				calls[cell]++
				if cell == 0 && calls[cell] > 1 {
					return read(1)
				}
				return 1
			},
		}, plan)
		if !errors.Is(err, ErrWTOPlanUncovered) || result != nil {
			t.Fatalf("result=%v error=%v calls=%v, want nil/uncovered during narrowing", result, err, calls)
		}
	})

	t.Run("self", func(t *testing.T) {
		plan := NewWTOPlan([]int{0}, nil)
		domain := capLattice{top: 8}.lattice()
		domain.Narrow = mini
		calls := 0
		result, err := SolveWTOContext(context.Background(), EquationSystem[int, int]{
			Lattice: domain, Cells: []int{0}, WidenAt: func(int) bool { return true },
			Evaluate: func(cell int, read func(int) int) int {
				calls++
				if calls > 1 {
					return read(cell)
				}
				return 1
			},
		}, plan)
		if !errors.Is(err, ErrWTOPlanUncovered) || result != nil {
			t.Fatalf("result=%v error=%v calls=%d, want nil/uncovered during narrowing", result, err, calls)
		}
	})
}

func TestSolveWTONarrowingUsesFrozenEmissionAudit(t *testing.T) {
	full := NewWTOPlan([]int{0, 1}, func(cell int) []int {
		if cell == 0 {
			return []int{0}
		}
		return nil
	})
	plan, err := RestrictWTOPlan(full, []int{0})
	if err != nil {
		t.Fatal(err)
	}
	domain := capLattice{top: 8}.lattice()
	domain.Narrow = mini
	calls := 0
	result, solveErr := SolveWTOContext(context.Background(), EquationSystem[int, int]{
		Lattice: domain, Cells: []int{0}, WidenAt: plan.IsComponentHead,
		Transfer: func(_ int, _ func(int) int, emit func(int, int)) {
			calls++
			emit(0, 1)
			if calls >= 3 {
				emit(1, 1)
			}
		},
	}, plan)
	if !errors.Is(solveErr, ErrWTOPlanUncovered) || result != nil {
		t.Fatalf("result=%v error=%v calls=%d, want nil/uncovered during narrowing", result, solveErr, calls)
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

func TestSolveWTOCancellationDuringNarrowingPublishesNothing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	domain := capLattice{top: 8}.lattice()
	domain.Narrow = mini
	calls := 0
	sys := EquationSystem[int, int]{
		Lattice: domain, Cells: []int{0}, WidenAt: func(int) bool { return true },
		Evaluate: func(_ int, _ func(int) int) int {
			calls++
			if calls > 1 {
				cancel()
			}
			return 1
		},
	}
	result, err := SolveWTOContext(ctx, sys, NewWTOPlan(sys.Cells, nil))
	if !errors.Is(err, ErrCanceled) || !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want cancellation during narrowing", err)
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
		if err != nil || !reflect.DeepEqual(plan.Elements(), secondPlan.Elements()) || !reflect.DeepEqual(wto, second) {
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
	elements := plan.Elements()
	if len(elements) != 1 || !elements[0].IsComponent() || elements[0].Vertex == 1 {
		t.Fatalf("plan = %#v, want component headed away from widening cell 1", elements)
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
					if plan.core.rank[other] < plan.core.rank[cell] {
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
				if plan.core.rank[other] < plan.core.rank[cell] {
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
