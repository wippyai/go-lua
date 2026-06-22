package state

import "github.com/wippyai/go-lua/analysis/domain/value/axis"

const LaneStoreRelations LaneID = "store-relations"

var storeRelationsDomainLane = stateLaneFactory{
	id: LaneStoreRelations,
	build: func(reg *axis.Registry) stateLaneOps {
		return stateLane(storeRelationDomain(),
			func(s State) storeRelationLane { return s.storeRelations },
			func(out *State, lane storeRelationLane) { out.storeRelations = lane },
		)
	},
}
