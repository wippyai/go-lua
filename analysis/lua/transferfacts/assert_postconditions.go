package transferfacts

import (
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (l *lowerer) assertPostconditionRefinement(fact semantics.CallFact) (factflow.PostconditionRefinement, bool) {
	if !l.isDirectGlobalAssertStatementCall(fact) || len(fact.Args) == 0 {
		return factflow.PostconditionRefinement{}, false
	}
	check := branchcond.Normalize(fact.Args[0], l.bindings)
	branchRefinement, ok := l.branchRefinement(semantics.BranchConditionFact{Check: check})
	if !ok {
		return factflow.PostconditionRefinement{}, false
	}
	value, ok := branchRefinement.TrueValue()
	if !ok {
		return factflow.PostconditionRefinement{}, false
	}
	return factflow.NewPostconditionRefinement(branchRefinement.TargetPath(), value), true
}

func (l *lowerer) isDirectGlobalAssertStatementCall(fact semantics.CallFact) bool {
	if fact.Context != semantics.CallContextStatement || fact.Call == nil || fact.Receiver != nil || fact.Method != "" || len(fact.TypeArgs) != 0 {
		return false
	}
	fn, ok := fact.Func.(*ast.IdentExpr)
	return ok && l.bindings.ResolvesToGlobal(fn, "assert")
}
