package state

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/lattice/lift"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/state/numbound"
	"github.com/wippyai/go-lua/analysis/internal/mapedit"
)

type numFloorLane struct {
	lane lift.MustMapLane[pathdom.PathKey, numbound.Floor]
}

func numFloorLaneFromLift(lane lift.MustMapLane[pathdom.PathKey, numbound.Floor]) numFloorLane {
	return numFloorLane{lane: lane}
}

func (l numFloorLane) asLift() lift.MustMapLane[pathdom.PathKey, numbound.Floor] {
	return l.lane
}

func (l numFloorLane) reachable() numFloorLane {
	if !l.lane.Bottom() {
		return l
	}
	return numFloorLane{lane: lift.MustMapValues(mapedit.Clone(l.lane.Values()))}
}

func (l numFloorLane) snapshot() NumFloorsSnapshot {
	out := NumFloorsSnapshot{Bottom: l.lane.Bottom()}
	if out.Bottom {
		return out
	}
	values := l.lane.Values()
	if len(values) == 0 {
		return out
	}
	out.Floors = make(map[pathdom.PathKey]int64, len(values))
	for key, floor := range values {
		out.Floors[key] = floor.Lo
	}
	return out
}

func (l numFloorLane) read(pathKey pathdom.PathKey) (int64, bool) {
	if pathKey == "" || l.lane.Bottom() {
		return 0, false
	}
	floor, ok := l.lane.Values()[pathKey]
	if !ok {
		return 0, false
	}
	return floor.Lo, true
}

func (l numFloorLane) write(pathKey pathdom.PathKey, lo int64) (numFloorLane, bool) {
	if pathKey == "" {
		return l, false
	}
	if !l.lane.Bottom() {
		if existing, ok := l.lane.Values()[pathKey]; ok && existing.Lo >= lo {
			return l, false
		}
	}
	floors := mapedit.Clone(l.lane.Values())
	if floors == nil {
		floors = make(map[pathdom.PathKey]numbound.Floor, 1)
	}
	floors[pathKey] = numbound.Floor{Lo: lo}
	l.lane = lift.MustMapValues(floors)
	return l, true
}

func (l numFloorLane) clear(pathKey pathdom.PathKey) (numFloorLane, bool) {
	if pathKey == "" || l.lane.Bottom() {
		return l, false
	}
	floors := mapedit.Clone(l.lane.Values())
	if _, ok := floors[pathKey]; !ok {
		return l, false
	}
	delete(floors, pathKey)
	if len(floors) == 0 {
		floors = nil
	}
	l.lane = lift.MustMapValues(floors)
	return l, true
}

func numFloorMapDomain() lattice.Lattice[numFloorLane] {
	return floorMapDomain(numbound.MapDomain(), numFloorLaneFromLift, numFloorLane.asLift)
}
