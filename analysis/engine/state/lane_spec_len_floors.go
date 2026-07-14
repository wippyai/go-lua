package state

import (
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
)

const LaneLenFloors LaneID = "len-floors"

var lenFloorsLaneSpec = laneSpec{
	id:           LaneLenFloors,
	keySpaceMode: laneKeySpaceOwned,
	rekey: func(s State, from, to *keyspace.KeySpace) (State, bool) {
		lane, ok := s.lenFloors.rekey(from, to)
		if !ok {
			return s, false
		}
		s.lenFloors = lane
		return s, true
	},
	fingerprint: fingerprintLenFloors,
	markReachable: func(s State) State {
		s.lenFloors = s.lenFloors.reachable()
		return s
	},
	build: func(reg *axis.Registry, _ DomainOptions) laneOps {
		return stateLane(lenFloorMapDomain(),
			func(s State) lenFloorLane { return s.lenFloors },
			func(out *State, lane lenFloorLane) { out.lenFloors = lane },
		)
	},
}
