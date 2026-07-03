package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func orderRootRefinementsBeforeDescendants(refinements []factflow.BranchRefinement) []factflow.BranchRefinement {
	if len(refinements) < 2 {
		return refinements
	}
	out := make([]factflow.BranchRefinement, 0, len(refinements))
	for _, refinement := range refinements {
		if len(refinement.TargetPath().Segments) == 0 {
			out = append(out, refinement)
		}
	}
	for _, refinement := range refinements {
		if len(refinement.TargetPath().Segments) != 0 {
			out = append(out, refinement)
		}
	}
	return out
}

func (l *lowerer) rootRefinementsForBranchRefinement(refinement factflow.BranchRefinement) []factflow.BranchRefinement {
	target := refinement.TargetPath()
	var out []factflow.BranchRefinement
	if value, ok := refinement.TrueValue(); ok && value.HasPresence(presence.Present()) {
		if root, ok := l.rootPresenceRefinement(target, true); ok {
			out = append(out, root)
		}
	}
	if value, ok := refinement.FalseValue(); ok && value.HasPresence(presence.Present()) {
		if root, ok := l.rootPresenceRefinement(target, false); ok {
			out = append(out, root)
		}
	}
	return out
}

func (l *lowerer) truthyBooleanRootRefinements(check branchcond.Check) []factflow.BranchRefinement {
	switch check.Kind {
	case branchcond.CheckTruthy:
		var out []factflow.BranchRefinement
		if root, ok := l.truthyBooleanRootLiteralRefinement(check.Path, typ.LiteralBool(true), true); ok {
			out = append(out, root)
		}
		if root, ok := l.truthyBooleanRootLiteralRefinement(check.Path, typ.LiteralBool(false), false); ok {
			out = append(out, root)
		}
		return out
	case branchcond.CheckFalsy:
		var out []factflow.BranchRefinement
		if root, ok := l.truthyBooleanRootLiteralRefinement(check.Path, typ.LiteralBool(false), true); ok {
			out = append(out, root)
		}
		if root, ok := l.truthyBooleanRootLiteralRefinement(check.Path, typ.LiteralBool(true), false); ok {
			out = append(out, root)
		}
		return out
	default:
		return nil
	}
}

func (l *lowerer) truthyBooleanRootRefinementOnEdge(check branchcond.Check, cond bool) []factflow.BranchRefinement {
	switch check.Kind {
	case branchcond.CheckTruthy:
		if cond {
			if root, ok := l.truthyBooleanRootLiteralRefinement(check.Path, typ.LiteralBool(true), true); ok {
				return []factflow.BranchRefinement{root}
			}
		} else {
			if root, ok := l.truthyBooleanRootLiteralRefinement(check.Path, typ.LiteralBool(false), false); ok {
				return []factflow.BranchRefinement{root}
			}
		}
	case branchcond.CheckFalsy:
		if cond {
			if root, ok := l.truthyBooleanRootLiteralRefinement(check.Path, typ.LiteralBool(false), true); ok {
				return []factflow.BranchRefinement{root}
			}
		} else {
			if root, ok := l.truthyBooleanRootLiteralRefinement(check.Path, typ.LiteralBool(true), false); ok {
				return []factflow.BranchRefinement{root}
			}
		}
	}
	return nil
}

func (l *lowerer) truthyBooleanRootRefinementForImplication(check branchcond.Check, polarity bool, edge bool) []factflow.BranchRefinement {
	switch check.Kind {
	case branchcond.CheckTruthy:
		if polarity {
			if root, ok := l.truthyBooleanRootLiteralRefinement(check.Path, typ.LiteralBool(true), edge); ok {
				return []factflow.BranchRefinement{root}
			}
		} else {
			if root, ok := l.truthyBooleanRootLiteralRefinement(check.Path, typ.LiteralBool(false), edge); ok {
				return []factflow.BranchRefinement{root}
			}
		}
	case branchcond.CheckFalsy:
		if polarity {
			if root, ok := l.truthyBooleanRootLiteralRefinement(check.Path, typ.LiteralBool(false), edge); ok {
				return []factflow.BranchRefinement{root}
			}
		} else {
			if root, ok := l.truthyBooleanRootLiteralRefinement(check.Path, typ.LiteralBool(true), edge); ok {
				return []factflow.BranchRefinement{root}
			}
		}
	}
	return nil
}

func (l *lowerer) truthyBooleanRootLiteralRefinement(target path.Path, lit typ.Type, cond bool) (factflow.BranchRefinement, bool) {
	if len(target.Segments) != 0 {
		return l.rootLiteralRefinement(target, lit, cond)
	}
	if target.Symbol == 0 {
		return factflow.BranchRefinement{}, false
	}
	rootType, ok := l.symbolTypes[target.Symbol]
	if !ok || !subtype.IsSubtype(rootType, typ.Boolean) || !subtype.IsSubtype(lit, rootType) {
		return factflow.BranchRefinement{}, false
	}
	value := factflow.NewValueConstraint(l.valueFromTypeWithWitness(lit))
	if cond {
		return factflow.NewBranchRefinement(target, value, true, factflow.ValueRefinement{}, false), true
	}
	return factflow.NewBranchRefinement(target, factflow.ValueRefinement{}, false, value, true), true
}
