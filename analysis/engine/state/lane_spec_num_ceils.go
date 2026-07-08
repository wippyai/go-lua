package state

import "github.com/wippyai/go-lua/analysis/domain/value/axis"

const LaneNumCeils LaneID = "num-ceils"

var numCeilsLaneSpec = laneSpec{
	id: LaneNumCeils,
	markReachable: func(s State) State {
		s.numCeils = s.numCeils.reachable()
		return s
	},
	build: func(_ *axis.Registry, options DomainOptions) laneOps {
		return stateLane(numCeilMapDomain(options.WidenThresholds),
			func(s State) numCeilLane { return s.numCeils },
			func(out *State, lane numCeilLane) { out.numCeils = lane },
		)
	},
}
