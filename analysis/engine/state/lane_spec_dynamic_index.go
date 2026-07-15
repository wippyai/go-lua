package state

import (
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
)

const LaneDynamicIndex LaneID = "dynamic-index"

var dynamicIndexLaneSpec = laneSpec{
	id:           LaneDynamicIndex,
	keySpaceMode: laneKeySpaceOwned,
	boundary:     boundaryLaneOps{expand: expandDynamicIndexBoundary, project: projectDynamicIndexBoundary, rebase: rebaseDynamicIndexBoundary, apply: applyDynamicIndexBoundary, equal: equalDynamicIndexBoundary},
	rekey: func(s State, from, to *keyspace.KeySpace) (State, bool) {
		lane, ok := s.dynamicIndex.rekey(from, to)
		if !ok {
			return s, false
		}
		s.dynamicIndex = lane
		return s, true
	},
	fingerprint: fingerprintDynamicIndex,
	build: func(reg *axis.Registry, _ DomainOptions) laneOps {
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

func expandDynamicIndexBoundary(expansion *boundaryClosureExpansion, source State) {
	if source.dynamicIndex.top {
		return
	}
	for factKey, fact := range source.dynamicIndex.values {
		if expansion.connect(factKey.Table) {
			expansion.addValue(fact.KeyValue)
			expansion.addValue(fact.Value)
		}
	}
}
