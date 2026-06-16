package state

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
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
	if _, ok := l.values[id]; !ok {
		return l, false
	}
	out := make(map[identity.ID]placement.Value, len(l.values)-1)
	for k, v := range l.values {
		if k != id {
			out[k] = v
		}
	}
	if len(out) == 0 {
		out = nil
	}
	l.values = out
	return l, true
}

func (l placementLane) with(id identity.ID, value placement.Value) placementLane {
	values := clonePlacementValues(l.values)
	if values == nil {
		values = make(map[identity.ID]placement.Value, 1)
	}
	values[id] = value
	l.values = values
	return l
}

func clonePlacementValues(in map[identity.ID]placement.Value) map[identity.ID]placement.Value {
	if len(in) == 0 {
		return nil
	}
	out := make(map[identity.ID]placement.Value, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
