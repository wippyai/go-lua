package state

import "github.com/wippyai/go-lua/analysis/domain/value/axis"

const LaneFrozenTables LaneID = "frozen-tables"

var frozenTablesDomainLane = stateLaneFactory{
	id: LaneFrozenTables,
	build: func(reg *axis.Registry) stateLaneOps {
		return stateLane(frozenTableDomain(),
			func(s State) frozenTableLane { return s.frozenTables },
			func(out *State, lane frozenTableLane) { out.frozenTables = lane },
		)
	},
}
