package state

import "github.com/wippyai/go-lua/analysis/domain/value/axis"

const LaneDiffRelations LaneID = "diff-relations"

var diffRelationsDomainLane = stateLaneFactory{
	id: LaneDiffRelations,
	build: func(reg *axis.Registry) stateLaneOps {
		return stateLane(diffRelationDomain(),
			func(s State) diffRelationLane { return s.diffRelations },
			func(out *State, lane diffRelationLane) { out.diffRelations = lane },
		)
	},
}
