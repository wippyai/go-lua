package state

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"
)

const LaneChannelSelect LaneID = "channel-select"

var channelSelectLaneSpec = laneSpec{
	id:           LaneChannelSelect,
	keySpaceMode: laneKeySpaceFree,
	fingerprint:  fingerprintChannelSelect,
	boundary:     boundaryLaneOps{expand: expandChannelSelectBoundary},
	markReachable: func(s State) State {
		s.channelSelect = s.channelSelect.Reachable()
		return s
	},
	build: func(reg *axis.Registry, _ DomainOptions) laneOps {
		return stateLane(channelselectfact.Domain(),
			func(s State) channelselectfact.Lane { return s.channelSelect },
			func(out *State, lane channelselectfact.Lane) { out.channelSelect = lane },
		)
	},
}

func expandChannelSelectBoundary(expansion *boundaryClosureExpansion, source State) {
	for _, fact := range source.channelSelect.Snapshot().Facts {
		if expansion.connect(expansion.addStateKey(fact.Result), expansion.addStateKey(fact.Case)) && fact.HasPayload {
			expansion.addValue(fact.Payload)
		}
	}
}
