package semantics

import (
	"slices"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
)

func copyBranchConditionFact(fact BranchConditionFact) BranchConditionFact {
	fact.Check = copyBranchConditionCheck(fact.Check)
	return fact
}

func copyBranchConditionCheck(check branchcond.Check) branchcond.Check {
	check.Path = copyPath(check.Path)
	return check
}

func copyPath(p path.Path) path.Path {
	p.Segments = slices.Clone(p.Segments)
	return p
}
