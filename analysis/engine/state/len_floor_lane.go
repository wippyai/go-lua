package state

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/lattice/lift"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/state/lenbound"
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
	return lenFloorLane{lane: lift.MustMapValues(cloneLenFloors(l.lane.Values()))}
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
	floors := cloneLenFloors(l.lane.Values())
	if floors == nil {
		floors = make(map[pathdom.PathKey]lenbound.Floor, 1)
	}
	floors[pathKey] = lenbound.Floor{Lo: lo}
	l.lane = lift.MustMapValues(floors)
	return l, true
}

func lenFloorMapDomain() lattice.Lattice[lenFloorLane] {
	domain := lenbound.MapDomain()
	return lattice.Lattice[lenFloorLane]{
		Bottom: func() lenFloorLane { return lenFloorLaneFromLift(domain.Bottom()) },
		Top:    func() lenFloorLane { return lenFloorLaneFromLift(domain.Top()) },
		Equal: func(a, b lenFloorLane) bool {
			return domain.Equal(a.asLift(), b.asLift())
		},
		LessOrEq: func(a, b lenFloorLane) bool {
			return domain.LessOrEq(a.asLift(), b.asLift())
		},
		Join: func(a, b lenFloorLane) lenFloorLane {
			return lenFloorLaneFromLift(domain.Join(a.asLift(), b.asLift()))
		},
		Widen: func(prev, next lenFloorLane) lenFloorLane {
			return lenFloorLaneFromLift(domain.Widen(prev.asLift(), next.asLift()))
		},
	}
}

func cloneLenFloors(in map[pathdom.PathKey]lenbound.Floor) map[pathdom.PathKey]lenbound.Floor {
	if len(in) == 0 {
		return nil
	}
	out := make(map[pathdom.PathKey]lenbound.Floor, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
