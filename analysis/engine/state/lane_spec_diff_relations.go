package state

import "github.com/wippyai/go-lua/analysis/domain/value/axis"

const LaneDiffRelations LaneID = "diff-relations"

var diffRelationsLaneSpec = laneSpec{
	id:           LaneDiffRelations,
	keySpaceMode: laneKeySpaceFree,
	fingerprint:  fingerprintDiffRelations,
	boundary:     boundaryLaneOps{expand: expandDiffRelationsBoundary},
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

func expandDiffRelationsBoundary(expansion *boundaryClosureExpansion, source State) {
	if source.diffRelations.bottom {
		return
	}
	for relation := range source.diffRelations.values {
		a := expansion.addStateKey(relation.A.Key)
		c := expansion.addStateKey(relation.C.Key)
		if relation.B.valid() {
			expansion.connect(a, expansion.addStateKey(relation.B.Key), c)
			continue
		}
		expansion.connect(a, c)
	}
}
