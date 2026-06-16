package state

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/placement"
)

func (s State) ReadPlacement(id identity.ID) placement.Value {
	return s.placement.read(id)
}

func (s State) WritePlacement(id identity.ID, value placement.Value) State {
	if id == (identity.ID{}) {
		return s
	}
	if s.placement.top {
		panic("state: cannot finite-write placement into top placement lane")
	}
	if value == placement.Bottom {
		placements, changed := s.placement.without(id)
		if !changed {
			return s
		}
		out := s.reachable()
		out.placement = placements
		return out
	}
	if placement.Equal(s.placement.read(id), value) {
		return s
	}
	out := s.reachable()
	out.placement = s.placement.with(id, value)
	return out
}
