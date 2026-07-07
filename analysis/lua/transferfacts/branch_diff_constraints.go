package transferfacts

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
)

func (l *lowerer) branchDiffConstraintsFromWIR(point cfg.Point) []factflow.BranchDiffConstraint {
	if l == nil || l.wir == nil {
		return nil
	}
	var out []factflow.BranchDiffConstraint
	for _, inst := range l.wir.PointInstructions(point) {
		if inst.Op != wir.OpBranch {
			continue
		}
		out = append(out, branchDiffConstraintsFromWIRDescriptors(l.wir.BranchDiffConstraints(inst.DiffConstraints))...)
	}
	return out
}

func branchDiffConstraintsFromWIRDescriptors(in []wir.BranchDiffConstraint) []factflow.BranchDiffConstraint {
	if len(in) == 0 {
		return nil
	}
	out := make([]factflow.BranchDiffConstraint, 0, len(in))
	for _, d := range in {
		out = append(out, branchDiffConstraintFromWIR(d))
	}
	return out
}

func branchDiffConstraintFromWIR(d wir.BranchDiffConstraint) factflow.BranchDiffConstraint {
	coHi2, hi2Path, hi2IsLen := branchDiffSecondUpperBound(d.HasHi2, d.CoHi2, d.Hi2Path, d.Hi2IsLen)
	return factflow.NewBranchScaledConstraintOnEdge(
		d.CoHi,
		d.HiPath,
		d.HiIsLen,
		coHi2,
		hi2Path,
		hi2IsLen,
		d.LoPath,
		d.LoIsLen,
		d.C,
		d.Edge,
	)
}

func branchDiffSecondUpperBound(has bool, co int64, p pathdom.Path, isLen bool) (int64, pathdom.Path, bool) {
	if !has {
		return 0, pathdom.Path{}, false
	}
	return co, p, isLen
}
