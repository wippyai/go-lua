package transferfacts

import (
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
)

func (l *lowerer) branchPathRelations(fact semantics.BranchConditionFact) (factflow.BranchPathRelationSet, bool) {
	left := fact.Check.Path
	right := fact.Check.OtherPath
	if left.IsEmpty() || right.IsEmpty() {
		return factflow.BranchPathRelationSet{}, false
	}
	switch fact.Check.Kind {
	case branchcond.CheckPathEqual:
		return factflow.NewBranchPathRelationSet(
			factflow.NewBranchPathEquality(left, right, true, false),
			factflow.NewBranchPathInequality(left, right, false, true),
		), true
	case branchcond.CheckPathNot:
		return factflow.NewBranchPathRelationSet(
			factflow.NewBranchPathInequality(left, right, true, false),
			factflow.NewBranchPathEquality(left, right, false, true),
		), true
	default:
		return factflow.BranchPathRelationSet{}, false
	}
}
