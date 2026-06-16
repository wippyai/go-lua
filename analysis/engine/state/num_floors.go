package state

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
)

type NumFloorsSnapshot struct {
	Bottom bool
	Floors map[pathdom.PathKey]int64
}

func (s State) NumFloorsSnapshot() NumFloorsSnapshot {
	return s.numFloors.snapshot()
}

// ReadNumFloor reads the proven lower bound for a numeric path key: a returned
// (lo, true) asserts value(pathKey) >= lo at this point.
func (s State) ReadNumFloor(pathKey pathdom.PathKey) (int64, bool) {
	return s.numFloors.read(pathKey)
}

// WriteNumFloor records that value(pathKey) >= lo holds at this point.
func (s State) WriteNumFloor(pathKey pathdom.PathKey, lo int64) State {
	out := s.reachable()
	floors, changed := out.numFloors.write(pathKey, lo)
	if !changed {
		return s
	}
	out.numFloors = floors
	return out
}

// ClearNumFloor removes any finite lower-bound proof for pathKey. It is used
// when a write gives no numeric lower-bound evidence for the new value.
func (s State) ClearNumFloor(pathKey pathdom.PathKey) State {
	floors, changed := s.numFloors.clear(pathKey)
	if !changed {
		return s
	}
	out := s.reachable()
	out.numFloors = floors
	return out
}
