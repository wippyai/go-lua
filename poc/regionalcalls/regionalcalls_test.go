package regionalcalls

import (
	"context"
	"math/rand"
	"reflect"
	"sort"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/engine/solve"
)

type facts uint8

const (
	factKnown facts = 1 << iota
	factOther
	factUnknown = factKnown | factOther
)

func factsDomain() lattice.Lattice[facts] {
	join := func(a, b facts) facts { return a | b }
	return lattice.Lattice[facts]{
		Bottom:   func() facts { return 0 },
		Top:      func() facts { return 0xff },
		Equal:    func(a, b facts) bool { return a == b },
		LessOrEq: func(a, b facts) bool { return a&b == a },
		Join:     join,
		Widen:    join,
	}
}

func TestReplacementClosesOldUnknownToKnownPrecisionHole(t *testing.T) {
	binding := factUnknown
	cells := []int{0, 1, 2}
	transfer := func(owner int, read func(int) facts, emit func(int, facts)) {
		switch owner {
		case 0:
			emit(1, binding)
		case 1:
			emit(2, read(1))
		}
	}
	system := solve.EquationSystem[int, facts]{Lattice: factsDomain(), Cells: cells, Transfer: transfer}
	plan := solve.NewWTOPlan(cells, func(cell int) []int {
		switch cell {
		case 0:
			return []int{1}
		case 1:
			return []int{1, 2}
		default:
			return nil
		}
	})
	old, _, retained, err := solve.BuildRetainedWTO(context.Background(), system, plan, solve.RetainedBudget{})
	if err != nil {
		t.Fatal(err)
	}
	defer retained.Release()
	if old[2] != factUnknown {
		t.Fatalf("old exit=%02b want unknown", old[2])
	}

	binding = factKnown // non-monotone replacement of owner 0's contribution
	update, err := retained.BeginUpdate([]int{0}, transfer, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer update.Abort()
	if err := update.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	regional, _, err := update.Publish(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	clean, err := solve.SolveWTO(system, plan)
	if err != nil {
		t.Fatal(err)
	}
	if regional[2] != factKnown || !reflect.DeepEqual(regional, clean) {
		t.Fatalf("regional=%v clean=%v", regional, clean)
	}
	// The old hybrid resume operation is exactly this join. It cannot recover.
	if resumed := factsDomain().Join(old[2], factKnown); resumed != factUnknown {
		t.Fatalf("resume unexpectedly recovered precision: %02b", resumed)
	}
	if retainedExit, _ := retained.Value(2); retainedExit != factUnknown {
		t.Fatal("uncommitted regional generation leaked into the base")
	}
	if err := update.Commit(); err != nil {
		t.Fatal(err)
	}
	if committed, _ := retained.Value(2); committed != factKnown {
		t.Fatalf("committed exit=%02b want known", committed)
	}
}

func TestRandomReplacementDifferentialAgainstCleanWTO(t *testing.T) {
	for seed := int64(0); seed < 500; seed++ {
		rng := rand.New(rand.NewSource(seed))
		n := 2 + rng.Intn(7)
		cells := make([]int, n)
		reads := make([][]int, n)
		emits := make([][]int, n)
		base := make([]facts, n)
		for owner := range n {
			cells[owner] = owner
			base[owner] = facts(1 << rng.Intn(2))
			for other := range n {
				if rng.Intn(5) == 0 {
					reads[owner] = append(reads[owner], other)
				}
				if rng.Intn(4) == 0 {
					emits[owner] = append(emits[owner], other)
				}
			}
		}
		transfer := func(owner int, read func(int) facts, emit func(int, facts)) {
			value := base[owner]
			for _, dependency := range reads[owner] {
				value |= read(dependency)
			}
			for _, destination := range emits[owner] {
				emit(destination, value)
			}
		}
		influences := influenceGraph(n, reads, emits)
		plan := solve.NewWTOPlan(cells, func(cell int) []int { return influences[cell] })
		system := solve.EquationSystem[int, facts]{Lattice: factsDomain(), Cells: cells, Transfer: transfer}
		_, _, retained, err := solve.BuildRetainedWTO(context.Background(), system, plan, solve.RetainedBudget{})
		if err != nil {
			t.Fatalf("seed %d build: %v", seed, err)
		}
		changed := rng.Intn(n)
		// Arbitrary replacement: it may grow, shrink, or become Bottom.
		base[changed] = facts(rng.Intn(4))
		regionalStats := &solve.Stats{}
		update, err := retained.BeginUpdate([]int{changed}, transfer, nil)
		if err == nil {
			err = update.SetStats(regionalStats)
		}
		if err == nil {
			err = update.Run(context.Background())
		}
		var regional map[int]facts
		if err == nil {
			regional, _, err = update.Publish(context.Background())
		}
		cleanStats := &solve.Stats{}
		system.Stats = cleanStats
		clean, cleanErr := solve.SolveWTO(system, plan)
		if err != nil || cleanErr != nil || !reflect.DeepEqual(regional, clean) {
			t.Fatalf("seed=%d changed=%d regional=%v clean=%v err=%v cleanErr=%v reads=%v emits=%v base=%v", seed, changed, regional, clean, err, cleanErr, reads, emits, base)
		}
		if regionalStats.TransferCalls == 0 || cleanStats.TransferCalls == 0 {
			t.Fatalf("seed %d missing cone/full work counters", seed)
		}
		update.Abort()
		retained.Release()
	}
}

func influenceGraph(n int, reads, emits [][]int) map[int][]int {
	sets := make(map[int]map[int]struct{}, n)
	add := func(from, to int) {
		if sets[from] == nil {
			sets[from] = make(map[int]struct{})
		}
		sets[from][to] = struct{}{}
	}
	for owner := range n {
		for _, dependency := range reads[owner] {
			add(dependency, owner)
		}
		for _, destination := range emits[owner] {
			add(owner, destination)
		}
	}
	out := make(map[int][]int, n)
	for from, destinations := range sets {
		for to := range destinations {
			out[from] = append(out[from], to)
		}
		sort.Ints(out[from])
	}
	return out
}
