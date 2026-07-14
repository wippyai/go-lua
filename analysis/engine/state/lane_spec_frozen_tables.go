package state

import "github.com/wippyai/go-lua/analysis/domain/value/axis"

const LaneFrozenTables LaneID = "frozen-tables"

var frozenTablesLaneSpec = laneSpec{
	id:          LaneFrozenTables,
	fingerprint: fingerprintFrozenTables,
	markReachable: func(s State) State {
		s.frozenTables = s.frozenTables.reachable()
		return s
	},
	build: func(reg *axis.Registry, _ DomainOptions) laneOps {
		return stateLane(frozenTableDomain(),
			func(s State) frozenTableLane { return s.frozenTables },
			func(out *State, lane frozenTableLane) { out.frozenTables = lane },
		)
	},
}
