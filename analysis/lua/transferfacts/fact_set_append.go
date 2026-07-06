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
	prior := out[point]
	existing := prior.Refinements()
	existing = append(existing, refinements...)
	out[point] = factflow.NewBranchRefinementSet(existing...).
		WithLenRefinements(prior.LenRefinements()...).
		WithNumFloorRefinements(prior.NumFloorRefinements()...)
}

func appendBranchLenRefinement(out map[cfg.Point]factflow.BranchRefinementSet, point cfg.Point, lenFloors ...factflow.BranchLenRefinement) {
	if len(lenFloors) == 0 {
		return
	}
	out[point] = out[point].WithLenRefinements(lenFloors...)
}

func appendBranchNumFloorRefinement(out map[cfg.Point]factflow.BranchRefinementSet, point cfg.Point, numFloors ...factflow.BranchNumFloorRefinement) {
	if len(numFloors) == 0 {
		return
	}
	out[point] = out[point].WithNumFloorRefinements(numFloors...)
}

func appendBranchDiffConstraint(out map[cfg.Point]factflow.BranchRefinementSet, point cfg.Point, diffs ...factflow.BranchDiffConstraint) {
	if len(diffs) == 0 {
		return
	}
	out[point] = out[point].WithDiffConstraints(diffs...)
}

func appendBranchPathEvidence(out map[cfg.Point]factflow.BranchPathEvidenceSet, point cfg.Point, proofs ...factflow.BranchPathEvidence) {
	if len(proofs) == 0 {
		return
	}
	existing := out[point].Evidence()
	existing = append(existing, proofs...)
	out[point] = factflow.NewBranchPathEvidenceSet(existing...)
}

func appendBranchPathRelations(out map[cfg.Point]factflow.BranchPathRelationSet, point cfg.Point, relations ...factflow.BranchPathRelation) {
	if len(relations) == 0 {
		return
	}
	existing := out[point].Relations()
	existing = append(existing, relations...)
	out[point] = factflow.NewBranchPathRelationSet(existing...)
}

func appendBranchPresenceRelations(out map[cfg.Point]factflow.BranchPresenceRelationSet, point cfg.Point, relations ...factflow.BranchPresenceRelation) {
	if len(relations) == 0 {
		return
	}
	existing := out[point].Relations()
	existing = append(existing, relations...)
	out[point] = factflow.NewBranchPresenceRelationSet(existing...)
}

func appendPathValuePresenceImplications(out map[cfg.Point]factflow.PathValuePresenceImplicationSet, point cfg.Point, implications ...factflow.PathValuePresenceImplication) {
	if len(implications) == 0 {
		return
	}
	existing := out[point].Implications()
	existing = append(existing, implications...)
	out[point] = factflow.NewPathValuePresenceImplicationSet(existing...)
}

func appendCallResultValues(out map[cfg.Point]factflow.CallResultValueSet, point cfg.Point, values ...factflow.CallResultValue) {
	if len(values) == 0 {
		return
	}
	existing := out[point].Values()
	existing = append(existing, values...)
	out[point] = factflow.NewCallResultValueSet(existing...)
}
