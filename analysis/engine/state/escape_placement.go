package state

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/engine/state/escapeplacement"
)

func (s State) ReadEscapePlacement(id identity.ID) escapeplacement.Value {
	if id == (identity.ID{}) {
		return escapeplacement.Bottom
	}
	if s.escapePlacementTop {
		return escapeplacement.Unknown
	}
	if placement, ok := s.escapePlacement[id]; ok {
		return placement
	}
	return escapeplacement.Bottom
}

func (s State) WriteEscapePlacement(id identity.ID, placement escapeplacement.Value) State {
	if id == (identity.ID{}) {
		return s
	}
	if s.escapePlacementTop {
		panic("state: cannot finite-write escape placement into top escape-placement lane")
	}
	if placement == escapeplacement.Bottom {
		placements, changed := escapeplacement.DeleteEntry(s.escapePlacement, id)
		if !changed {
			return s
		}
		out := s.reachable()
		out.escapePlacement = placements
		return out
	}
	placements := escapeplacement.CloneMap(s.escapePlacement)
	if placements == nil {
		placements = make(map[identity.ID]escapeplacement.Value, 1)
	}
	placements[id] = placement
	out := s.reachable()
	out.escapePlacement = placements
	return out
}
