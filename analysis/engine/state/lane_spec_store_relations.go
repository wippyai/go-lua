package state

import "github.com/wippyai/go-lua/analysis/domain/value/axis"

const LaneStoreRelations LaneID = "store-relations"

var storeRelationsLaneSpec = laneSpec{
	id:           LaneStoreRelations,
	keySpaceMode: laneKeySpaceFree,
	fingerprint:  fingerprintStoreRelations,
	boundary:     boundaryLaneOps{expand: expandStoreRelationsBoundary},
	markReachable: func(s State) State {
		s.storeRelations = s.storeRelations.reachable()
		return s
	},
	build: func(reg *axis.Registry, _ DomainOptions) laneOps {
		return stateLane(storeRelationDomain(),
			func(s State) storeRelationLane { return s.storeRelations },
			func(out *State, lane storeRelationLane) { out.storeRelations = lane },
		)
	},
}

func expandStoreRelationsBoundary(expansion *boundaryClosureExpansion, source State) {
	if source.storeRelations.bottom {
		return
	}
	for relation := range source.storeRelations.values {
		expansion.connect(expansion.addStateKey(relation.Source), expansion.addStateKey(relation.Into))
	}
}
