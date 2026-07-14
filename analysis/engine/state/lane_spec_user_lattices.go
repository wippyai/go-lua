package state

import (
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/state/userlattice"
)

const LaneUserLattices LaneID = "user-lattices"

var userLatticesLaneSpec = laneSpec{
	id:           LaneUserLattices,
	keySpaceMode: laneKeySpaceOwned,
	rekey: func(s State, from, to *keyspace.KeySpace) (State, bool) {
		lane, ok := s.userLattices.rekey(from, to)
		if !ok {
			return s, false
		}
		s.userLattices = lane
		return s, true
	},
	fingerprint: fingerprintUserLattices,
	build: func(reg *axis.Registry, _ DomainOptions) laneOps {
		rt := userlattice.RuntimeFor(reg)
		return stateLane(userLatticeDomain(rt),
			func(s State) userLatticeLane { return s.userLattices },
			func(out *State, lane userLatticeLane) { out.userLattices = lane },
		)
	},
}
