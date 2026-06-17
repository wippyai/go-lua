package state

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
)

type placementLane struct {
	mapLane[identity.ID, placement.Value]
}

func placementLaneFromMap(
	domain lattice.Lattice[map[identity.ID]placement.Value],
	values map[identity.ID]placement.Value,
) placementLane {
	return placementLane{mapLaneFromMap(domain, values)}
}

func (l placementLane) read(id identity.ID) placement.Value {
	if id == (identity.ID{}) {
		return placement.Bottom
	}
	if l.isTop() {
		return placement.Unknown
	}
	if value, ok := l.get(id); ok {
		return value
	}
	return placement.Bottom
}

func (l placementLane) without(id identity.ID) (placementLane, bool) {
	values, changed := l.mapLane.without(id)
	if !changed {
		return l, false
	}
	return placementLane{values}, true
}

func (l placementLane) with(id identity.ID, value placement.Value) placementLane {
	if value == placement.Bottom {
		panic("state: placement lane with requires non-bottom placement")
	}
	return placementLane{l.mapLane.with(id, value)}
}
