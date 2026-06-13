package transferfacts

import (
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
)

func (l *lowerer) branchRefinementsForCheck(check branchcond.Check) []factflow.BranchRefinement {
	refinement, ok := l.branchRefinement(semantics.BranchConditionFact{Check: check})
	if !ok {
		return nil
	}
	out := l.rootRefinementsForBranchRefinement(refinement)
	out = append(out, l.truthyBooleanRootRefinements(check)...)
	out = append(out, refinement)
	return out
}

func (l *lowerer) branchEdgeRefinements(check branchcond.Check, cond bool) []factflow.BranchRefinement {
	refinement, ok := l.branchEdgeRefinement(check, cond)
	if !ok {
		return nil
	}
	out := l.rootRefinementsForBranchRefinement(refinement)
	if check.Kind == branchcond.CheckTruthy || check.Kind == branchcond.CheckFalsy {
		out = append(out, l.truthyBooleanRootRefinementOnEdge(check, cond)...)
	}
	out = append(out, refinement)
	return out
}

func (l *lowerer) branchEdgeRefinement(check branchcond.Check, cond bool) (factflow.BranchRefinement, bool) {
	refinement, ok := l.branchRefinement(semantics.BranchConditionFact{Check: check})
	if !ok {
		return factflow.BranchRefinement{}, false
	}
	value, ok := refinement.ValueForEdge(cond)
	if !ok {
		return factflow.BranchRefinement{}, false
	}
	if cond {
		return factflow.NewBranchRefinement(refinement.TargetPath(), value, true, factflow.ValueRefinement{}, false), true
	}
	return factflow.NewBranchRefinement(refinement.TargetPath(), factflow.ValueRefinement{}, false, value, true), true
}
