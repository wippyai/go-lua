package transferfacts

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
)

func TestBranchFloorRefinementEdgeLaws(t *testing.T) {
	target := path.NewPath(1, "i")

	lenCheck := branchcond.Check{
		Kind:     branchcond.CheckLenGe,
		Path:     target,
		LenFloor: 2,
		Negated:  true,
	}
	if _, ok := branchLenRefinementOnEdge(lenCheck, true); ok {
		t.Fatal("negated length floor must not publish on true edge")
	}
	lenFloor, ok := branchLenRefinementForImplication(branchcond.ImpliedCheck{
		Check:    lenCheck,
		Edge:     false,
		Polarity: false,
	})
	if !ok || !lenFloor.ArrayPath().Equal(target) || lenFloor.Floor() != 2 || lenFloor.Cond() {
		t.Fatalf("negated length implication mismatch: %#v ok=%v", lenFloor, ok)
	}

	numCheck := branchcond.Check{
		Kind:     branchcond.CheckNumGe,
		Path:     target,
		NumFloor: 1,
		Negated:  false,
	}
	if _, ok := branchNumFloorRefinementForImplication(branchcond.ImpliedCheck{
		Check:    numCheck,
		Edge:     false,
		Polarity: false,
	}); ok {
		t.Fatal("numeric floor implication must require the polarity matching the proven bound")
	}
	numFloor, ok := branchNumFloorRefinementOnEdge(numCheck, true)
	if !ok || !numFloor.TargetPath().Equal(target) || numFloor.Floor() != 1 || !numFloor.Cond() {
		t.Fatalf("numeric floor direct edge mismatch: %#v ok=%v", numFloor, ok)
	}
}
