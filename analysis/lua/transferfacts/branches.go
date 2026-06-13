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
			factflow.ValueRefinement{}, false,
		), true
	case branchcond.CheckFalsy:
		return factflow.NewBranchRefinement(
			target,
			factflow.ValueRefinement{}, false,
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
