package state

import (
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

func (s State) ReadPlacement(id identity.ID) placement.Value {
	if !s.laneEnabled(lanePlacementBit) {
		return placement.Bottom
	}
	return s.placement.read(id)
}

func (s State) WritePlacement(id identity.ID, value placement.Value) State {
	if id == (identity.ID{}) || !s.laneEnabled(lanePlacementBit) {
		return s
	}
	if s.placement.isTop() {
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

// PlacementOfValue reads the placement for value's singleton identity. Values
// without an exact identity have no placement fact and read as bottom.
func (s State) PlacementOfValue(reg *axis.Registry, value product.Value) placement.Value {
	id, ok := identityvalue.ExactID(reg, value)
	if !ok {
		return placement.Bottom
	}
	return s.ReadPlacement(id)
}

// ValueHasStackLocalExactIdentity reports whether value has a singleton
// identity whose placement remains confined to the current activation.
func (s State) ValueHasStackLocalExactIdentity(reg *axis.Registry, value product.Value) bool {
	return s.PlacementOfValue(reg, value) == placement.Stack
}

// ValueHasLocalExclusiveExactIdentity reports whether value has a singleton
// identity whose placement proves no external writer can materialize missing
// slots at this state point.
func (s State) ValueHasLocalExclusiveExactIdentity(reg *axis.Registry, value product.Value) bool {
	p := s.PlacementOfValue(reg, value)
	return p == placement.Stack || p == placement.OwnedHeap
}
