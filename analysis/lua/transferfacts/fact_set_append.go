package transferfacts

import (
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func appendPostconditionRefinements(out map[cfg.Point]factflow.PostconditionRefinementSet, point cfg.Point, refinements ...factflow.PostconditionRefinement) {
	if len(refinements) == 0 {
		return
	}
	existing := out[point].Refinements()
	existing = append(existing, refinements...)
	out[point] = factflow.NewPostconditionRefinementSet(existing...)
}

func appendBranchRefinement(out map[cfg.Point]factflow.BranchRefinementSet, point cfg.Point, refinements ...factflow.BranchRefinement) {
	if len(refinements) == 0 {
		return
	}
	existing := out[point].Refinements()
	existing = append(existing, refinements...)
	out[point] = factflow.NewBranchRefinementSet(existing...)
}

func appendBranchProofs(out map[cfg.Point]factflow.BranchProofSet, point cfg.Point, proofs ...factflow.BranchProof) {
	if len(proofs) == 0 {
		return
	}
	existing := out[point].Proofs()
	existing = append(existing, proofs...)
	out[point] = factflow.NewBranchProofSet(existing...)
}

func appendBranchPresenceRelations(out map[cfg.Point]factflow.BranchPresenceRelationSet, point cfg.Point, relations ...factflow.BranchPresenceRelation) {
	if len(relations) == 0 {
		return
	}
	existing := out[point].Relations()
	existing = append(existing, relations...)
	out[point] = factflow.NewBranchPresenceRelationSet(existing...)
}

func appendCallResultValues(out map[cfg.Point]factflow.CallResultValueSet, point cfg.Point, values ...factflow.CallResultValue) {
	if len(values) == 0 {
		return
	}
	existing := out[point].Values()
	existing = append(existing, values...)
	out[point] = factflow.NewCallResultValueSet(existing...)
}
