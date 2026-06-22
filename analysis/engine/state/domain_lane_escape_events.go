package state

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/state/escapeevent"
)

const LaneEscapeEvents LaneID = "escape-events"

var escapeEventsDomainLane = stateLaneFactory{
	id: LaneEscapeEvents,
	build: func(reg *axis.Registry) stateLaneOps {
		return stateLane(escapeevent.Domain(),
			func(s State) escapeevent.Lane { return s.escapeEvents },
			func(out *State, lane escapeevent.Lane) { out.escapeEvents = lane },
		)
	},
}
