package solve

import (
	"context"
	"math/rand"
	"reflect"
	"sync"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/lattice"
)

// TestRetainedUpdateAuditRandomDifferential is an audit-only randomized
// differential check. Keep it deterministic so a discovered seed reproduces.
func TestRetainedUpdateAuditRandomDifferential(t *testing.T) {
	for seed := int64(0); seed < 1000; seed++ {
		rng := rand.New(rand.NewSource(seed))
		n := 1 + rng.Intn(7)
		cells := make([]int, n)
		for i := range cells {
			cells[i] = i
		}
		edges := make(map[int][]int)
		weights := make([][]int, n)
		reads := make([][]bool, n)
		for from := 0; from < n; from++ {
			weights[from] = make([]int, n)
			reads[from] = make([]bool, n)
			for to := 0; to < n; to++ {
				if rng.Intn(4) == 0 {
					edges[from] = append(edges[from], to)
					reads[from][to] = true
					weights[from][to] = rng.Intn(4)
				}
			}
		}
		initial := make([]int, n)
		for i := range initial {
			initial[i] = rng.Intn(3)
		}
		sys := EquationSystem[int, int]{
			Lattice: capLattice{top: 15}.joinOnly(), Cells: cells,
			InitialSparse: func(c int) (int, bool) { return initial[c], initial[c] != 0 },
			Transfer: func(owner int, read func(int) int, emit func(int, int)) {
				for destination := 0; destination < n; destination++ {
					if reads[owner][destination] {
						emit(destination, min(15, read(owner)+weights[owner][destination]))
					}
				}
			},
		}
		plan := NewWTOPlan(cells, func(c int) []int { return edges[c] })
		_, _, retained, err := BuildRetainedWTO(context.Background(), sys, plan, RetainedBudget{})
		if err != nil {
			t.Fatalf("seed %d build: %v", seed, err)
		}

		changed := rng.Intn(n)
		for destination := 0; destination < n; destination++ {
			if reads[changed][destination] && rng.Intn(2) == 0 {
				weights[changed][destination] = rng.Intn(4)
			}
		}
		u, err := retained.BeginUpdate([]int{changed}, sys.Transfer, nil)
		if err == nil {
			err = u.Run(context.Background())
		}
		var got map[int]int
		if err == nil {
			got, _, err = u.Publish(context.Background())
		}
		want, cleanErr := SolveWTO(sys, plan)
		if err != nil || cleanErr != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("seed=%d changed=%d regional=%v clean=%v err=%v cleanErr=%v edges=%v weights=%v", seed, changed, got, want, err, cleanErr, edges, weights)
		}
		u.Abort()
		retained.Release()
	}
}

func TestRetainedUpdateConcurrentReadersObserveWholeGenerations(t *testing.T) {
	outputs := map[int]map[int]int{0: {1: 4}}
	sys := constantSystem([]int{0, 1}, &outputs)
	retained, _ := buildRetainedInts(t, sys, map[int][]int{0: {1}})
	stop := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_, _ = retained.Value(1)
					_ = retained.Version(1)
					_ = retained.Usage()
				}
			}
		}()
	}
	for i := 0; i < 50; i++ {
		u, err := retained.BeginUpdate([]int{0}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := u.Run(context.Background()); err != nil {
			t.Fatal(err)
		}
		if _, _, err := u.Publish(context.Background()); err != nil {
			t.Fatal(err)
		}
		if err := u.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	close(stop)
	wg.Wait()
}

type retainedBenchState struct{ words [4]uint64 }

func retainedBenchLattice() lattice.Lattice[retainedBenchState] {
	join := func(a, b retainedBenchState) retainedBenchState {
		for i := range a.words {
			a.words[i] |= b.words[i]
		}
		return a
	}
	return lattice.Lattice[retainedBenchState]{
		Bottom: func() retainedBenchState { return retainedBenchState{} },
		Equal:  func(a, b retainedBenchState) bool { return a == b },
		Join:   join,
		Widen:  join,
	}
}

func BenchmarkRetainedSparse500(b *testing.B) {
	const n = 500
	cells := make([]int, n)
	facts := make([]retainedBenchState, n)
	for i := range cells {
		cells[i] = i
		facts[i].words[(i/64)%len(facts[i].words)] = 1 << uint(i%64)
	}
	sys := EquationSystem[int, retainedBenchState]{
		Lattice: retainedBenchLattice(), Cells: cells,
		Transfer: func(cell int, _ func(int) retainedBenchState, emit func(int, retainedBenchState)) {
			if cell+1 < n {
				emit(cell+1, facts[cell])
			}
		},
	}
	plan := NewWTOPlan(cells, func(cell int) []int {
		if cell+1 < n {
			return []int{cell + 1}
		}
		return nil
	})
	b.Run("clean", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			facts[n-2].words[0] = uint64(i&1) + 1
			if _, err := SolveWTO(sys, plan); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("regional", func(b *testing.B) {
		_, _, retained, err := BuildRetainedWTO(context.Background(), sys, plan, RetainedBudget{})
		if err != nil {
			b.Fatal(err)
		}
		defer retained.Release()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			facts[n-2].words[0] = uint64(i&1) + 1
			u, err := retained.BeginUpdate([]int{n - 2}, nil, nil)
			if err == nil {
				err = u.Run(context.Background())
			}
			if err == nil {
				_, _, err = u.Publish(context.Background())
			}
			if err == nil {
				err = u.Commit()
			}
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

func TestRetainedUpdateAuditRandomDynamicEdges(t *testing.T) {
	for seed := int64(0); seed < 2000; seed++ {
		rng := rand.New(rand.NewSource(seed + 10000))
		n := 2 + rng.Intn(6)
		cells := make([]int, n)
		for i := range cells {
			cells[i] = i
		}
		edges := make([][]bool, n)
		weights := make([][]int, n)
		for from := 0; from < n; from++ {
			edges[from], weights[from] = make([]bool, n), make([]int, n)
			for to := 0; to < n; to++ {
				edges[from][to] = rng.Intn(5) == 0
				weights[from][to] = 1 + rng.Intn(3)
			}
		}
		successors := func(c int) []int {
			var out []int
			for to := 0; to < n; to++ {
				if edges[c][to] {
					out = append(out, to)
				}
			}
			return out
		}
		sys := EquationSystem[int, int]{
			Lattice: capLattice{top: 12}.joinOnly(), Cells: cells,
			InitialSparse: func(c int) (int, bool) { return 1, c == 0 },
			Transfer: func(owner int, read func(int) int, emit func(int, int)) {
				for destination := 0; destination < n; destination++ {
					if edges[owner][destination] {
						emit(destination, min(12, read(owner)+weights[owner][destination]))
					}
				}
			},
		}
		oldPlan := NewWTOPlan(cells, successors)
		_, _, retained, err := BuildRetainedWTO(context.Background(), sys, oldPlan, RetainedBudget{})
		if err != nil {
			t.Fatalf("seed %d build: %v", seed, err)
		}
		changed := rng.Intn(n)
		to := rng.Intn(n)
		edges[changed][to] = !edges[changed][to]
		u, err := retained.BeginUpdate([]int{changed}, sys.Transfer, nil)
		if err == nil {
			err = u.Run(context.Background())
		}
		var got map[int]int
		if err == nil {
			got, _, err = u.Publish(context.Background())
		}
		newPlan := NewWTOPlan(cells, successors)
		want, cleanErr := SolveWTO(sys, newPlan)
		if err != nil || cleanErr != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("seed=%d changed=%d to=%d regional=%v clean=%v err=%v cleanErr=%v", seed, changed, to, got, want, err, cleanErr)
		}
		u.Abort()
		retained.Release()
	}
}

func TestRetainedUpdateNewSelfEdgeRebuildsPlan(t *testing.T) {
	edges := map[int][]int{}
	self := false
	sys := EquationSystem[int, int]{
		Lattice:       capLattice{top: 12}.joinOnly(),
		Cells:         []int{0},
		InitialSparse: func(c int) (int, bool) { return 1, c == 0 },
		Transfer: func(cell int, read func(int) int, emit func(int, int)) {
			if self {
				emit(0, min(12, read(0)+2))
			}
		},
	}
	retained, oldPlan := buildRetainedInts(t, sys, edges)
	self = true
	u, err := retained.BeginUpdate([]int{0}, sys.Transfer, nil)
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
	cleanPlan := NewWTOPlan(sys.Cells, func(int) []int { return []int{0} })
	want, err := SolveWTO(sys, cleanPlan)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("regional=%v clean=%v err=%v", got, want, err)
	}
	if u.scratch.plan == oldPlan {
		t.Fatal("new self edge did not rebuild WTO plan")
	}
	u.Abort()
}

func TestSolveWTODynamicSelfEmissionRequiresRebuiltPlan(t *testing.T) {
	sys := EquationSystem[int, int]{
		Lattice:       capLattice{top: 12}.joinOnly(),
		Cells:         []int{0},
		InitialSparse: func(c int) (int, bool) { return 1, c == 0 },
		Transfer: func(_ int, read func(int) int, emit func(int, int)) {
			emit(0, min(12, read(0)+2))
		},
	}
	uncovered := NewWTOPlan(sys.Cells, func(int) []int { return nil })
	if _, err := SolveWTO(sys, uncovered); err != ErrWTOPlanUncovered {
		t.Fatalf("error=%v, want %v", err, ErrWTOPlanUncovered)
	}
	covered := NewWTOPlan(sys.Cells, func(int) []int { return []int{0} })
	got, err := SolveWTO(sys, covered)
	if err != nil || got[0] != 12 {
		t.Fatalf("covered=%v err=%v", got, err)
	}
}
