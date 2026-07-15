package state

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
)

const LaneTypestates LaneID = "typestates"

var typestatesLaneSpec = laneSpec{
	id:           LaneTypestates,
	keySpaceMode: laneKeySpaceFree,
	fingerprint:  fingerprintTypestates,
	boundary:     boundaryLaneOps{expand: expandTypestatesBoundary, project: projectTypestatesBoundary, rebase: rebaseTypestatesBoundary, apply: applyTypestatesBoundary, equal: equalTypestatesBoundary},
	build: func(reg *axis.Registry, _ DomainOptions) laneOps {
		return stateLane(typestate.Domain,
			func(s State) typestate.Store { return s.typestates },
			func(out *State, store typestate.Store) { out.typestates = store },
		)
	},
}

func expandTypestatesBoundary(expansion *boundaryClosureExpansion, source State) {
	for _, resource := range source.typestates.Resources() {
		if path, ok := expansion.keys.FromStateKey(pathdom.PathKey(resource.ID.String())); ok {
			expansion.connect(path)
		}
	}
}
