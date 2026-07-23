package transferfacts

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/wir"
)

func TestWIRBranchDiffConstraintProjectionHonorsHasHi2(t *testing.T) {
	hi := path.NewPath(1, "hi")
	staleHi2 := path.NewPath(2, "stale")
	lo := path.NewPath(3, "lo")

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

func TestWIRBranchDiffConstraintProjectionCarriesDescriptorFields(t *testing.T) {
	hi := path.NewPath(1, "hi")
	hi2 := path.NewPath(2, "hi2")
	lo := path.NewPath(3, "lo")

	fromWIR := branchDiffConstraintsFromWIRDescriptors([]wir.BranchDiffConstraint{{
		CoHi:     2,
		HiPath:   hi,
		HiIsLen:  true,
		CoHi2:    -1,
		Hi2Path:  hi2,
		Hi2IsLen: true,
		HasHi2:   true,
		LoPath:   lo,
		LoIsLen:  true,
		C:        7,
		Edge:     false,
	}})
	if len(fromWIR) != 1 {
		t.Fatalf("projected constraints = %#v, want one", fromWIR)
	}
	got := fromWIR[0]
	if got.CoHi() != 2 ||
		!got.HiPath().Equal(hi) ||
		!got.HiIsLength() ||
		got.CoHi2() != -1 ||
		!got.Hi2Path().Equal(hi2) ||
		!got.Hi2IsLength() ||
		!got.HasHi2() ||
		!got.LoPath().Equal(lo) ||
		!got.LoIsLength() ||
		got.C() != 7 ||
		got.Cond() {
		t.Fatalf("WIR projected constraint lost descriptor fields: %#v", got)
	}
}
