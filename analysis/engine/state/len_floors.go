package state

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
)

// ReadLenFloor reads the proven length floor for pathKey: a returned (lo, true)
// asserts len(pathKey) >= lo at this point. A missing key or a bottom lane
// reads as no floor.
func (s State) ReadLenFloor(pathKey pathdom.PathKey) (int64, bool) {
	return s.lenFloors.read(pathKey)
}

// WriteLenFloor records that len(pathKey) >= lo holds at this point, meeting any
// existing floor by keeping the stronger (larger) bound. Writing a non-positive
// floor is a no-op.
func (s State) WriteLenFloor(pathKey pathdom.PathKey, lo int64) State {
	out := s.reachable()
	floors, changed := out.lenFloors.write(pathKey, lo)
	if !changed {
		return s
	}
	out.lenFloors = floors
	return out
}
