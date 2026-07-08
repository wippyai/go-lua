package state

import (
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
)

const LaneTypestates LaneID = "typestates"

var typestatesLaneSpec = laneSpec{
	id: LaneTypestates,
	build: func(reg *axis.Registry, _ DomainOptions) laneOps {
		return stateLane(typestate.Domain,
			func(s State) typestate.Store { return s.typestates },
			func(out *State, store typestate.Store) { out.typestates = store },
		)
	},
}
