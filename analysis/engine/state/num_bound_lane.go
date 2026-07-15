package state

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/lattice/lift"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/engine/state/numbound"
	"github.com/wippyai/go-lua/analysis/internal/mapedit"
)

type numBoundLane struct {
	lane lift.MustMapLane[keyspace.Key, int64]
}

func expandNumBoundBoundary(expansion *boundaryClosureExpansion, bounds numBoundLane) {
	if bounds.lane.Bottom() {
		return
	}
	for path := range bounds.lane.Values() {
		expansion.connect(path)
	}
}

func numBoundLaneFromLift(lane lift.MustMapLane[keyspace.Key, int64]) numBoundLane {
	return numBoundLane{lane: lane}
}

func (l numBoundLane) asLift() lift.MustMapLane[keyspace.Key, int64] {
	return l.lane
}

func (l numBoundLane) Reachable() numBoundLane {
	if !l.lane.Bottom() {
		return l
	}
	return numBoundLane{lane: lift.MustMapValues(mapedit.Clone(l.lane.Values()))}
}

func (l numBoundLane) Read(key keyspace.Key) (int64, bool) {
	if key.Kind == keyspace.KindInvalid || l.lane.Bottom() {
		return 0, false
	}
	value, ok := l.lane.Values()[key]
	return value, ok
}

func (l numBoundLane) Write(key keyspace.Key, value int64, direction numbound.Direction) (numBoundLane, bool) {
	if key.Kind == keyspace.KindInvalid {
		return l, false
	}
	if !l.lane.Bottom() {
		if existing, ok := l.lane.Values()[key]; ok && numBoundStrongerOrEqual(direction, existing, value) {
			return l, false
		}
	}
	values := mapedit.Clone(l.lane.Values())
	if values == nil {
		values = make(map[keyspace.Key]int64, 1)
	}
	values[key] = value
	l.lane = lift.MustMapValues(values)
	return l, true
}

func (l numBoundLane) Clear(key keyspace.Key) (numBoundLane, bool) {
	if key.Kind == keyspace.KindInvalid || l.lane.Bottom() {
		return l, false
	}
	values, changed := mapedit.Without(l.lane.Values(), key)
	if !changed {
		return l, false
	}
	l.lane = lift.MustMapValues(values)
	return l, true
}

func numBoundLaneDomain(direction numbound.Direction, thresholds []int64) lattice.Lattice[numBoundLane] {
	bottom, top := maxNumBound, minNumBound
	if direction == numbound.Upper {
		bottom, top = minNumBound, maxNumBound
	}
	return wrapDomain(lift.MustMap[keyspace.Key, int64](numbound.IntDomain(numbound.Spec{
		Direction:  direction,
		Bottom:     bottom,
		Top:        top,
		Thresholds: thresholds,
	})), numBoundLaneFromLift, numBoundLane.asLift)
}

func numBoundSnapshot(lane numBoundLane, ks *keyspace.KeySpace) (bool, map[pathaddr.StateKey]int64) {
	if lane.lane.Bottom() {
		return true, nil
	}
	values := lane.lane.Values()
	if len(values) == 0 {
		return false, nil
	}
	out := make(map[pathaddr.StateKey]int64, len(values))
	for key, value := range values {
		stateKey, ok := pathaddr.StateKeyFromPathKey(ks.Format(key))
		if !ok {
			continue
		}
		out[stateKey] = value
	}
	return false, out
}

func numBoundRekey(lane numBoundLane, from, to *keyspace.KeySpace) (numBoundLane, bool) {
	if from != nil && !from.Valid() || to != nil && !to.Valid() {
		return lane, false
	}
	if lane.lane.Bottom() || len(lane.lane.Values()) == 0 {
		return lane, true
	}
	if from == nil || to == nil {
		return lane, false
	}
	values := lane.lane.Values()
	rekeyed := make(map[keyspace.Key]int64, len(values))
	for key, value := range values {
		next, ok := to.ImportKey(from, key)
		if !ok {
			return lane, false
		}
		rekeyed[next] = value
	}
	return numBoundLane{lane: lift.MustMapValues(rekeyed)}, true
}

const (
	maxNumBound = int64(^uint64(0) >> 1)
	minNumBound = -maxNumBound - 1
)

func numBoundStrongerOrEqual(direction numbound.Direction, existing, next int64) bool {
	return direction == numbound.Upper && existing <= next ||
		direction == numbound.Lower && existing >= next
}
