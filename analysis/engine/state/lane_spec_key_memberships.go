package state

import "github.com/wippyai/go-lua/analysis/domain/value/axis"

const LaneKeyMemberships LaneID = "key-memberships"

var keyMembershipsLaneSpec = laneSpec{
	id: LaneKeyMemberships,
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
