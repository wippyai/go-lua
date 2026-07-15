package state

import (
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/state/numbound"
)

const LaneNumFloors LaneID = "num-floors"

var numFloorsLaneSpec = laneSpec{
	id:           LaneNumFloors,
	keySpaceMode: laneKeySpaceOwned,
	boundary:     boundaryLaneOps{expand: expandNumFloorsBoundary, project: projectNumFloorsBoundary, rebase: rebaseNumFloorsBoundary, apply: applyNumFloorsBoundary, equal: equalNumFloorsBoundary},
	rekey: func(s State, from, to *keyspace.KeySpace) (State, bool) {
		lane, ok := numBoundRekey(s.numFloors, from, to)
		if !ok {
			return s, false
		}
		s.numFloors = lane
		return s, true
	},
	fingerprint: fingerprintNumFloors,
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

func expandNumFloorsBoundary(expansion *boundaryClosureExpansion, source State) {
	expandNumBoundBoundary(expansion, source.numFloors)
}
