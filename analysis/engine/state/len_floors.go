package state

import (
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
)

// ReadLenFloor reads the proven length floor for pathKey: a returned (lo, true)
// asserts len(pathKey) >= lo at this point. A missing key or a bottom lane
// reads as no floor.
func (s State) ReadLenFloor(ks *keyspace.KeySpace, pathKey pathaddr.StateKey) (int64, bool) {
	key, ok := ks.FromStateKey(pathKey.PathKey())
	if !ok {
		return 0, false
	}
	return s.lenFloors.read(key)
}

// WriteLenFloor records that len(pathKey) >= lo holds at this point, meeting any
// existing floor by keeping the stronger (larger) bound. Writing a non-positive
// floor is a no-op.
func (s State) WriteLenFloor(ks *keyspace.KeySpace, pathKey pathaddr.StateKey, lo int64) State {
	key, ok := ks.FromStateKey(pathKey.PathKey())
	if !ok {
		return s
	}
	out := s.reachable()
	floors, changed := out.lenFloors.write(key, lo)
	if !changed {
		return s
	}
	out.lenFloors = floors
	return out
}
