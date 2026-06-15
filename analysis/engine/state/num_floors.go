package state

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice/lift"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/state/numbound"
)

type NumFloorsSnapshot struct {
	Bottom bool
	Floors map[pathdom.PathKey]int64
}

func (s State) NumFloorsSnapshot() NumFloorsSnapshot {
	out := NumFloorsSnapshot{Bottom: s.numFloors.Bottom()}
	if !out.Bottom {
		values := s.numFloors.Values()
		if len(values) != 0 {
			out.Floors = make(map[pathdom.PathKey]int64, len(values))
			for key, floor := range values {
				out.Floors[key] = floor.Lo
			}
		}
	}
	return out
}

// ReadNumFloor reads the proven lower bound for a numeric path key: a returned
// (lo, true) asserts value(pathKey) >= lo at this point.
func (s State) ReadNumFloor(pathKey pathdom.PathKey) (int64, bool) {
	if pathKey == "" || s.numFloors.Bottom() {
		return 0, false
	}
	floor, ok := s.numFloors.Values()[pathKey]
	if !ok {
		return 0, false
	}
	return floor.Lo, true
}

// WriteNumFloor records that value(pathKey) >= lo holds at this point.
func (s State) WriteNumFloor(pathKey pathdom.PathKey, lo int64) State {
	if pathKey == "" {
		return s
	}
	out := s.reachable()
	floors := cloneNumFloors(out.numFloors.Values())
	if floors == nil {
		floors = make(map[pathdom.PathKey]numbound.Floor, 1)
	}
	if existing, ok := floors[pathKey]; ok && existing.Lo >= lo {
		return s
	}
	floors[pathKey] = numbound.Floor{Lo: lo}
	out.numFloors = lift.MustMapValues(floors)
	return out
}

// ClearNumFloor removes any finite lower-bound proof for pathKey. It is used
// when a write gives no numeric lower-bound evidence for the new value.
func (s State) ClearNumFloor(pathKey pathdom.PathKey) State {
	if pathKey == "" || s.numFloors.Bottom() {
		return s
	}
	floors := cloneNumFloors(s.numFloors.Values())
	if _, ok := floors[pathKey]; !ok {
		return s
	}
	delete(floors, pathKey)
	if len(floors) == 0 {
		floors = nil
	}
	out := s.reachable()
	out.numFloors = lift.MustMapValues(floors)
	return out
}
