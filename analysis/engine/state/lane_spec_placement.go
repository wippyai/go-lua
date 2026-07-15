package state

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/lattice/lift"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
)

const LanePlacement LaneID = "placement"

var placementLaneSpec = laneSpec{
	id:           LanePlacement,
	keySpaceMode: laneKeySpaceFree,
	fingerprint:  fingerprintPlacement,
	boundary:     boundaryLaneOps{expand: expandPlacementBoundary, project: projectPlacementBoundary, rebase: rebasePlacementBoundary, apply: applyPlacementBoundary, equal: equalPlacementBoundary},
	build: func(reg *axis.Registry, _ DomainOptions) laneOps {
		domain := placementMapDomain()
		return stateLane(domain,
			func(s State) map[identity.ID]placement.Value {
				return s.placement.asMap(domain)
			},
			func(out *State, placements map[identity.ID]placement.Value) {
				out.placement = placementLaneFromMap(domain, placements)
			},
		)
	},
}

func expandPlacementBoundary(expansion *boundaryClosureExpansion, source State) {
	if !expansion.closure.allIdentities {
		return
	}
	for id := range source.placement.values {
		expansion.addIdentity(id)
	}
}

func placementMapDomain() lattice.Lattice[map[identity.ID]placement.Value] {
	return lift.Map[identity.ID, placement.Value](placement.Lattice())
}
