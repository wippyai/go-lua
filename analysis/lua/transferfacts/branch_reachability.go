package transferfacts

import (
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (l *lowerer) branchEdgeReachability(point cfg.Point, expr ast.Expr) (factflow.BranchEdgeReachability, bool) {
	if l.wir != nil {
		if reachability, ok := l.branchEdgeReachabilityFromWIR(point); ok {
			return reachability, true
		}
		if l.wir.HasInstruction(point, wir.OpBranch) {
			return factflow.BranchEdgeReachability{}, false
		}
	}
	return semanticBranchEdgeReachability(expr)
}

func semanticBranchEdgeReachability(expr ast.Expr) (factflow.BranchEdgeReachability, bool) {
	truthy, ok := branchcond.StaticLuaTruthiness(expr)
	if !ok {
		return factflow.BranchEdgeReachability{}, false
	}
	return factflow.NewBranchEdgeReachability(!truthy, truthy), true
}
