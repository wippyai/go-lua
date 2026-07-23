package transformer

import "github.com/wippyai/go-lua/analysis/engine/operationplan"

func isBranchEdgeOwnedKind(kind operationplan.Kind) bool {
	switch kind {
	case operationplan.BranchEdgeReachability, operationplan.BranchConditionSource, operationplan.BranchRefinement, operationplan.BranchPathEvidence:
		return true
	default:
		return false
	}
}
