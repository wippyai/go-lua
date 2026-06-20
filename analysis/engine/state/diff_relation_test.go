package state

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func diffKey(name string) pathdom.PathKey { return pathdom.PathKey(name) }

func TestDiffConstraintWriteAndSnapshot(t *testing.T) {
	s := State{}.reachable()
	s = s.WriteDiffConstraint(diffKey("i"), diffKey("j"), -1)      // i < j
	s = s.WriteDiffConstraint(diffKey("j"), LengthRelKey("xs"), 0) // j <= len(xs)
	snap := s.RelConstraints()
	if len(snap.Constraints) != 2 {
		t.Fatalf("want 2 constraints, got %d: %+v", len(snap.Constraints), snap.Constraints)
	}
}

func TestSumConstraintWriteAndSnapshot(t *testing.T) {
	s := State{}.reachable()
	// i + j <= len(xs), captured as i + j - len(xs) <= 0.
	s = s.WriteSumConstraint(diffKey("i"), diffKey("j"), LengthRelKey("xs"), 0)
	snap := s.RelConstraints()
	if len(snap.Constraints) != 1 {
		t.Fatalf("want 1 constraint, got %d: %+v", len(snap.Constraints), snap.Constraints)
	}
	c := snap.Constraints[0]
	if c.B == "" {
		t.Fatalf("sum constraint should carry a B operand, got %+v", c)
	}
	// Commutative sum dedups: j + i records the same canonical constraint.
	s2 := s.WriteSumConstraint(diffKey("j"), diffKey("i"), LengthRelKey("xs"), 0)
	if len(s2.RelConstraints().Constraints) != 1 {
		t.Fatalf("commutative sum should dedup, got %+v", s2.RelConstraints().Constraints)
	}
}

func TestDiffConstraintJoinIsIntersection(t *testing.T) {
	dom := Domain(standard.Registry())
	a := State{}.reachable().WriteDiffConstraint(diffKey("i"), diffKey("j"), -1).
		WriteDiffConstraint(diffKey("a"), diffKey("b"), 0)
	b := State{}.reachable().WriteDiffConstraint(diffKey("i"), diffKey("j"), -1)
	joined := dom.Join(a, b)
	got := joined.RelConstraints().Constraints
	// Only the constraint present on BOTH paths survives a must-join.
	if len(got) != 1 || got[0].A != diffKey("i") || got[0].C != diffKey("j") || got[0].K != -1 {
		t.Fatalf("join must keep only common constraint, got %+v", got)
	}
}

func TestDiffConstraintClearedOnMutation(t *testing.T) {
	s := State{}.reachable().WriteDiffConstraint(diffKey("i"), LengthRelKey("xs"), 0)
	cleared, ok := s.diffRelations.clearMatching(func(k pathdom.PathKey) bool {
		return k == diffKey("xs")
	})
	if !ok {
		t.Fatal("mutation of xs should clear a constraint over len(xs)")
	}
	if _, _, items := cleared.snapshot(relConstraintLess); len(items) != 0 {
		t.Fatalf("constraint over len(xs) should be cleared, got %+v", items)
	}
}
