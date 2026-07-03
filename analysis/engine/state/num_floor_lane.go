package state

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/lattice/lift"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/engine/state/numbound"
	"github.com/wippyai/go-lua/analysis/internal/mapedit"
)

type numFloorLane struct {
	lane lift.MustMapLane[keyspace.Key, numbound.Floor]
}

func numFloorLaneFromLift(lane lift.MustMapLane[keyspace.Key, numbound.Floor]) numFloorLane {
	return numFloorLane{lane: lane}
}

func (l numFloorLane) asLift() lift.MustMapLane[keyspace.Key, numbound.Floor] {
	return l.lane
}

func (l numFloorLane) reachable() numFloorLane {
	if !l.lane.Bottom() {
		return l
	}
	return numFloorLane{lane: lift.MustMapValues(mapedit.Clone(l.lane.Values()))}
}

func (l numFloorLane) snapshot(ks *keyspace.KeySpace) NumFloorsSnapshot {
	out := NumFloorsSnapshot{Bottom: l.lane.Bottom()}
	if out.Bottom {
		return out
	}
	values := l.lane.Values()
	if len(values) == 0 {
		return out
	}
	out.Floors = make(map[pathaddr.StateKey]int64, len(values))
	for key, floor := range values {
		stateKey, ok := pathaddr.StateKeyFromPathKey(ks.Format(key))
		if !ok {
			continue
		}
		out.Floors[stateKey] = floor.Lo
	}
	return out
}

func (l numFloorLane) read(key keyspace.Key) (int64, bool) {
	if key.Kind == keyspace.KindInvalid || l.lane.Bottom() {
		return 0, false
	}
	floor, ok := l.lane.Values()[key]
	if !ok {
		return 0, false
	}
	return floor.Lo, true
}

func (l numFloorLane) write(key keyspace.Key, lo int64) (numFloorLane, bool) {
	if key.Kind == keyspace.KindInvalid {
		return l, false
	}
	if !l.lane.Bottom() {
		if existing, ok := l.lane.Values()[key]; ok && existing.Lo >= lo {
			return l, false
		}
	}
	floors := mapedit.Clone(l.lane.Values())
	if floors == nil {
		floors = make(map[keyspace.Key]numbound.Floor, 1)
	}
	floors[key] = numbound.Floor{Lo: lo}
	l.lane = lift.MustMapValues(floors)
	return l, true
}

func (l numFloorLane) clear(key keyspace.Key) (numFloorLane, bool) {
	if key.Kind == keyspace.KindInvalid || l.lane.Bottom() {
		return l, false
	}
	floors, changed := mapedit.Without(l.lane.Values(), key)
	if !changed {
		return l, false
	}
	l.lane = lift.MustMapValues(floors)
	return l, true
}

func (l numFloorLane) rekey(from, to *keyspace.KeySpace) numFloorLane {
	if from == nil || to == nil || from == to || l.lane.Bottom() {
		return l
	}
	values := l.lane.Values()
	if len(values) == 0 {
		return l
	}
	rekeyed := make(map[keyspace.Key]numbound.Floor, len(values))
	for key, floor := range values {
		next, ok := to.FromStateKey(from.Format(key))
		if !ok {
			continue
		}
		rekeyed[next] = floor
	}
	return numFloorLane{lane: lift.MustMapValues(rekeyed)}
}

func numFloorMapDomain() lattice.Lattice[numFloorLane] {
	return wrapDomain(numbound.MapDomain(), numFloorLaneFromLift, numFloorLane.asLift)
}
