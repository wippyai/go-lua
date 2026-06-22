package state

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
)

const LanePathEvidence LaneID = "path-evidence"

var pathEvidenceDomainLane = stateLaneFactory{
	id: LanePathEvidence,
	build: func(reg *axis.Registry) stateLaneOps {
		return stateLane(pathevidence.Domain(reg),
			func(s State) pathevidence.Lane { return s.pathEvidence },
			func(out *State, lane pathevidence.Lane) { out.pathEvidence = lane },
		)
	},
}
