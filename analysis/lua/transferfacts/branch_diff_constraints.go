package transferfacts

import (
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/compiler/ast"
)

// branchDiffConstraints extracts difference-logic facts from normalized
// branch-condition descriptors proven on each edge. Syntax recognition belongs
// to branchcond/wirlower; this layer only projects neutral descriptors into the
// factflow lane.
func (l *lowerer) branchDiffConstraints(condition ast.Expr) []factflow.BranchDiffConstraint {
	return branchDiffConstraintsFromDescriptors(branchcond.BranchDiffConstraintsOnBothEdges(condition, l.bindings))
}

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
	out := make([]branchcond.BranchDiffConstraint, 0, len(in))
	for _, d := range in {
		out = append(out, branchcond.BranchDiffConstraint{
			CoHi:     d.CoHi,
			HiPath:   d.HiPath,
			HiIsLen:  d.HiIsLen,
			CoHi2:    d.CoHi2,
			Hi2Path:  d.Hi2Path,
			Hi2IsLen: d.Hi2IsLen,
			HasHi2:   d.HasHi2,
			LoPath:   d.LoPath,
			LoIsLen:  d.LoIsLen,
			C:        d.C,
			Edge:     d.Edge,
		})
	}
	return branchDiffConstraintsFromDescriptors(out)
}

func branchDiffConstraintsFromDescriptors(in []branchcond.BranchDiffConstraint) []factflow.BranchDiffConstraint {
	if len(in) == 0 {
		return nil
	}
	out := make([]factflow.BranchDiffConstraint, 0, len(in))
	for _, d := range in {
		out = append(out, factflow.NewBranchScaledConstraintOnEdge(
			d.CoHi,
			d.HiPath,
			d.HiIsLen,
			d.CoHi2,
			d.Hi2Path,
			d.Hi2IsLen,
			d.LoPath,
			d.LoIsLen,
			d.C,
			d.Edge,
		))
	}
	return out
}
