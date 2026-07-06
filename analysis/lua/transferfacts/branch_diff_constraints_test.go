package transferfacts

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
)

func TestBranchDiffConstraintProjectionHonorsHasHi2(t *testing.T) {
	hi := path.NewPath(1, "hi")
	staleHi2 := path.NewPath(2, "stale")
	lo := path.NewPath(3, "lo")

	fromBranchCond := branchDiffConstraintsFromDescriptors([]branchcond.BranchDiffConstraint{{
		CoHi:    1,
		HiPath:  hi,
		CoHi2:   1,
		Hi2Path: staleHi2,
		HasHi2:  false,
		LoPath:  lo,
		Edge:    true,
	}})
	if len(fromBranchCond) != 1 {
		t.Fatalf("branchcond constraints = %#v, want one", fromBranchCond)
	}
	if fromBranchCond[0].HasHi2() {
		t.Fatalf("branchcond projection kept stale hi2 path: %#v", fromBranchCond[0])
	}

	fromWIR := branchDiffConstraintsFromWIRDescriptors([]wir.BranchDiffConstraint{{
		CoHi:    1,
		HiPath:  hi,
		CoHi2:   1,
		Hi2Path: staleHi2,
		HasHi2:  false,
		LoPath:  lo,
		Edge:    true,
	}})
	if len(fromWIR) != 1 {
		t.Fatalf("WIR constraints = %#v, want one", fromWIR)
	}
	if fromWIR[0].HasHi2() {
		t.Fatalf("WIR projection kept stale hi2 path: %#v", fromWIR[0])
	}
}
