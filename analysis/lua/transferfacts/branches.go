package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
)

func (l *lowerer) branchRefinement(fact semantics.BranchConditionFact) (factflow.BranchRefinement, bool) {
	target := fact.Check.Path
	if target.IsEmpty() {
		return factflow.BranchRefinement{}, false
	}
	switch fact.Check.Kind {
	case branchcond.CheckNil:
		return factflow.NewBranchRefinement(
			target,
			l.presenceRefinement(presence.Absent()), true,
			l.typedPresenceRefinement(target, presence.Present()), true,
		), true
	case branchcond.CheckNotNil:
		return factflow.NewBranchRefinement(
			target,
			l.typedPresenceRefinement(target, presence.Present()), true,
			l.presenceRefinement(presence.Absent()), true,
		), true
	case branchcond.CheckTruthy:
		return factflow.NewBranchRefinement(
			target,
			l.typedPresenceRefinement(target, presence.Present()), true,
			l.falsyAbsentRefinement(), true,
		), true
	case branchcond.CheckFalsy:
		trueValue := factflow.ValueRefinement{}
		hasTrue := false
		if len(target.Segments) != 0 {
			trueValue = l.boolLiteralRefinement(false)
			hasTrue = true
		}
		return factflow.NewBranchRefinement(
			target,
			trueValue, hasTrue,
			l.typedPresenceRefinement(target, presence.Present()), true,
		), true
	case branchcond.CheckLiteralEqual, branchcond.CheckLiteralNot:
		return l.literalBranchRefinement(target, fact.Check.Kind, fact.Check.LiteralString)
	case branchcond.CheckTypeEqual, branchcond.CheckTypeNot:
		return l.typeBranchRefinement(target, fact.Check.Kind, fact.Check.TypeName)
	default:
		return factflow.BranchRefinement{}, false
	}
}

// branchLenRefinements lowers non-empty / lower-bound length guards into
// true-edge length-floor facts. A guard such as #xs > 0 raises len(xs) >= 1 on
// the true edge only; the false edge and merges never carry it.
func (l *lowerer) branchLenRefinements(fact semantics.BranchConditionFact) []factflow.BranchLenRefinement {
	if fact.Check.Kind == branchcond.CheckLenGe {
		if lowered, ok := l.branchLenRefinement(fact.Check); ok {
			return []factflow.BranchLenRefinement{lowered}
		}
		return nil
	}
	if fact.Check.Kind != branchcond.CheckNone {
		return nil
	}
	var out []factflow.BranchLenRefinement
	for _, check := range branchcond.TruthyChecks(fact.Condition, l.bindings) {
		if lowered, ok := l.branchLenRefinement(check); ok {
			out = append(out, lowered)
		}
	}
	return out
}

func (l *lowerer) branchLenRefinement(check branchcond.Check) (factflow.BranchLenRefinement, bool) {
	if check.Kind != branchcond.CheckLenGe || check.Path.IsEmpty() || check.LenFloor <= 0 {
		return factflow.BranchLenRefinement{}, false
	}
	return factflow.NewBranchLenRefinement(check.Path, check.LenFloor), true
}

func (l *lowerer) branchRefinements(fact semantics.BranchConditionFact) []factflow.BranchRefinement {
	if fact.Check.Kind != branchcond.CheckNone {
		if lowered := l.branchRefinementsForCheck(fact.Check); len(lowered) != 0 {
			return lowered
		}
		return nil
	}
	var out []factflow.BranchRefinement
	for _, check := range branchcond.TruthyChecks(fact.Condition, l.bindings) {
		out = append(out, l.branchEdgeRefinements(check, true)...)
	}
	for _, check := range branchcond.FalsyChecks(fact.Condition, l.bindings) {
		out = append(out, l.branchEdgeRefinements(check, false)...)
	}
	return orderRootRefinementsBeforeDescendants(out)
}
