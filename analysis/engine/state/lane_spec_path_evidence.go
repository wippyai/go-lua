package state

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
)

const LanePathEvidence LaneID = "path-evidence"

var pathEvidenceLaneSpec = laneSpec{
	id:          LanePathEvidence,
	fingerprint: fingerprintPathEvidence,
	markReachable: func(s State) State {
		s.pathEvidence = s.pathEvidence.Reachable()
		return s
	},
	build: func(reg *axis.Registry, _ DomainOptions) laneOps {
		return stateLane(pathevidence.Domain(reg),
			func(s State) pathevidence.Lane { return s.pathEvidence },
			func(out *State, lane pathevidence.Lane) { out.pathEvidence = lane },
		)
	},
}
