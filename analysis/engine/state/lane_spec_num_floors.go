package state

import "github.com/wippyai/go-lua/analysis/domain/value/axis"

const LaneNumFloors LaneID = "num-floors"

var numFloorsLaneSpec = laneSpec{
	id: LaneNumFloors,
	markReachable: func(s State) State {
		s.numFloors = s.numFloors.reachable()
		return s
	},
	build: func(reg *axis.Registry, _ DomainOptions) laneOps {
		return stateLane(numFloorMapDomain(),
			func(s State) numFloorLane { return s.numFloors },
			func(out *State, lane numFloorLane) { out.numFloors = lane },
		)
	},
}
