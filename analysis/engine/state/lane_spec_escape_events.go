package state

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/state/escapeevent"
)

const LaneEscapeEvents LaneID = "escape-events"

var escapeEventsLaneSpec = laneSpec{
	id:           LaneEscapeEvents,
	keySpaceMode: laneKeySpaceFree,
	fingerprint:  fingerprintEscapeEvents,
	boundary:     boundaryLaneOps{expand: expandEscapeEventsBoundary},
	markReachable: func(s State) State {
		s.escapeEvents = s.escapeEvents.Reachable()
		return s
	},
	build: func(reg *axis.Registry, _ DomainOptions) laneOps {
		return stateLane(escapeevent.Domain(),
			func(s State) escapeevent.Lane { return s.escapeEvents },
			func(out *State, lane escapeevent.Lane) { out.escapeEvents = lane },
		)
	},
}

func expandEscapeEventsBoundary(expansion *boundaryClosureExpansion, source State) {
	for _, fact := range source.escapeEvents.Snapshot().Facts {
		expansion.connect(expansion.addStateKey(fact.Target))
	}
}
