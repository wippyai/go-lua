package region

import (
	"context"
	"errors"
	"maps"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/engine/solve"
)

type graphSystem struct {
	cells      []int
	successors map[int][]int
	initial    map[int]int
	widenAt    map[int]bool
	widenDelay int
	transfer   func(int, func(int) int, func(int, int))
}

func TestRegionDifferentialAcyclic(t *testing.T) {
	system := graphSystem{
		cells:      []int{0, 1, 2, 3},
		successors: map[int][]int{0: {1, 2}, 1: {3}, 2: {3}},
		initial:    map[int]int{0: 1},
	}
	system.transfer = flowTransfer(system.successors, 20, nil)
	assertDifferential(t, system, finiteIntLattice(20))
}

func TestRegionDifferentialLoopWideningAndNarrowing(t *testing.T) {
	system := graphSystem{
		cells:      []int{0, 1, 2, 3},
		successors: map[int][]int{0: {1}, 1: {2, 3}, 2: {1}},
		initial:    map[int]int{0: 1}, widenAt: map[int]bool{1: true}, widenDelay: 1,
	}
	system.transfer = flowTransfer(system.successors, 3, map[int]bool{2: true})
	result := assertDifferential(t, system, wideningIntLattice(10))
	if result.Stats.WidenCalls == 0 || result.Stats.NarrowCalls == 0 {
		t.Fatalf("widen/narrow stats = %#v", result.Stats)
	}
	if result.Values[1] != 3 || result.Values[2] != 3 || result.Values[3] != 3 {
		t.Fatalf("narrowed loop values = %v", result.Values)
	}
}

func TestRegionDifferentialNestedLoops(t *testing.T) {
	system := graphSystem{
		cells: []int{0, 1, 2, 3, 4, 5},
		successors: map[int][]int{
			0: {1}, 1: {2, 5}, 2: {3, 4}, 3: {2}, 4: {1},
		},
		initial: map[int]int{0: 1}, widenAt: map[int]bool{1: true, 2: true},
	}
	system.transfer = flowTransfer(system.successors, 4, map[int]bool{3: true, 4: true})
	result := assertDifferential(t, system, finiteIntLattice(4))
	if result.Stats.Components < 2 {
		t.Fatalf("nested WTO components = %d, want at least 2", result.Stats.Components)
	}
}

func TestRegionDifferentialCancellationPublishesNothing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	system := graphSystem{
		cells: []int{0, 1}, successors: map[int][]int{0: {1}}, initial: map[int]int{0: 1},
	}
	system.transfer = func(cell int, read func(int) int, emit func(int, int)) {
		if cell == 0 {
			emit(1, read(0))
			cancel()
		}
	}
	regionResult, regionErr := Run(ctx, regionSystem(system, finiteIntLattice(5)))
	legacyResult, _, legacyErr := solve.SolveWTOContextWithVersions(ctx, solveSystem(system, finiteIntLattice(5)), solve.NewWTOPlan(system.cells, system.successor))
	if !errors.Is(regionErr, solve.ErrCanceled) || !errors.Is(regionErr, context.Canceled) {
		t.Fatalf("region cancellation = %v", regionErr)
	}
	if !errors.Is(legacyErr, solve.ErrCanceled) || !errors.Is(legacyErr, context.Canceled) {
		t.Fatalf("legacy cancellation = %v", legacyErr)
	}
	if regionResult.Values != nil || regionResult.Observations != nil || legacyResult != nil {
		t.Fatalf("canceled publish: region=%#v legacy=%v", regionResult, legacyResult)
	}
}

func assertDifferential(t testing.TB, system graphSystem, domain lattice.Lattice[int]) Result[int, int] {
	t.Helper()
	legacy, legacyVersions, err := solve.SolveWTOWithVersions(solveSystem(system, domain), solve.NewWTOPlan(system.cells, system.successor))
	if err != nil {
		t.Fatalf("legacy SolveWTO: %v", err)
	}
	result, err := Run(nil, regionSystem(system, domain))
	if err != nil {
		t.Fatalf("region Run: %v", err)
	}
	if !maps.Equal(result.Values, legacy) || !maps.Equal(result.Revisions, legacyVersions) {
		t.Fatalf("region/legacy differ: values=%v/%v revisions=%v/%v", result.Values, legacy, result.Revisions, legacyVersions)
	}
	if result.Stats.TransferCalls == 0 || result.Stats.MaxRevision == 0 || len(result.Observations) != result.Stats.ObservationCnt {
		t.Fatalf("incomplete stats/observations: %#v observations=%d", result.Stats, len(result.Observations))
	}
	for index, cell := range system.cells {
		observation := result.Observations[index]
		if observation.Cell != cell || observation.Value != result.Values[cell] || observation.Revision != result.Revisions[cell] {
			t.Fatalf("observation %d = %#v", index, observation)
		}
	}
	second, err := Run(nil, regionSystem(system, domain))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result, second) {
		t.Fatal("region results, revisions, observations, or stats are nondeterministic")
	}
	return result
}

func regionSystem(system graphSystem, domain lattice.Lattice[int]) System[int, int] {
	return System[int, int]{
		Lattice: domain, Cells: system.cells,
		Plan:                solve.NewWTOPlan(system.cells, system.successor),
		InitialSparse:       func(cell int) (int, bool) { value, ok := system.initial[cell]; return value, ok },
		Transfer:            system.transfer,
		WidenAt:             func(cell int) bool { return system.widenAt[cell] },
		WidenDelay:          func(int) int { return system.widenDelay },
		CaptureObservations: true,
	}
}

func TestRegionRejectsMismatchedPrebuiltPlan(t *testing.T) {
	system := graphSystem{cells: []int{0, 1}, successors: map[int][]int{0: {1}}}
	system.transfer = flowTransfer(system.successors, 2, nil)
	configured := regionSystem(system, finiteIntLattice(2))
	configured.Plan = solve.NewWTOPlan([]int{0}, func(int) []int { return nil })
	result, err := Run(nil, configured)
	if !errors.Is(err, solve.ErrWTOPlanUncovered) || result.Values != nil {
		t.Fatalf("mismatched plan result/error = %#v/%v", result, err)
	}
}

func solveSystem(system graphSystem, domain lattice.Lattice[int]) solve.EquationSystem[int, int] {
	return solve.EquationSystem[int, int]{
		Lattice: domain, Cells: system.cells,
		InitialSparse: func(cell int) (int, bool) { value, ok := system.initial[cell]; return value, ok },
		Transfer:      system.transfer,
		WidenAt:       func(cell int) bool { return system.widenAt[cell] },
		WidenDelay:    func(int) int { return system.widenDelay },
	}
}

func (s graphSystem) successor(cell int) []int { return s.successors[cell] }

func flowTransfer(successors map[int][]int, cap int, increment map[int]bool) func(int, func(int) int, func(int, int)) {
	return func(cell int, read func(int) int, emit func(int, int)) {
		value := read(cell)
		if increment[cell] {
			value = min(cap, value+1)
		}
		for _, successor := range successors[cell] {
			emit(successor, value)
		}
	}
}

func finiteIntLattice(top int) lattice.Lattice[int] {
	return lattice.Lattice[int]{
		Bottom: func() int { return 0 }, Top: func() int { return top },
		Equal:    func(left, right int) bool { return left == right },
		LessOrEq: func(left, right int) bool { return left <= right },
		Join:     func(left, right int) int { return max(left, right) },
		Widen:    func(left, right int) int { return max(left, right) },
	}
}

func wideningIntLattice(top int) lattice.Lattice[int] {
	domain := finiteIntLattice(top)
	domain.Widen = func(previous, next int) int {
		if next > previous {
			return top
		}
		return previous
	}
	domain.Narrow = func(previous, next int) int { return min(previous, next) }
	return domain
}
