package state

import (
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
)

const LaneKeyMemberships LaneID = "key-memberships"

var keyMembershipsLaneSpec = laneSpec{
	id:           LaneKeyMemberships,
	keySpaceMode: laneKeySpaceOwned,
	rekey: func(s State, from, to *keyspace.KeySpace) (State, bool) {
		lane, ok := s.keyMemberships.rekey(from, to)
		if !ok {
			return s, false
		}
		s.keyMemberships = lane
		return s, true
	},
	fingerprint: fingerprintKeyMemberships,
	markReachable: func(s State) State {
		s.keyMemberships = s.keyMemberships.reachable()
		return s
	},
	build: func(reg *axis.Registry, _ DomainOptions) laneOps {
		return stateLane(keyMembershipDomain(),
			func(s State) keyMembershipLane { return s.keyMemberships },
			func(out *State, lane keyMembershipLane) { out.keyMemberships = lane },
		)
	},
}
