package semantics

import (
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
)

func copyBranchConditionFact(fact BranchConditionFact) BranchConditionFact {
	fact.Check = copyBranchConditionCheck(fact.Check)
	return fact
}

func copyBranchConditionCheck(check branchcond.Check) branchcond.Check {
	check.Path = check.Path.Clone()
	check.OtherPath = check.OtherPath.Clone()
	return check
}
