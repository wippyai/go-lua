package transferfacts

import (
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
)

func (l *lowerer) branchRefinementsForCheck(check branchcond.Check) []factflow.BranchRefinement {
	refinement, ok := l.branchValueRefinementForCheck(check)
	if !ok {
		return nil
	}
	out := l.rootRefinementsForBranchRefinement(refinement)
	out = append(out, l.truthyBooleanRootRefinements(check)...)
	out = append(out, refinement)
	if descendant, ok := l.literalDescendantRefinementForCheck(check, refinement); ok {
		out = append(out, descendant)
	}
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

func (l *lowerer) branchImplicationRefinements(implied branchcond.ImpliedCheck) []factflow.BranchRefinement {
	refinement, ok := l.branchImplicationRefinement(implied)
	if !ok {
		return nil
	}
	out := l.rootRefinementsForBranchRefinement(refinement)
	if implied.Check.Kind == branchcond.CheckTruthy || implied.Check.Kind == branchcond.CheckFalsy {
		out = append(out, l.truthyBooleanRootRefinementForImplication(implied.Check, implied.Polarity, implied.Edge)...)
	}
	out = append(out, refinement)
	return out
}

func (l *lowerer) branchEdgeRefinement(check branchcond.Check, cond bool) (factflow.BranchRefinement, bool) {
	refinement, ok := l.branchValueRefinementForCheck(check)
	if !ok {
		return factflow.BranchRefinement{}, false
	}
	return branchRefinementOnSingleEdge(refinement, cond, cond)
}

func (l *lowerer) branchImplicationRefinement(implied branchcond.ImpliedCheck) (factflow.BranchRefinement, bool) {
	refinement, ok := l.branchValueRefinementForCheck(implied.Check)
	if !ok {
		return factflow.BranchRefinement{}, false
	}
	return branchRefinementOnSingleEdge(refinement, implied.Polarity, implied.Edge)
}

func branchRefinementOnSingleEdge(refinement factflow.BranchRefinement, valueEdge bool, targetEdge bool) (factflow.BranchRefinement, bool) {
	value, ok := refinement.ValueForEdge(valueEdge)
	if !ok {
		return factflow.BranchRefinement{}, false
	}
	if targetEdge {
		return factflow.NewBranchRefinement(refinement.TargetPath(), value, true, factflow.ValueRefinement{}, false), true
	}
	return factflow.NewBranchRefinement(refinement.TargetPath(), factflow.ValueRefinement{}, false, value, true), true
}

func (l *lowerer) literalDescendantRefinementForCheck(check branchcond.Check, refinement factflow.BranchRefinement) (factflow.BranchRefinement, bool) {
	if check.Kind != branchcond.CheckLiteralEqual && check.Kind != branchcond.CheckLiteralNot {
		return factflow.BranchRefinement{}, false
	}
	if check.Path.IsEmpty() || len(check.Path.Segments) == 0 || refinement.TargetPath().Equal(check.Path) {
		return factflow.BranchRefinement{}, false
	}
	lit, ok := check.LiteralValue()
	if !ok {
		return factflow.BranchRefinement{}, false
	}
	return l.descendantLiteralRefinement(check.Path, check.Kind, lit)
}
