package state

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/placement"
)

func (s State) ReadPlacement(id identity.ID) placement.Value {
	if id == (identity.ID{}) {
		return placement.Bottom
	}
	if s.placementTop {
		return placement.Unknown
	}
	if value, ok := s.placement[id]; ok {
		return value
	}
	return placement.Bottom
}

func (s State) WritePlacement(id identity.ID, value placement.Value) State {
	if id == (identity.ID{}) {
		return s
	}
	if s.placementTop {
		panic("state: cannot finite-write placement into top placement lane")
	}
	if value == placement.Bottom {
		placements, changed := deletePlacementEntry(s.placement, id)
		if !changed {
			return s
		}
		out := s.reachable()
		out.placement = placements
		return out
	}
	if existing, ok := s.placement[id]; ok && placement.Equal(existing, value) {
		return s
	}
	placements := clonePlacementMap(s.placement)
	if placements == nil {
		placements = make(map[identity.ID]placement.Value, 1)
	}
	placements[id] = value
	out := s.reachable()
	out.placement = placements
	return out
}
