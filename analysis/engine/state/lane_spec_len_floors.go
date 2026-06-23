package state

import "github.com/wippyai/go-lua/analysis/domain/value/axis"

const LaneLenFloors LaneID = "len-floors"

var lenFloorsLaneSpec = laneSpec{
	id: LaneLenFloors,
	markReachable: func(s State) State {
		s.lenFloors = s.lenFloors.reachable()
		return s
	},
	build: func(reg *axis.Registry) laneOps {
		return stateLane(lenFloorMapDomain(),
			func(s State) lenFloorLane { return s.lenFloors },
			func(out *State, lane lenFloorLane) { out.lenFloors = lane },
		)
	},
}
