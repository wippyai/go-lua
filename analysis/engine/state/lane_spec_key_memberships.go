package state

import (
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
)

const LaneKeyMemberships LaneID = "key-memberships"

var keyMembershipsLaneSpec = laneSpec{
	id:           LaneKeyMemberships,
	keySpaceMode: laneKeySpaceOwned,
	boundary:     boundaryLaneOps{expand: expandKeyMembershipsBoundary},
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

func expandKeyMembershipsBoundary(expansion *boundaryClosureExpansion, source State) {
	if source.keyMemberships.bottom {
		return
	}
	for membership := range source.keyMemberships.path {
		expansion.connect(expansion.addStateKey(membership.Key), expansion.addStateKey(membership.Table))
	}
	for membership := range source.keyMemberships.dynamic {
		expansion.connect(membership.Container, expansion.addStateKey(membership.Table))
	}
	for membership := range source.keyMemberships.dynamicAll {
		expansion.connect(membership.Container, expansion.addStateKey(membership.Table))
	}
	for origin := range source.keyMemberships.valueOrigins {
		expansion.connect(expansion.addStateKey(origin.Value), origin.Container)
	}
	for origin := range source.keyMemberships.readOrigins {
		expansion.connect(expansion.addStateKey(origin.Value), origin.Container, expansion.addStateKey(origin.Key))
	}
	for restore := range source.keyMemberships.pendingRestores {
		expansion.connect(restore.Container, expansion.addStateKey(restore.Table), expansion.addStateKey(restore.Key))
	}
}
