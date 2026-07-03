package state

import "github.com/wippyai/go-lua/analysis/domain/value/axis"

const LaneValues LaneID = "values"

var valuesLaneSpec = laneSpec{
	id: LaneValues,
	build: func(reg *axis.Registry) laneOps {
		domain := valueLaneDomain(reg)
		return stateLane(domain,
			func(s State) valueLane {
				return s.values
			},
			func(out *State, values valueLane) {
				out.values = values
			},
		)
	},
}
