package state

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func diffKey(name string) pathdom.PathKey { return pathdom.PathKey(name) }

func diffStateKey(t *testing.T, name string) pathaddr.StateKey {
	t.Helper()
	key, ok := pathaddr.StateKeyFromPathKey(diffKey(name))
	if !ok {
		t.Fatalf("StateKeyFromPathKey(%q) failed", name)
	}
	return key
}

func diffValue(t *testing.T, name string) RelOperand {
	t.Helper()
	return RelValueOperand(diffStateKey(t, name))
}

func diffLength(t *testing.T, name string) RelOperand {
	t.Helper()
	return RelLengthOperand(diffStateKey(t, name))
}

func TestDiffConstraintWriteAndSnapshot(t *testing.T) {
	s := State{}.reachable()
	s = s.WriteDiffConstraint(diffValue(t, "i"), diffValue(t, "j"), -1)  // i < j
	s = s.WriteDiffConstraint(diffValue(t, "j"), diffLength(t, "xs"), 0) // j <= len(xs)
	snap := s.RelConstraints()
	if len(snap.Constraints) != 2 {
		t.Fatalf("want 2 constraints, got %d: %+v", len(snap.Constraints), snap.Constraints)
	}
}

func TestSumConstraintWriteAndSnapshot(t *testing.T) {
	s := State{}.reachable()
	// i + j <= len(xs), captured as i + j - len(xs) <= 0.
	s = s.WriteSumConstraint(diffValue(t, "i"), diffValue(t, "j"), diffLength(t, "xs"), 0)
	snap := s.RelConstraints()
	if len(snap.Constraints) != 1 {
		t.Fatalf("want 1 constraint, got %d: %+v", len(snap.Constraints), snap.Constraints)
	}
	c := snap.Constraints[0]
	if !c.B.valid() {
		t.Fatalf("sum constraint should carry a B operand, got %+v", c)
	}
	// Commutative sum dedups: j + i records the same canonical constraint.
	s2 := s.WriteSumConstraint(diffValue(t, "j"), diffValue(t, "i"), diffLength(t, "xs"), 0)
	if len(s2.RelConstraints().Constraints) != 1 {
		t.Fatalf("commutative sum should dedup, got %+v", s2.RelConstraints().Constraints)
	}
}

func TestScaledConstraintWriteAndSnapshot(t *testing.T) {
	s := State{}.reachable()
	// 2*i <= len(xs), captured as 2*i - len(xs) <= 0.
	s = s.WriteScaledConstraint(2, diffValue(t, "i"), 0, RelOperand{}, diffLength(t, "xs"), 0)
	snap := s.RelConstraints()
	if len(snap.Constraints) != 1 {
		t.Fatalf("want 1 constraint, got %d: %+v", len(snap.Constraints), snap.Constraints)
	}
	c := snap.Constraints[0]
	if c.CoA != 2 || c.A != diffValue(t, "i") || c.B.valid() || c.C != diffLength(t, "xs") || c.K != 0 {
		t.Fatalf("unexpected scaled constraint: %+v", c)
	}
	// WriteSumConstraint defaults coefficients to 1.
	u := State{}.reachable().WriteSumConstraint(diffValue(t, "i"), diffValue(t, "j"), diffLength(t, "xs"), 0)
	uc := u.RelConstraints().Constraints[0]
	if uc.CoA != 1 || uc.CoB != 1 {
		t.Fatalf("sum constraint should default coefficients to 1, got %+v", uc)
	}
}

func TestRelOperandDistinguishesValueAndLengthForSamePath(t *testing.T) {
	xsValue := diffValue(t, "xs")
	xsLength := diffLength(t, "xs")
	if xsValue == xsLength {
		t.Fatal("value(xs) and len(xs) should be distinct relation operands")
	}
	if xsValue.NumericKey() == xsLength.NumericKey() {
		t.Fatal("value(xs) and len(xs) should flatten to distinct numeric solver variables")
	}
	s := State{}.reachable().WriteDiffConstraint(xsValue, xsLength, 0)
	constraints := s.RelConstraints().Constraints
	if len(constraints) != 1 || constraints[0].A != xsValue || constraints[0].C != xsLength {
		t.Fatalf("value/length relation was not preserved: %+v", constraints)
	}
}

func TestDiffConstraintJoinIsIntersection(t *testing.T) {
	dom := Domain(standard.Registry())
	a := State{}.reachable().WriteDiffConstraint(diffValue(t, "i"), diffValue(t, "j"), -1).
		WriteDiffConstraint(diffValue(t, "a"), diffValue(t, "b"), 0)
	b := State{}.reachable().WriteDiffConstraint(diffValue(t, "i"), diffValue(t, "j"), -1)
	joined := dom.Join(a, b)
	got := joined.RelConstraints().Constraints
	// Only the constraint present on BOTH paths survives a must-join.
	if len(got) != 1 || got[0].A != diffValue(t, "i") || got[0].C != diffValue(t, "j") || got[0].K != -1 {
		t.Fatalf("join must keep only common constraint, got %+v", got)
	}
}

func TestDiffConstraintClearedOnMutation(t *testing.T) {
	s := State{}.reachable().
		WriteDiffConstraint(diffValue(t, "i"), diffLength(t, "xs"), 0).
		WriteDiffConstraint(diffValue(t, "xs"), diffValue(t, "j"), 0).
		WriteDiffConstraint(diffValue(t, "keep"), diffValue(t, "j"), 0)
	cleared, ok := s.diffRelations.clearMatching(func(k pathaddr.StateKey) bool {
		return k == diffStateKey(t, "xs")
	})
	if !ok {
		t.Fatal("mutation of xs should clear a constraint over len(xs)")
	}
	if _, _, items := cleared.snapshot(relConstraintLess); len(items) != 1 ||
		items[0].A != diffValue(t, "keep") ||
		items[0].C != diffValue(t, "j") {
		t.Fatalf("constraints over value(xs)/len(xs) should be cleared, got %+v", items)
	}
}
