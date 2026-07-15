package state

import (
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
)

const LanePathEvidence LaneID = "path-evidence"

var pathEvidenceLaneSpec = laneSpec{
	id:           LanePathEvidence,
	keySpaceMode: laneKeySpaceOwned,
	boundary:     boundaryLaneOps{expand: expandPathEvidenceBoundary, project: projectPathEvidenceBoundary, rebase: rebasePathEvidenceBoundary, apply: applyPathEvidenceBoundary, equal: equalPathEvidenceBoundary},
	rekey: func(s State, from, to *keyspace.KeySpace) (State, bool) {
		lane, ok := s.pathEvidence.RekeyValueLanes(from, to)
		if !ok {
			return s, false
		}
		s.pathEvidence = lane
		return s, true
	},
	fingerprint: fingerprintPathEvidence,
	markReachable: func(s State) State {
		s.pathEvidence = s.pathEvidence.Reachable()
		return s
	},
	build: func(reg *axis.Registry, _ DomainOptions) laneOps {
		return stateLane(pathevidence.Domain(reg),
			func(s State) pathevidence.Lane { return s.pathEvidence },
			func(out *State, lane pathevidence.Lane) { out.pathEvidence = lane },
		)
	},
}

func expandPathEvidenceBoundary(expansion *boundaryClosureExpansion, source State) {
	source.pathEvidence.ForEachPathRefinement(func(path keyspace.Key, value product.Value) bool {
		if expansion.closure.pathTouches(expansion.keys, path) {
			expansion.addPath(path)
			expansion.addValue(value)
		}
		return true
	})
	source.pathEvidence.ForEachPathStaticMember(func(path keyspace.Key, value product.Value) bool {
		if expansion.closure.pathTouches(expansion.keys, path) {
			expansion.addPath(path)
			expansion.addValue(value)
		}
		return true
	})
	source.pathEvidence.ForEachBranchProof(func(proof pathevidence.BranchProof) bool {
		if expansion.closure.pathTouches(expansion.keys, proof.Path) || expansion.closure.pathTouches(expansion.keys, proof.Other) {
			expansion.addPath(proof.Path)
			expansion.addPath(proof.Other)
		}
		return true
	})
	source.pathEvidence.ForEachPathPresenceImplication(func(implication pathevidence.PathPresenceImplication) bool {
		if expansion.closure.pathTouches(expansion.keys, implication.Trigger) ||
			expansion.closure.pathTouches(expansion.keys, implication.TriggerOther) ||
			expansion.closure.pathTouches(expansion.keys, implication.Target) {
			expansion.addPath(implication.Trigger)
			expansion.addPath(implication.TriggerOther)
			expansion.addPath(implication.Target)
			if implication.HasTriggerValue {
				expansion.addValue(implication.TriggerValue)
			}
			if implication.HasTargetValue {
				expansion.addValue(implication.TargetValue)
			}
		}
		return true
	})
}
