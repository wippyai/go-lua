package state

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"
)

const LaneChannelSelect LaneID = "channel-select"

var channelSelectDomainLane = stateLaneSpec{
	id: LaneChannelSelect,
	markReachable: func(s State) State {
		s.channelSelect = s.channelSelect.Reachable()
		return s
	},
	build: func(reg *axis.Registry) stateLaneOps {
		return stateLane(channelselectfact.Domain(),
			func(s State) channelselectfact.Lane { return s.channelSelect },
			func(out *State, lane channelselectfact.Lane) { out.channelSelect = lane },
		)
	},
}
