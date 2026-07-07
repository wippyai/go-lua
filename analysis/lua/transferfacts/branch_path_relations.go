package transferfacts

import (
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
)

func (l *lowerer) branchPathRelationsFromWIR(point cfg.Point) (factflow.BranchPathRelationSet, bool) {
	if l == nil || l.wir == nil {
		return factflow.BranchPathRelationSet{}, false
	}
	var relations []factflow.BranchPathRelation
	l.forEachWIRBranchCheck(point, func(check branchcond.Check) {
		if check.Kind != branchcond.CheckNone {
			relations = append(relations, checkPathRelations(check, true, true)...)
		}
	}, func(implied branchcond.ImpliedCheck) {
		relations = append(relations, checkPathRelationsForImplication(implied)...)
	})
	if len(relations) == 0 {
		return factflow.BranchPathRelationSet{}, false
	}
	return factflow.NewBranchPathRelationSet(relations...), true
}

// checkPathRelations lowers one path or type comparison into branch relations.
// activeOnTrue/activeOnFalse select the edges on which the matched and unmatched
// relations apply, so a direct condition narrows both edges while a decomposed
// conjunct or disjunct narrows only its own edge.
func checkPathRelations(check branchcond.Check, activeOnTrue, activeOnFalse bool) []factflow.BranchPathRelation {
	matched, ok := branchPathRelationForCheck(check, true, activeOnTrue, false)
	if !ok {
		return nil
	}
	unmatched, ok := branchPathRelationForCheck(check, false, false, activeOnFalse)
	if !ok {
		return []factflow.BranchPathRelation{matched}
	}
	return []factflow.BranchPathRelation{matched, unmatched}
}

func checkPathRelationsForImplication(implied branchcond.ImpliedCheck) []factflow.BranchPathRelation {
	relation, ok := branchPathRelationForCheck(implied.Check, implied.Polarity, implied.Edge, !implied.Edge)
	if !ok {
		return nil
	}
	return []factflow.BranchPathRelation{relation}
}

func branchPathRelationForCheck(
	check branchcond.Check,
	matches bool,
	activeOnTrue bool,
	activeOnFalse bool,
) (factflow.BranchPathRelation, bool) {
	left := check.Path
	right := check.OtherPath
	if left.IsEmpty() || right.IsEmpty() {
		return factflow.BranchPathRelation{}, false
	}
	switch check.Kind {
	case branchcond.CheckPathEqual:
		if matches {
			return factflow.NewBranchPathEquality(left, right, activeOnTrue, activeOnFalse), true
		}
		return factflow.NewBranchPathInequality(left, right, activeOnTrue, activeOnFalse), true
	case branchcond.CheckPathNot:
		if matches {
			return factflow.NewBranchPathInequality(left, right, activeOnTrue, activeOnFalse), true
		}
		return factflow.NewBranchPathEquality(left, right, activeOnTrue, activeOnFalse), true
	case branchcond.CheckTypeEqual:
		if matches {
			return factflow.NewBranchPathTypeMatch(left, right, activeOnTrue, activeOnFalse), true
		}
		return factflow.NewBranchPathTypeUnmatch(left, right, activeOnTrue, activeOnFalse), true
	case branchcond.CheckTypeNot:
		if matches {
			return factflow.NewBranchPathTypeUnmatch(left, right, activeOnTrue, activeOnFalse), true
		}
		return factflow.NewBranchPathTypeMatch(left, right, activeOnTrue, activeOnFalse), true
	default:
		return factflow.BranchPathRelation{}, false
	}
}
