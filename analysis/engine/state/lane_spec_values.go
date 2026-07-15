package state

import "github.com/wippyai/go-lua/analysis/domain/value/axis"

const LaneValues LaneID = "values"

var valuesLaneSpec = laneSpec{
	id:           LaneValues,
	keySpaceMode: laneKeySpaceFree,
	fingerprint:  fingerprintValues,
	boundary:     boundaryLaneOps{expand: expandBoundaryNoop},
	build: func(reg *axis.Registry, _ DomainOptions) laneOps {
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
