package state

import "github.com/wippyai/go-lua/analysis/domain/value/axis"

const LaneDiffRelations LaneID = "diff-relations"

var diffRelationsLaneSpec = laneSpec{
	id:          LaneDiffRelations,
	fingerprint: fingerprintDiffRelations,
	markReachable: func(s State) State {
		s.diffRelations = s.diffRelations.reachable()
		return s
	},
	build: func(reg *axis.Registry, _ DomainOptions) laneOps {
		return stateLane(diffRelationDomain(),
			func(s State) diffRelationLane { return s.diffRelations },
			func(out *State, lane diffRelationLane) { out.diffRelations = lane },
		)
	},
}
