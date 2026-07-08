package state

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/state/userlattice"
)

const LaneUserLattices LaneID = "user-lattices"

var userLatticesLaneSpec = laneSpec{
	id: LaneUserLattices,
	build: func(reg *axis.Registry, _ DomainOptions) laneOps {
		rt := userlattice.RuntimeFor(reg)
		return stateLane(userLatticeDomain(rt),
			func(s State) userLatticeLane { return s.userLattices },
			func(out *State, lane userLatticeLane) { out.userLattices = lane },
		)
	},
}
