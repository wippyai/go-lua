package transferfacts

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
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
	out := make([]factflow.BranchDiffConstraint, 0, len(in))
	for _, d := range in {
		out = append(out, newBranchDiffConstraintFromDescriptor(branchDiffDescriptorFromWIR(d)))
	}
	return out
}

func branchDiffConstraintsFromDescriptors(in []branchcond.BranchDiffConstraint) []factflow.BranchDiffConstraint {
	if len(in) == 0 {
		return nil
	}
	out := make([]factflow.BranchDiffConstraint, 0, len(in))
	for _, d := range in {
		out = append(out, newBranchDiffConstraintFromDescriptor(branchDiffDescriptorFromBranchCond(d)))
	}
	return out
}

type branchDiffConstraintDescriptor struct {
	coHi     int64
	hiPath   pathdom.Path
	hiIsLen  bool
	coHi2    int64
	hi2Path  pathdom.Path
	hi2IsLen bool
	hasHi2   bool
	loPath   pathdom.Path
	loIsLen  bool
	c        int64
	edge     bool
}

func branchDiffDescriptorFromWIR(d wir.BranchDiffConstraint) branchDiffConstraintDescriptor {
	return branchDiffConstraintDescriptor{
		coHi:     d.CoHi,
		hiPath:   d.HiPath,
		hiIsLen:  d.HiIsLen,
		coHi2:    d.CoHi2,
		hi2Path:  d.Hi2Path,
		hi2IsLen: d.Hi2IsLen,
		hasHi2:   d.HasHi2,
		loPath:   d.LoPath,
		loIsLen:  d.LoIsLen,
		c:        d.C,
		edge:     d.Edge,
	}
}

func branchDiffDescriptorFromBranchCond(d branchcond.BranchDiffConstraint) branchDiffConstraintDescriptor {
	return branchDiffConstraintDescriptor{
		coHi:     d.CoHi,
		hiPath:   d.HiPath,
		hiIsLen:  d.HiIsLen,
		coHi2:    d.CoHi2,
		hi2Path:  d.Hi2Path,
		hi2IsLen: d.Hi2IsLen,
		hasHi2:   d.HasHi2,
		loPath:   d.LoPath,
		loIsLen:  d.LoIsLen,
		c:        d.C,
		edge:     d.Edge,
	}
}

func newBranchDiffConstraintFromDescriptor(d branchDiffConstraintDescriptor) factflow.BranchDiffConstraint {
	if !d.hasHi2 {
		d.coHi2 = 0
		d.hi2Path = pathdom.Path{}
		d.hi2IsLen = false
	}
	return factflow.NewBranchScaledConstraintOnEdge(
		d.coHi,
		d.hiPath,
		d.hiIsLen,
		d.coHi2,
		d.hi2Path,
		d.hi2IsLen,
		d.loPath,
		d.loIsLen,
		d.c,
		d.edge,
	)
}
