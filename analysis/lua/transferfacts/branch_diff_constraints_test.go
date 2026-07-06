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

func TestBranchDiffConstraintProjectionMatchesAcrossDescriptorSources(t *testing.T) {
	hi := path.NewPath(1, "hi")
	hi2 := path.NewPath(2, "hi2")
	lo := path.NewPath(3, "lo")

	fromBranchCond := branchDiffConstraintsFromDescriptors([]branchcond.BranchDiffConstraint{{
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
	if len(fromBranchCond) != 1 || len(fromWIR) != 1 {
		t.Fatalf("projected lengths = %d/%d, want one each", len(fromBranchCond), len(fromWIR))
	}
	assertSameBranchDiffConstraint(t, fromBranchCond[0], fromWIR[0])
}

func assertSameBranchDiffConstraint(t *testing.T, left, right interface {
	CoHi() int64
	HiPath() path.Path
	HiIsLength() bool
	CoHi2() int64
	Hi2Path() path.Path
	Hi2IsLength() bool
	HasHi2() bool
	LoPath() path.Path
	LoIsLength() bool
	C() int64
	Cond() bool
}) {
	t.Helper()
	if left.CoHi() != right.CoHi() ||
		!left.HiPath().Equal(right.HiPath()) ||
		left.HiIsLength() != right.HiIsLength() ||
		left.CoHi2() != right.CoHi2() ||
		!left.Hi2Path().Equal(right.Hi2Path()) ||
		left.Hi2IsLength() != right.Hi2IsLength() ||
		left.HasHi2() != right.HasHi2() ||
		!left.LoPath().Equal(right.LoPath()) ||
		left.LoIsLength() != right.LoIsLength() ||
		left.C() != right.C() ||
		left.Cond() != right.Cond() {
		t.Fatalf("constraints differ:\nleft:  %#v\nright: %#v", left, right)
	}
}
