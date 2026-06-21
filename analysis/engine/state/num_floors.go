package state

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
)

type NumFloorsSnapshot struct {
	Bottom bool
	Floors map[pathdom.PathKey]int64
}

func (s State) NumFloorsSnapshot(ks *keyspace.KeySpace) NumFloorsSnapshot {
	return s.numFloors.snapshot(ks)
}

// ReadNumFloor reads the proven lower bound for a numeric path key: a returned
// (lo, true) asserts value(pathKey) >= lo at this point.
func (s State) ReadNumFloor(ks *keyspace.KeySpace, pathKey pathaddr.StateKey) (int64, bool) {
	key, ok := ks.FromStateKey(pathKey.PathKey())
	if !ok {
		return 0, false
	}
	return s.numFloors.read(key)
}

// WriteNumFloor records that value(pathKey) >= lo holds at this point.
func (s State) WriteNumFloor(ks *keyspace.KeySpace, pathKey pathaddr.StateKey, lo int64) State {
	key, ok := ks.FromStateKey(pathKey.PathKey())
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

// ClearNumFloor removes any finite lower-bound proof for pathKey. It is used
// when a write gives no numeric lower-bound evidence for the new value.
func (s State) ClearNumFloor(ks *keyspace.KeySpace, pathKey pathaddr.StateKey) State {
	key, ok := ks.FromStateKey(pathKey.PathKey())
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
