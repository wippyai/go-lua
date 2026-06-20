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
	snap := s.DiffConstraints()
	if len(snap.Constraints) != 2 {
		t.Fatalf("want 2 constraints, got %d: %+v", len(snap.Constraints), snap.Constraints)
	}
}

func TestDiffConstraintJoinIsIntersection(t *testing.T) {
	dom := Domain(standard.Registry())
	a := State{}.reachable().WriteDiffConstraint(diffKey("i"), diffKey("j"), -1).
		WriteDiffConstraint(diffKey("a"), diffKey("b"), 0)
	b := State{}.reachable().WriteDiffConstraint(diffKey("i"), diffKey("j"), -1)
	joined := dom.Join(a, b)
	got := joined.DiffConstraints().Constraints
	// Only the constraint present on BOTH paths survives a must-join.
	if len(got) != 1 || got[0].Hi != diffKey("i") || got[0].Lo != diffKey("j") || got[0].C != -1 {
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
	if _, _, items := cleared.snapshot(diffConstraintLess); len(items) != 0 {
		t.Fatalf("constraint over len(xs) should be cleared, got %+v", items)
	}
}
