package factapply

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/engine/state"
)

// ChannelSelectLaneContract is the exhaustive registered-axis footprint of
// ApplyChannelSelect. Values dependencies are carried separately by the
// transaction's finite slot inventory; this contract owns the residual State
// product only.
type ChannelSelectLaneContract struct {
	lanes  state.LaneSet
	reads  state.LaneSet
	writes state.LaneSet
}

// SealChannelSelectLaneContract classifies every enabled registered axis.
// Unknown and duplicate lanes fail closed, so catalog growth cannot silently
// turn an opaque prefix transfer into an incomplete projection.
func SealChannelSelectLaneContract(lanes state.LaneSet) (ChannelSelectLaneContract, error) {
	seen := make(map[state.LaneID]struct{}, lanes.Len())
	reads := make([]state.LaneID, 0, 1)
	writes := make([]state.LaneID, 0, 1)
	for _, lane := range lanes.IDs() {
		if _, duplicate := seen[lane]; duplicate {
			return ChannelSelectLaneContract{}, fmt.Errorf("factapply: duplicate channel-select lane %q", lane)
		}
		seen[lane] = struct{}{}
		read, write, known := channelSelectLaneUse(lane)
		if !known {
			return ChannelSelectLaneContract{}, fmt.Errorf("factapply: unclassified channel-select lane %q", lane)
		}
		if read {
			reads = append(reads, lane)
		}
		if write {
			writes = append(writes, lane)
		}
	}
	if len(seen) == 0 {
		return ChannelSelectLaneContract{}, fmt.Errorf("factapply: channel-select lane inventory is empty")
	}
	return ChannelSelectLaneContract{
		lanes: state.NewLaneSet(lanes.IDs()...), reads: state.NewLaneSet(reads...), writes: state.NewLaneSet(writes...),
	}, nil
}

// ReadLanes returns the exact residual State axes read by channel publication.
func (c ChannelSelectLaneContract) ReadLanes() (state.LaneSet, bool) {
	if c.lanes.Len() == 0 {
		return state.LaneSet{}, false
	}
	return state.NewLaneSet(c.reads.IDs()...), true
}

// WriteLanes returns the exact residual State axes changed by channel publication.
func (c ChannelSelectLaneContract) WriteLanes() (state.LaneSet, bool) {
	if c.lanes.Len() == 0 {
		return state.LaneSet{}, false
	}
	return state.NewLaneSet(c.writes.IDs()...), true
}

func channelSelectLaneUse(lane state.LaneID) (read, write, known bool) {
	switch lane {
	case state.LaneChannelSelect:
		// AddChannelSelectFact is a must-fact insertion over the existing lane.
		return true, true, true
	case state.LaneValues,
		state.LanePathEvidence,
		state.LaneDynamicIndex,
		state.LaneHeapTableIdentity,
		state.LaneFrozenTables,
		state.LaneEffectDeltas,
		state.LaneEscapeEvents,
		state.LaneStoreRelations,
		state.LaneKeyMemberships,
		state.LaneTypestates,
		state.LanePlacement,
		state.LaneLenFloors,
		state.LaneNumFloors,
		state.LaneNumCeils,
		state.LaneDiffRelations,
		state.LaneUserLattices:
		return false, false, true
	default:
		return false, false, false
	}
}
