package state

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice/lift"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/state/lenbound"
)

// ReadLenFloor reads the proven length floor for pathKey: a returned (lo, true)
// asserts len(pathKey) >= lo at this point. A missing key or a bottom lane
// reads as no floor.
func (s State) ReadLenFloor(pathKey pathdom.PathKey) (int64, bool) {
	if pathKey == "" || s.lenFloors.Bottom() {
		return 0, false
	}
	floor, ok := s.lenFloors.Values()[pathKey]
	if !ok || floor.Lo <= 0 {
		return 0, false
	}
	return floor.Lo, true
}

// WriteLenFloor records that len(pathKey) >= lo holds at this point, meeting any
// existing floor by keeping the stronger (larger) bound. Writing a non-positive
// floor is a no-op.
func (s State) WriteLenFloor(pathKey pathdom.PathKey, lo int64) State {
	if pathKey == "" || lo <= 0 {
		return s
	}
	out := s.reachable()
	floors := cloneFloors(out.lenFloors.Values())
	if floors == nil {
		floors = make(map[pathdom.PathKey]lenbound.Floor, 1)
	}
	if existing, ok := floors[pathKey]; ok && existing.Lo >= lo {
		return s
	}
	floors[pathKey] = lenbound.Floor{Lo: lo}
	out.lenFloors = lift.MustMapValues(floors)
	return out
}
