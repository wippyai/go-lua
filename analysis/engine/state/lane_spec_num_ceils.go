package state

import (
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/state/numbound"
)

const LaneNumCeils LaneID = "num-ceils"

var numCeilsLaneSpec = laneSpec{
	id:           LaneNumCeils,
	keySpaceMode: laneKeySpaceOwned,
	rekey: func(s State, from, to *keyspace.KeySpace) (State, bool) {
		lane, ok := numBoundRekey(s.numCeils, from, to)
		if !ok {
			return s, false
		}
		s.numCeils = lane
		return s, true
	},
	fingerprint: fingerprintNumCeils,
	markReachable: func(s State) State {
		s.numCeils = s.numCeils.Reachable()
		return s
	},
	build: func(_ *axis.Registry, options DomainOptions) laneOps {
		return stateLane(numBoundLaneDomain(numbound.Upper, options.WidenThresholds),
			func(s State) numBoundLane { return s.numCeils },
			func(out *State, lane numBoundLane) { out.numCeils = lane },
		)
	},
}
