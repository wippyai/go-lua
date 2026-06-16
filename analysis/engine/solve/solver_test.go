package solve

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/lattice"
	latticelaws "github.com/wippyai/go-lua/analysis/test/laws/lattice"
)

// capLattice is a tiny finite chain lattice 0 ⊑ 1 ⊑ … ⊑ cap used to drive the
// solver. Join is max, Meet is min, and Widen jumps straight to the top (cap)
// whenever the value moves — the simplest true widening: any ascending chain
// stabilizes after one widening step. The chain is finite so Widen=Join would
// also terminate; jump-to-top lets a single test exercise the widening path on
// an otherwise long ascending chain.
type capLattice struct{ top int }

func (c capLattice) lattice() lattice.Lattice[int] {
	return lattice.Lattice[int]{
		Bottom:   func() int { return 0 },
		Top:      func() int { return c.top },
		Equal:    func(a, b int) bool { return a == b },
		LessOrEq: func(a, b int) bool { return a <= b },
		Join:     maxi,
		Meet:     mini,
		Widen: func(prev, next int) int {
			if next == prev {
				return prev
			}
			return c.top
		},
	}
}

// joinOnly returns the same chain lattice but with Widen=Join, the valid
// finite-height widening. Used where we want the exact least fixed point rather
// than the widened over-approximation.
func (c capLattice) joinOnly() lattice.Lattice[int] {
	l := c.lattice()
	l.Widen = maxi
	return l
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

// TestLawSuite_CapLattice drives the standard lattice laws on the cap lattice.
// Both widening variants are checked so the test domain itself is known sound
// before it is used to validate the solver.
func TestLawSuite_CapLattice(t *testing.T) {
	cl := capLattice{top: 8}
	sample := []int{0, 1, 3, 5, 8}
	latticelaws.LawSuite[int]{Name: "cap-widen-top", Domain: cl.lattice(), Sample: sample}.Run(t)
	latticelaws.LawSuite[int]{Name: "cap-join", Domain: cl.joinOnly(), Sample: sample}.Run(t)
}

func TestSolve_PanicsOnNilTransfer(t *testing.T) {
	cl := capLattice{top: 1}
	requireSolvePanic(t, EquationSystem[string, int]{
		Lattice: cl.joinOnly(),
		Cells:   []string{"x"},
	}, "solve: EquationSystem.Transfer is nil")
}

func TestSolve_PanicsOnMissingLatticeHooks(t *testing.T) {
	cl := capLattice{top: 1}
	noOpTransfer := func(string, func(string) int, func(string, int)) {}

	tests := []struct {
		name   string
		mutate func(*EquationSystem[string, int])
		want   string
	}{
		{
			name: "Bottom",
			mutate: func(sys *EquationSystem[string, int]) {
				sys.Lattice.Bottom = nil
			},
			want: "solve: EquationSystem.Lattice.Bottom is nil",
		},
		{
			name: "Equal",
			mutate: func(sys *EquationSystem[string, int]) {
				sys.Lattice.Equal = nil
			},
			want: "solve: EquationSystem.Lattice.Equal is nil",
		},
		{
			name: "Join",
			mutate: func(sys *EquationSystem[string, int]) {
				sys.Lattice.Join = nil
			},
			want: "solve: EquationSystem.Lattice.Join is nil",
		},
		{
			name: "Widen",
			mutate: func(sys *EquationSystem[string, int]) {
				sys.Lattice.Widen = nil
				sys.WidenAt = func(string) bool { return true }
			},
			want: "solve: EquationSystem.Lattice.Widen is nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sys := EquationSystem[string, int]{
				Lattice:  cl.joinOnly(),
				Cells:    []string{"x"},
				Initial:  func(string) int { return 0 },
				Transfer: noOpTransfer,
			}
			tt.mutate(&sys)

			requireSolvePanic(t, sys, tt.want)
		})
	}
}

// TestSolve_MonotoneLeastFixedPoint solves a small monotone system and checks
// the result is the expected least fixed point.
//
// System (join-only lattice, top high enough to be irrelevant):
//
//	a := 2
//	b := max(a, 3)        // depends on a
//	c := max(b, 1)        // depends on b
//
// Least solution: a=2, b=3, c=3.
func TestSolve_MonotoneLeastFixedPoint(t *testing.T) {
	cl := capLattice{top: 100}
	sys := EquationSystem[string, int]{
		Lattice: cl.joinOnly(),
		Cells:   []string{"a", "b", "c"},
		Initial: func(string) int { return 0 },
		Transfer: func(cell string, read func(string) int, emit func(string, int)) {
			switch cell {
			case "a":
				emit("a", 2)
			case "b":
				emit("b", maxi(read("a"), 3))
			case "c":
				emit("c", maxi(read("b"), 1))
			}
		},
		WidenAt: nil,
	}

	got := Solve(sys)
	want := map[string]int{"a": 2, "b": 3, "c": 3}
	assertMapEqual(t, cl.joinOnly(), got, want)
}

func TestSolve_StatsCountsActualTransferInvocations(t *testing.T) {
	cl := capLattice{top: 10}
	stats := Stats{}
	visits := 0
	sys := EquationSystem[string, int]{
		Lattice: cl.joinOnly(),
		Cells:   []string{"x"},
		Transfer: func(cell string, read func(string) int, emit func(string, int)) {
			visits++
			if visits == 1 {
				emit("x", 1)
			}
		},
		Stats: &stats,
	}

	got := Solve(sys)
	if got["x"] != 1 {
		t.Fatalf("x = %d, want 1", got["x"])
	}
	if stats.TransferCalls != visits || stats.TransferCalls != 1 {
		t.Fatalf("TransferCalls = %d, visits = %d, want 1", stats.TransferCalls, visits)
	}
}

// TestSolve_DiamondRequeuesDependents checks the diamond dependency graph
// converges: a feeds b and c, both feed d. A late bump to a must propagate
// through both branches to d.
//
//	a := seed (re-emitted higher on second visit)
//	b := a + 0
//	c := a + 0
//	d := max(b, c)
func TestSolve_DiamondRequeuesDependents(t *testing.T) {
	cl := capLattice{top: 100}

	// a's transfer emits a growing value the first two visits, forcing b, c, d
	// to be re-queued and reconverge. The growth is bounded so it stabilizes.
	aVisits := 0
	sys := EquationSystem[string, int]{
		Lattice: cl.joinOnly(),
		Cells:   []string{"a", "b", "c", "d"},
		Initial: func(string) int { return 0 },
		Transfer: func(cell string, read func(string) int, emit func(string, int)) {
			switch cell {
			case "a":
				_ = read("a")
				aVisits++
				// Two-step ascending source: 5 then 9, then stable.
				if aVisits == 1 {
					emit("a", 5)
				} else {
					emit("a", 9)
				}
			case "b":
				emit("b", read("a"))
			case "c":
				emit("c", read("a"))
			case "d":
				emit("d", maxi(read("b"), read("c")))
			}
		},
	}

	got := Solve(sys)
	want := map[string]int{"a": 9, "b": 9, "c": 9, "d": 9}
	assertMapEqual(t, cl.joinOnly(), got, want)
}

// TestSolve_SelfDependentReconverges checks a cell that reads a cell it also
// depends on transitively (cycle) reconverges once its input is re-queued.
//
//	a feeds b; b feeds a back (cycle a <-> b). Both clamp to the joined value.
//	With join-only and a finite lattice the cycle terminates at the lub.
func TestSolve_SelfDependentReconverges(t *testing.T) {
	cl := capLattice{top: 100}
	sys := EquationSystem[string, int]{
		Lattice: cl.joinOnly(),
		Cells:   []string{"a", "b"},
		Initial: func(c string) int {
			if c == "a" {
				return 4
			}
			return 7
		},
		Transfer: func(cell string, read func(string) int, emit func(string, int)) {
			switch cell {
			case "a":
				emit("a", read("b"))
			case "b":
				emit("b", read("a"))
			}
		},
	}
	got := Solve(sys)
	// Both accumulate to the join of the two seeds (7).
	want := map[string]int{"a": 7, "b": 7}
	assertMapEqual(t, cl.joinOnly(), got, want)
}

// TestSolve_WidenForcesTermination drives an otherwise strictly ascending chain
// and verifies Widen at the cyclic cell forces termination at Top.
//
// Without widening, cell "x" would climb 0,1,2,… forever because its transfer
// emits read(x)+1. WidenAt("x") makes the first increase jump straight to Top,
// after which the chain is stationary. We assert the solver returns and the
// value is Top.
func TestSolve_WidenForcesTermination(t *testing.T) {
	cl := capLattice{top: 50}
	stats := Stats{}
	sys := EquationSystem[string, int]{
		Lattice: cl.lattice(), // Widen = jump-to-top
		Cells:   []string{"x"},
		Initial: func(string) int { return 0 },
		Transfer: func(cell string, read func(string) int, emit func(string, int)) {
			// Strictly ascending source: always one above current.
			emit("x", read("x")+1)
		},
		WidenAt: func(c string) bool { return c == "x" },
		Stats:   &stats,
	}

	got := Solve(sys)
	if got["x"] != cl.top {
		t.Fatalf("widening did not drive x to Top: got %d, want %d", got["x"], cl.top)
	}
	if stats.TransferCalls < 2 {
		t.Fatalf("TransferCalls = %d, want at least 2", stats.TransferCalls)
	}
}

// TestSolve_WidenDoesNotCollapseInitialFanIn checks the solver-level precision
// rule: a WidenAt cell still exact-joins contributions that arrive before the
// cell's own transfer has run. Widening starts on revisits, not while collecting
// the initial predecessor fan-in.
func TestSolve_WidenDoesNotCollapseInitialFanIn(t *testing.T) {
	cl := capLattice{top: 50}
	sys := EquationSystem[string, int]{
		Lattice: cl.lattice(), // Widen = jump-to-top if it fires
		Cells:   []string{"a", "b", "c", "d", "target"},
		Initial: func(string) int { return 0 },
		Transfer: func(cell string, _ func(string) int, emit func(string, int)) {
			switch cell {
			case "a":
				emit("target", 1)
			case "b":
				emit("target", 2)
			case "c":
				emit("target", 3)
			case "d":
				emit("target", 4)
			}
		},
		WidenAt: func(c string) bool { return c == "target" },
	}

	got := Solve(sys)
	if got["target"] != 4 {
		t.Fatalf("initial fan-in was widened: target=%d, want exact join 4", got["target"])
	}
}

// TestSolve_DelayedWideningKeepsInitialJoinsExact checks the precision policy:
// a WidenAt cell may receive a few exact post-visit Join updates before Widen
// is allowed to accelerate the chain. This is not an iteration cap; after the
// delay, the same widening still forces termination.
func TestSolve_DelayedWideningKeepsInitialJoinsExact(t *testing.T) {
	cl := capLattice{top: 50}
	seen := []int{}
	sys := EquationSystem[string, int]{
		Lattice: cl.lattice(), // Widen = jump-to-top
		Cells:   []string{"x"},
		Initial: func(string) int { return 0 },
		Transfer: func(cell string, read func(string) int, emit func(string, int)) {
			cur := read("x")
			seen = append(seen, cur)
			emit("x", cur+1)
		},
		WidenAt:    func(c string) bool { return c == "x" },
		WidenDelay: func(c string) int { return 2 },
	}

	got := Solve(sys)
	if got["x"] != cl.top {
		t.Fatalf("delayed widening did not terminate at Top: got %d, want %d", got["x"], cl.top)
	}
	if len(seen) < 3 || seen[0] != 0 || seen[1] != 1 || seen[2] != 2 {
		t.Fatalf("widening delay did not keep first joins exact; seen reads = %v, want prefix [0 1 2]", seen)
	}
}

// TestSolve_AbstractRunsAfterJoin pins the solver-level abstraction hook. The
// hook must run on the cell's joined accumulator, not only on the transfer
// contribution, otherwise a stale value already stored in the cell could survive
// every future projected emit.
func TestSolve_AbstractRunsAfterJoin(t *testing.T) {
	cl := capLattice{top: 10}
	sys := EquationSystem[string, int]{
		Lattice: cl.joinOnly(),
		Cells:   []string{"source", "target"},
		Initial: func(string) int { return 0 },
		Transfer: func(cell string, _ func(string) int, emit func(string, int)) {
			if cell == "source" {
				emit("target", 1)
			}
		},
		Abstract: func(cell string, value int) int {
			if cell == "target" && value > 0 {
				return cl.top
			}
			return value
		},
	}

	got := Solve(sys)
	if got["target"] != cl.top {
		t.Fatalf("Abstract did not run on joined target value: got %d, want %d", got["target"], cl.top)
	}
}

// TestSolve_Deterministic runs the same system twice and asserts the maps are
// Equal under the lattice. Uses the diamond system whose convergence order
// depends on correct deterministic re-queueing.
func TestSolve_Deterministic(t *testing.T) {
	cl := capLattice{top: 100}
	// Wider fan: cells 1..6 with a back edge from 6 to 1, so the system cycles
	// through the diamond until reaching the lub; correct deterministic
	// re-queueing must yield the same result on both runs.
	mk := func() EquationSystem[int, int] {
		return EquationSystem[int, int]{
			Lattice: cl.joinOnly(),
			Cells:   []int{1, 2, 3, 4, 5, 6},
			Initial: func(c int) int {
				if c == 1 {
					return 3
				}
				return 0
			},
			Transfer: func(cell int, read func(int) int, emit func(int, int)) {
				switch cell {
				case 1:
					// back edge from 6 keeps the system cycling until lub.
					emit(1, read(6))
				case 2:
					emit(2, read(1))
				case 3:
					emit(3, read(1))
				case 4:
					emit(4, maxi(read(2), read(3)))
				case 5:
					emit(5, read(4))
				case 6:
					emit(6, read(5))
				}
			},
		}
	}

	first := Solve(mk())
	second := Solve(mk())

	l := cl.joinOnly()
	if len(first) != len(second) {
		t.Fatalf("non-deterministic size: %d vs %d", len(first), len(second))
	}
	for c, v := range first {
		if !l.Equal(v, second[c]) {
			t.Fatalf("non-deterministic result at cell %d: %d vs %d", c, v, second[c])
		}
	}
	// All cells converge to the join of the single seed (3).
	for c, v := range first {
		if v != 3 {
			t.Fatalf("cell %d converged to %d, want 3", c, v)
		}
	}
}

// TestSolve_EmitToCellOutsideCells checks emit into a cell absent from Cells
// accumulates and re-queues readers, but the cell does not appear in the
// result and its own Transfer never runs.
func TestSolve_EmitToCellOutsideCells(t *testing.T) {
	cl := capLattice{top: 100}
	ranGhost := false
	sys := EquationSystem[string, int]{
		Lattice: cl.joinOnly(),
		Cells:   []string{"a"},
		Initial: func(string) int { return 0 },
		Transfer: func(cell string, read func(string) int, emit func(string, int)) {
			switch cell {
			case "a":
				emit("ghost", 5) // ghost not in Cells
				_ = read("ghost")
				emit("a", read("ghost"))
			case "ghost":
				ranGhost = true
			}
		},
	}
	got := Solve(sys)
	if ranGhost {
		t.Fatalf("Transfer ran for a cell absent from Cells")
	}
	if _, present := got["ghost"]; present {
		t.Fatalf("ghost cell appeared in result map")
	}
	if got["a"] != 5 {
		t.Fatalf("a did not observe emitted ghost value: got %d, want 5", got["a"])
	}
}

func assertMapEqual(t *testing.T, l lattice.Lattice[int], got, want map[string]int) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("result size %d, want %d: got=%v want=%v", len(got), len(want), got, want)
	}
	for c, w := range want {
		g, ok := got[c]
		if !ok {
			t.Fatalf("missing cell %q in result", c)
		}
		if !l.Equal(g, w) {
			t.Fatalf("cell %q = %d, want %d", c, g, w)
		}
	}
}

func requireSolvePanic(t *testing.T, sys EquationSystem[string, int], want string) {
	t.Helper()
	defer func() {
		got := recover()
		if got == nil {
			t.Fatalf("expected panic %q", want)
		}
		msg, ok := got.(string)
		if !ok || msg != want {
			t.Fatalf("panic = %v, want %q", got, want)
		}
	}()
	Solve(sys)
}
