package numbound

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/lattice/lift"
)

const testMaxBound = int64(^uint64(0) >> 1)
const testMinBound = -testMaxBound - 1

func testDomain(direction Direction, thresholds []int64) lattice.Lattice[int64] {
	bottom, top := testMaxBound, testMinBound
	if direction == Upper {
		bottom, top = testMinBound, testMaxBound
	}
	return IntDomain(Spec{Direction: direction, Bottom: bottom, Top: top, Thresholds: thresholds})
}

func TestGenericBoundCoreDualOrderAndWiden(t *testing.T) {
	lower := testDomain(Lower, nil)
	upper := testDomain(Upper, []int64{20, 10, 20})
	if lower.Join(2, 5) != 2 || upper.Join(2, 5) != 5 {
		t.Fatalf("join must keep weaker floor and weaker ceiling")
	}
	if !lower.LessOrEq(5, 2) || upper.LessOrEq(5, 2) {
		t.Fatalf("floor and ceiling orders are not dual")
	}
	for name, d := range map[string]lattice.Lattice[int64]{"lower": lower, "upper": upper} {
		joined := d.Join(-3, 7)
		if d.Join(7, -3) != joined || d.Join(joined, joined) != joined {
			t.Fatalf("%s join is not commutative/idempotent", name)
		}
		if !d.LessOrEq(-3, joined) || !d.LessOrEq(7, joined) {
			t.Fatalf("%s join result %d is not above both operands", name, joined)
		}
		widened := d.Widen(2, 5)
		if !d.LessOrEq(2, widened) || !d.LessOrEq(5, widened) {
			t.Fatalf("%s widen result %d is not above both operands", name, widened)
		}
	}
	if lower.Widen(2, 5) != testMinBound || lower.Widen(5, 2) != 2 {
		t.Fatalf("lower widening did not preserve floor behavior")
	}
	if upper.Widen(5, 12) != 20 || upper.Widen(20, 30) != testMaxBound {
		t.Fatalf("upper widening did not snap through thresholds to Top")
	}
}

func TestGenericBoundCoreMustMapUsesDirection(t *testing.T) {
	left := lift.MustMapValues(map[string]int64{"x": 4, "y": 9})
	right := lift.MustMapValues(map[string]int64{"x": 1})
	lower := lift.MustMap[string, int64](testDomain(Lower, nil)).Join(left, right).Values()
	upper := lift.MustMap[string, int64](testDomain(Upper, nil)).Join(left, right).Values()
	if len(lower) != 1 || lower["x"] != 1 || len(upper) != 1 || upper["x"] != 4 {
		t.Fatalf("lane joins = lower %#v upper %#v, want x floor 1 / ceiling 4", lower, upper)
	}
}
