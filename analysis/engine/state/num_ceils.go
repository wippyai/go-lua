package state

import (
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
)

type NumCeilsSnapshot struct {
	Bottom bool
	Ceils  map[pathaddr.StateKey]int64
}

func (s State) NumCeilsSnapshot(ks *keyspace.KeySpace) NumCeilsSnapshot {
	if !s.laneEnabled(laneNumCeilsBit) {
		return NumCeilsSnapshot{Bottom: true}
	}
	return s.numCeils.snapshot(ks)
}

// ReadNumCeil reads the proven upper bound for a numeric state key: a returned
// (hi, true) asserts value(stateKey) <= hi at this point.
func (s State) ReadNumCeil(ks *keyspace.KeySpace, stateKey pathaddr.StateKey) (int64, bool) {
	if !s.laneEnabled(laneNumCeilsBit) {
		return 0, false
	}
	key, ok := ks.InternStateKey(stateKey)
	if !ok {
		return 0, false
	}
	return s.numCeils.read(key)
}

// WriteNumCeil records that value(stateKey) <= hi holds at this point.
func (s State) WriteNumCeil(ks *keyspace.KeySpace, stateKey pathaddr.StateKey, hi int64) State {
	if !s.laneEnabled(laneNumCeilsBit) {
		return s
	}
	key, ok := ks.InternStateKey(stateKey)
	if !ok {
		return s
	}
	out := s.reachable()
	ceils, changed := out.numCeils.write(key, hi)
	if !changed {
		return s
	}
	out.numCeils = ceils
	return out
}

// ClearNumCeil removes any finite upper-bound proof for stateKey. It is used
// when a write gives no numeric upper-bound evidence for the new value.
func (s State) ClearNumCeil(ks *keyspace.KeySpace, stateKey pathaddr.StateKey) State {
	if !s.laneEnabled(laneNumCeilsBit) {
		return s
	}
	key, ok := ks.InternStateKey(stateKey)
	if !ok {
		return s
	}
	ceils, changed := s.numCeils.clear(key)
	if !changed {
		return s
	}
	out := s.reachable()
	out.numCeils = ceils
	return out
}
