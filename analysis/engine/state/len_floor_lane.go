package state

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/lattice/lift"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/state/lenbound"
	"github.com/wippyai/go-lua/analysis/internal/mapedit"
)

type lenFloorLane struct {
	lane lift.MustMapLane[pathdom.PathKey, lenbound.Floor]
}

func lenFloorLaneFromLift(lane lift.MustMapLane[pathdom.PathKey, lenbound.Floor]) lenFloorLane {
	return lenFloorLane{lane: lane}
}

func (l lenFloorLane) asLift() lift.MustMapLane[pathdom.PathKey, lenbound.Floor] {
	return l.lane
}

func (l lenFloorLane) reachable() lenFloorLane {
	if !l.lane.Bottom() {
		return l
	}
	return lenFloorLane{lane: lift.MustMapValues(mapedit.Clone(l.lane.Values()))}
}

func (l lenFloorLane) read(pathKey pathdom.PathKey) (int64, bool) {
	if pathKey == "" || l.lane.Bottom() {
		return 0, false
	}
	floor, ok := l.lane.Values()[pathKey]
	if !ok || floor.Lo <= 0 {
		return 0, false
	}
	return floor.Lo, true
}

func (l lenFloorLane) write(pathKey pathdom.PathKey, lo int64) (lenFloorLane, bool) {
	if pathKey == "" || lo <= 0 {
		return l, false
	}
	if !l.lane.Bottom() {
		if existing, ok := l.lane.Values()[pathKey]; ok && existing.Lo >= lo {
			return l, false
		}
	}
	floors := mapedit.Clone(l.lane.Values())
	if floors == nil {
		floors = make(map[pathdom.PathKey]lenbound.Floor, 1)
	}
	floors[pathKey] = lenbound.Floor{Lo: lo}
	l.lane = lift.MustMapValues(floors)
	return l, true
}

func lenFloorMapDomain() lattice.Lattice[lenFloorLane] {
	return wrapDomain(lenbound.MapDomain(), lenFloorLaneFromLift, lenFloorLane.asLift)
}
