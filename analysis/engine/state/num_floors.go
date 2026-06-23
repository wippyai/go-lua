package state

import (
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
)

type NumFloorsSnapshot struct {
	Bottom bool
	Floors map[pathaddr.StateKey]int64
}

func (s State) NumFloorsSnapshot(ks *keyspace.KeySpace) NumFloorsSnapshot {
	if !s.laneEnabled(laneNumFloorsBit) {
		return NumFloorsSnapshot{Bottom: true}
	}
	return s.numFloors.snapshot(ks)
}

// ReadNumFloor reads the proven lower bound for a numeric state key: a returned
// (lo, true) asserts value(stateKey) >= lo at this point.
func (s State) ReadNumFloor(ks *keyspace.KeySpace, stateKey pathaddr.StateKey) (int64, bool) {
	if !s.laneEnabled(laneNumFloorsBit) {
		return 0, false
	}
	key, ok := ks.InternStateKey(stateKey)
	if !ok {
		return 0, false
	}
	return s.numFloors.read(key)
}

// WriteNumFloor records that value(stateKey) >= lo holds at this point.
func (s State) WriteNumFloor(ks *keyspace.KeySpace, stateKey pathaddr.StateKey, lo int64) State {
	if !s.laneEnabled(laneNumFloorsBit) {
		return s
	}
	key, ok := ks.InternStateKey(stateKey)
	if !ok {
		return s
	}
	out := s.reachable()
	floors, changed := out.numFloors.write(key, lo)
	if !changed {
		return s
	}
	out.numFloors = floors
	return out
}

// ClearNumFloor removes any finite lower-bound proof for stateKey. It is used
// when a write gives no numeric lower-bound evidence for the new value.
func (s State) ClearNumFloor(ks *keyspace.KeySpace, stateKey pathaddr.StateKey) State {
	if !s.laneEnabled(laneNumFloorsBit) {
		return s
	}
	key, ok := ks.InternStateKey(stateKey)
	if !ok {
		return s
	}
	floors, changed := s.numFloors.clear(key)
	if !changed {
		return s
	}
	out := s.reachable()
	out.numFloors = floors
	return out
}
