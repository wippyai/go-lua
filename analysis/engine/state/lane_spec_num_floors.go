package state

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/state/numbound"
)

const LaneNumFloors LaneID = "num-floors"

var numFloorsLaneSpec = laneSpec{
	id: LaneNumFloors,
	markReachable: func(s State) State {
		s.numFloors = s.numFloors.Reachable()
		return s
	},
	build: func(reg *axis.Registry, _ DomainOptions) laneOps {
		return stateLane(numBoundLaneDomain(numbound.Lower, nil),
			func(s State) numBoundLane { return s.numFloors },
			func(out *State, lane numBoundLane) { out.numFloors = lane },
		)
	},
}
