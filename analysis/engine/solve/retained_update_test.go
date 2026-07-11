package solve

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
)

func TestRetainedUpdateConcurrentBeginHasSingleWinner(t *testing.T) {
	outputs := map[int]map[int]int{}
	sys := constantSystem([]int{0}, &outputs)
	retained, _ := buildRetainedInts(t, sys, nil)
	start := make(chan struct{})
	results := make(chan error, 2)
	updates := make(chan *Update[int, int], 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			update, err := retained.BeginUpdate([]int{0}, nil, nil)
			results <- err
			updates <- update
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(updates)
	winners, activeErrors := 0, 0
	for err := range results {
		if err == nil {
			winners++
		} else if errors.Is(err, ErrUpdateActive) {
			activeErrors++
		} else {
			t.Fatalf("BeginUpdate error=%v", err)
		}
	}
	for update := range updates {
		if update != nil {
			update.Abort()
		}
	}
	if winners != 1 || activeErrors != 1 {
		t.Fatalf("winners=%d activeErrors=%d", winners, activeErrors)
	}
}

func TestRetainedUpdateReplacesStatsTransactionally(t *testing.T) {
	oldStats, nextStats := &Stats{}, &Stats{}
	outputs := map[int]map[int]int{0: {1: 2}}
	sys := constantSystem([]int{0, 1}, &outputs)
	sys.Stats = oldStats
	retained, _ := buildRetainedInts(t, sys, map[int][]int{0: {1}})
	oldCalls := oldStats.TransferCalls
	u, err := retained.BeginUpdate([]int{0}, sys.Transfer, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := u.SetStats(nextStats); err != nil {
		t.Fatal(err)
	}
	if err := u.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := u.Publish(context.Background()); err != nil {
		t.Fatal(err)
	}
	if oldStats.TransferCalls != oldCalls || nextStats.TransferCalls == 0 {
		t.Fatalf("old=%d/%d replacement=%d", oldCalls, oldStats.TransferCalls, nextStats.TransferCalls)
	}
	if err := u.Commit(); err != nil {
		t.Fatal(err)
	}
}

func buildRetainedInts(t *testing.T, sys EquationSystem[int, int], edges map[int][]int) (*RetainedSystem[int, int], *WTOPlan[int]) {
	t.Helper()
	plan := NewWTOPlan(sys.Cells, func(cell int) []int { return edges[cell] })
	_, _, retained, err := BuildRetainedWTO(context.Background(), sys, plan, RetainedBudget{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(retained.Release)
	return retained, plan
}

func updateAndCompareInts(t *testing.T, retained *RetainedSystem[int, int], changed []int, transfer Transfer[int, int], clean EquationSystem[int, int], plan *WTOPlan[int]) map[int]int {
	t.Helper()
	u, err := retained.BeginUpdate(changed, transfer, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = u.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, _, err := u.Publish(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want, err := SolveWTO(clean, plan)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("regional=%v clean=%v", got, want)
	}
	if err := u.Commit(); err != nil {
		t.Fatal(err)
	}
	return got
}

func constantSystem(cells []int, values *map[int]map[int]int) EquationSystem[int, int] {
	return EquationSystem[int, int]{Lattice: capLattice{top: 64}.joinOnly(), Cells: cells, Transfer: func(cell int, _ func(int) int, emit func(int, int)) {
		for destination, value := range (*values)[cell] {
			emit(destination, value)
		}
	}}
}

func TestRetainedUpdateOwnerContributionDecreases(t *testing.T) {
	values := map[int]map[int]int{0: {1: 9}}
	edges := map[int][]int{0: {1}}
	sys := constantSystem([]int{0, 1}, &values)
	retained, plan := buildRetainedInts(t, sys, edges)
	values[0][1] = 3
	updateAndCompareInts(t, retained, []int{0}, sys.Transfer, sys, plan)
}

func TestRetainedUpdateOwnerStopsEmitting(t *testing.T) {
	values := map[int]map[int]int{0: {1: 9}}
	edges := map[int][]int{0: {1}}
	sys := constantSystem([]int{0, 1}, &values)
	retained, plan := buildRetainedInts(t, sys, edges)
	delete(values[0], 1)
	got := updateAndCompareInts(t, retained, []int{0}, sys.Transfer, sys, plan)
	if got[1] != 0 {
		t.Fatalf("stale contribution: %v", got)
	}
}

func TestRetainedUpdateReplacesTwoDestinationsAtomically(t *testing.T) {
	values := map[int]map[int]int{0: {1: 8, 2: 2}}
	edges := map[int][]int{0: {1, 2}}
	sys := constantSystem([]int{0, 1, 2}, &values)
	retained, plan := buildRetainedInts(t, sys, edges)
	values[0] = map[int]int{1: 1, 2: 7}
	updateAndCompareInts(t, retained, []int{0}, sys.Transfer, sys, plan)
}

func TestRetainedUpdateChangedOwnerInsideNestedSCC(t *testing.T) {
	edges := map[int][]int{0: {1}, 1: {2}, 2: {1, 3}, 3: {2, 4}}
	bias := 1
	sys := EquationSystem[int, int]{Lattice: capLattice{top: 12}.joinOnly(), Cells: []int{0, 1, 2, 3, 4}, InitialSparse: func(c int) (int, bool) { return 1, c == 0 }, Transfer: func(cell int, read func(int) int, emit func(int, int)) {
		for _, to := range edges[cell] {
			emit(to, min(12, read(cell)+bias))
		}
	}}
	retained, plan := buildRetainedInts(t, sys, edges)
	bias = 2
	updateAndCompareInts(t, retained, []int{2}, sys.Transfer, sys, plan)
}

func TestRetainedUpdateNewForwardEdgeExpandsRegion(t *testing.T) {
	edges := map[int][]int{0: {1}}
	outputs := map[int]map[int]int{0: {1: 2}}
	sys := constantSystem([]int{0, 1, 2}, &outputs)
	retained, plan := buildRetainedInts(t, sys, edges)
	// The canonical WTO ranks the disconnected 2 before the 0 -> 1 chain, so
	// 2 -> 0 is a strict-forward dynamic emission.
	outputs[2] = map[int]int{0: 6}
	updateAndCompareInts(t, retained, []int{2}, sys.Transfer, sys, plan)
}

func TestRetainedUpdateNewBackwardEdgeForcesFullFallback(t *testing.T) {
	edges := map[int][]int{}
	outputs := map[int]map[int]int{}
	sys := constantSystem([]int{0, 1}, &outputs)
	retained, oldPlan := buildRetainedInts(t, sys, edges)
	from, to := 0, 1
	if oldPlan.rank[from] < oldPlan.rank[to] {
		from, to = to, from
	}
	outputs[from] = map[int]int{to: 5}
	u, err := retained.BeginUpdate([]int{from}, sys.Transfer, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := u.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, _, err := u.Publish(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cleanPlan := NewWTOPlan(sys.Cells, func(cell int) []int {
		if cell == from {
			return []int{to}
		}
		return nil
	})
	want, err := SolveWTO(sys, cleanPlan)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("fallback=%v clean=%v err=%v", got, want, err)
	}
	if u.scratch.plan == retained.plan {
		t.Fatal("backward edge did not rebuild WTO")
	}
	u.Abort()
}

func TestRetainedUpdateRemovedDynamicReadRemovesReverseEdge(t *testing.T) {
	edges := map[int][]int{0: {1}}
	reads := true
	sys := EquationSystem[int, int]{Lattice: capLattice{top: 8}.joinOnly(), Cells: []int{0, 1}, InitialSparse: func(c int) (int, bool) { return 4, c == 0 }, Transfer: func(cell int, read func(int) int, emit func(int, int)) {
		if cell == 1 && reads {
			emit(1, read(0))
		}
	}}
	// The transfer emits to its own declared equation, so conservatively plan
	// that recurrence even though the emitted value only reads cell 0.
	edges[1] = []int{1}
	retained, plan := buildRetainedInts(t, sys, edges)
	reads = false
	updateAndCompareInts(t, retained, []int{1}, sys.Transfer, sys, plan)
	for _, reverse := range retained.readers {
		if reverse.cell == 0 {
			for _, owner := range reverse.owners {
				if owner == 1 {
					t.Fatal("removed read retained")
				}
			}
		}
	}
}

func TestRetainedUpdateResetsWideningHistory(t *testing.T) {
	edges := map[int][]int{0: {0, 1}}
	step := 1
	sys := EquationSystem[int, int]{Lattice: capLattice{top: 10}.lattice(), Cells: []int{0, 1}, InitialSparse: func(c int) (int, bool) { return 1, c == 0 }, WidenAt: func(c int) bool { return c == 0 }, WidenDelay: func(int) int { return 1 }, Transfer: func(cell int, read func(int) int, emit func(int, int)) {
		if cell == 0 {
			emit(0, min(10, read(0)+step))
			emit(1, read(0))
		}
	}}
	retained, plan := buildRetainedInts(t, sys, edges)
	oldVisits := retained.VisitCount(0)
	step = 2
	updateAndCompareInts(t, retained, []int{0}, sys.Transfer, sys, plan)
	if retained.VisitCount(0) >= oldVisits*2 {
		t.Fatalf("history appears accumulated: old=%d new=%d", oldVisits, retained.VisitCount(0))
	}
}

func TestRetainedUpdateNarrowingCreatesEmittedOnlyCell(t *testing.T) {
	edges := map[int][]int{0: {1}, 1: {0}}
	domain := capLattice{top: 8}.lattice()
	domain.Narrow = mini
	sys := EquationSystem[int, int]{Lattice: domain, Cells: []int{0, 1}, WidenAt: func(cell int) bool { return cell == 0 }, Transfer: func(cell int, read func(int) int, emit func(int, int)) {
		if cell != 1 {
			return
		}
		if read(0) == 3 {
			emit(9, 3)
		}
		emit(0, 3)
	}}
	retained, plan := buildRetainedInts(t, sys, edges)
	if _, exists := retained.Value(9); exists {
		t.Fatal("cell 9 unexpectedly exists in pre-narrowing ascent")
	}
	u, _ := retained.BeginUpdate([]int{1}, nil, nil)
	if err := u.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, exists := u.scratch.Value(9); exists {
		t.Fatal("regional ascent created narrowing-only cell 9")
	}
	got, versions, err := u.Publish(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := got[9]; !exists || versions[9] == 0 {
		t.Fatalf("narrowing-only publication missing: values=%v versions=%v", got, versions)
	}
	_, _, cleanRetained, err := BuildRetainedWTO(context.Background(), sys, plan, RetainedBudget{})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanRetained.Release()
	cleanScratch := stateFromRetained(cleanRetained)
	if err := cleanScratch.runNarrowing(nil); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, cleanScratch.cur) {
		t.Fatalf("regional=%v clean=%v", got, cleanScratch.cur)
	}
	u.Abort()
}

func TestRetainedUpdateCancellationPreservesCheckpoint(t *testing.T) {
	edges := map[int][]int{0: {1}}
	outputs := map[int]map[int]int{0: {1: 2}}
	sys := constantSystem([]int{0, 1}, &outputs)
	retained, _ := buildRetainedInts(t, sys, edges)
	before, beforeOwners := cloneMap(retained.values), cloneOwners(retained.owners)
	ctx, cancel := context.WithCancel(context.Background())
	replacement := Transfer[int, int](func(cell int, _ func(int) int, emit func(int, int)) {
		if cell == 0 {
			emit(1, 7)
			cancel()
		}
	})
	u, _ := retained.BeginUpdate([]int{0}, replacement, nil)
	if err := u.Run(ctx); !errors.Is(err, ErrCanceled) {
		t.Fatalf("error=%v", err)
	}
	u.Abort()
	if !reflect.DeepEqual(before, retained.values) || !reflect.DeepEqual(beforeOwners, retained.owners) {
		t.Fatalf("cancellation after emit mutated checkpoint: before=%v after=%v", before, retained.values)
	}
}

func TestRetainedUpdateUnownedExternalReadForcesFullFallback(t *testing.T) {
	outputs := map[int]map[int]int{0: {1: 2}}
	sys := constantSystem([]int{0, 1}, &outputs)
	retained, _ := buildRetainedInts(t, sys, map[int][]int{0: {1}})
	outputs[0][1] = 4
	u, _ := retained.BeginUpdate([]int{0}, sys.Transfer, nil)
	u.RequireFullFallback()
	if err := u.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if u.scratch == nil || u.scratch == retained {
		t.Fatal("full fallback did not remain transactional")
	}
	got, _, err := u.Publish(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want, err := SolveWTO(sys, retained.plan)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("fallback=%v clean=%v err=%v", got, want, err)
	}
	u.Abort()
}

func TestRetainedUpdateReplacementWithEmptyChangedSetRunsFull(t *testing.T) {
	outputs := map[int]map[int]int{0: {1: 2}}
	sys := constantSystem([]int{0, 1}, &outputs)
	retained, plan := buildRetainedInts(t, sys, map[int][]int{0: {1}})
	outputs[0][1] = 6
	u, err := retained.BeginUpdate(nil, sys.Transfer, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := u.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, _, err := u.Publish(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want, err := SolveWTO(sys, plan)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("full=%v clean=%v err=%v", got, want, err)
	}
	u.Abort()
}

func TestRetainedUpdateRepeatedUpdatesBoundedAndDeterministic(t *testing.T) {
	outputs := map[int]map[int]int{0: {1: 1}}
	edges := map[int][]int{0: {1}}
	sys := constantSystem([]int{0, 1}, &outputs)
	retained, plan := buildRetainedInts(t, sys, edges)
	baseline := retained.Usage()
	for i := 0; i < 32; i++ {
		outputs[0][1] = 1 + i%3
		updateAndCompareInts(t, retained, []int{0}, sys.Transfer, sys, plan)
		if got := retained.Usage(); got.Owners != baseline.Owners || got.Outputs != baseline.Outputs || got.Reads != baseline.Reads {
			t.Fatalf("usage grew: baseline=%+v got=%+v", baseline, got)
		}
	}
	final := retained.values[1]
	outputs[0][1] = final
	updateAndCompareInts(t, retained, []int{0}, sys.Transfer, sys, plan)
	if retained.values[1] != final {
		t.Fatalf("nondeterministic final=%d got=%d", final, retained.values[1])
	}
}
