package state

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
)

const LaneDynamicIndex LaneID = "dynamic-index"

var dynamicIndexLaneSpec = laneSpec{
	id: LaneDynamicIndex,
	build: func(reg *axis.Registry) laneOps {
		domain := dynamicindex.MapDomain(reg)
		return stateLane(domain,
			func(s State) map[dynamicindex.Key]dynamicindex.Fact {
				return s.dynamicIndex.asMap(domain)
			},
			func(out *State, facts map[dynamicindex.Key]dynamicindex.Fact) {
				out.dynamicIndex = dynamicIndexLaneFromMap(domain, facts)
			},
		)
	},
}
