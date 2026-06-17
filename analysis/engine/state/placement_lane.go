package state

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/internal/mapedit"
)

type placementLane struct {
	values map[identity.ID]placement.Value
	top    bool
}

func placementLaneFromMap(
	domain lattice.Lattice[map[identity.ID]placement.Value],
	values map[identity.ID]placement.Value,
) placementLane {
	if domain.Equal(values, domain.Top()) {
		return placementLane{top: true}
	}
	return placementLane{values: values}
}

func (l placementLane) asMap(domain lattice.Lattice[map[identity.ID]placement.Value]) map[identity.ID]placement.Value {
	if l.top {
		return domain.Top()
	}
	return l.values
}

func (l placementLane) read(id identity.ID) placement.Value {
	if id == (identity.ID{}) {
		return placement.Bottom
	}
	if l.top {
		return placement.Unknown
	}
	if value, ok := l.values[id]; ok {
		return value
	}
	return placement.Bottom
}

func (l placementLane) hasFinite(id identity.ID) bool {
	if l.top {
		return false
	}
	_, ok := l.values[id]
	return ok
}

func (l placementLane) without(id identity.ID) (placementLane, bool) {
	values, changed := mapedit.Without(l.values, id)
	if !changed {
		return l, false
	}
	l.values = values
	return l, true
}

func (l placementLane) with(id identity.ID, value placement.Value) placementLane {
	l.values = mapedit.With(l.values, id, value)
	return l
}

func clonePlacementValues(in map[identity.ID]placement.Value) map[identity.ID]placement.Value {
	return mapedit.Clone(in)
}
