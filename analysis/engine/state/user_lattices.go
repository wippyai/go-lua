package state

import (
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/state/userlattice"
)

type UserLatticesSnapshot struct {
	Top    bool
	Values map[userlattice.AxisID]map[pathaddr.StateKey]userlattice.ElementID
}

func (s State) UserLatticesSnapshot(reg *axis.Registry, ks *keyspace.KeySpace) UserLatticesSnapshot {
	if !s.laneEnabled(laneUserLatticesBit) || reg == nil || ks == nil {
		return UserLatticesSnapshot{}
	}
	return s.userLattices.snapshot(userlattice.RuntimeFor(reg), ks)
}

// ReadUserElement reads a user axis element for stateKey. Missing cells denote
// the registered bottom element, returned with ok=true for valid axis/key pairs.
func (s State) ReadUserElement(reg *axis.Registry, ks *keyspace.KeySpace, axisID userlattice.AxisID, stateKey pathaddr.StateKey) (userlattice.ElementID, bool) {
	if !s.laneEnabled(laneUserLatticesBit) || reg == nil || ks == nil || stateKey == "" {
		return "", false
	}
	rt := userlattice.RuntimeFor(reg)
	axis, ok := rt.AxisByID(axisID)
	if !ok {
		return "", false
	}
	key, ok := ks.InternStateKey(stateKey)
	if !ok {
		return "", false
	}
	return axis.ElementName(s.userLattices.read(axis, key)), true
}

// WriteUserElement records a user axis element for stateKey. Writing the
// verified bottom element removes the finite cell.
func (s State) WriteUserElement(reg *axis.Registry, ks *keyspace.KeySpace, axisID userlattice.AxisID, stateKey pathaddr.StateKey, elementID userlattice.ElementID) State {
	if !s.laneEnabled(laneUserLatticesBit) || reg == nil || ks == nil || stateKey == "" {
		return s
	}
	rt := userlattice.RuntimeFor(reg)
	axis, ok := rt.AxisByID(axisID)
	if !ok {
		return s
	}
	elem, ok := axis.Element(elementID)
	if !ok {
		return s
	}
	key, ok := ks.InternStateKey(stateKey)
	if !ok {
		return s
	}
	return s.writeUserElement(axis, key, elem)
}

func (s State) writeUserElement(axis userlattice.Axis, key keyspace.Key, elem userlattice.Element) State {
	lane, changed := s.userLattices.write(axis, key, elem)
	if !changed {
		return s
	}
	out := s.reachable()
	out.userLattices = lane
	return out
}

// ClearUserElement removes any finite user-axis cell for stateKey.
func (s State) ClearUserElement(reg *axis.Registry, ks *keyspace.KeySpace, axisID userlattice.AxisID, stateKey pathaddr.StateKey) State {
	if !s.laneEnabled(laneUserLatticesBit) || reg == nil || ks == nil || stateKey == "" {
		return s
	}
	rt := userlattice.RuntimeFor(reg)
	axis, ok := rt.AxisByID(axisID)
	if !ok {
		return s
	}
	key, ok := ks.InternStateKey(stateKey)
	if !ok {
		return s
	}
	return s.writeUserElement(axis, key, axis.Bottom())
}

// ClearUserElements removes every registered user-axis cell for stateKey.
func (s State) ClearUserElements(reg *axis.Registry, ks *keyspace.KeySpace, stateKey pathaddr.StateKey) State {
	if !s.laneEnabled(laneUserLatticesBit) || reg == nil || ks == nil || stateKey == "" {
		return s
	}
	key, ok := ks.InternStateKey(stateKey)
	if !ok {
		return s
	}
	rt := userlattice.RuntimeFor(reg)
	out := s
	for i := 0; i < rt.Len(); i++ {
		axis := rt.AxisAt(i)
		out = out.writeUserElement(axis, key, axis.Bottom())
	}
	return out
}

// ApplyUserClaim applies a registered on-claim hook to stateKey.
func (s State) ApplyUserClaim(reg *axis.Registry, ks *keyspace.KeySpace, axisID userlattice.AxisID, stateKey pathaddr.StateKey, claim string) State {
	if !s.laneEnabled(laneUserLatticesBit) || reg == nil || ks == nil || stateKey == "" {
		return s
	}
	rt := userlattice.RuntimeFor(reg)
	axis, ok := rt.AxisByID(axisID)
	if !ok {
		return s
	}
	elem, ok := axis.Claim(claim)
	if !ok {
		return s
	}
	key, ok := ks.InternStateKey(stateKey)
	if !ok {
		return s
	}
	return s.writeUserElement(axis, key, elem)
}

// ApplyUserCallBoundary applies each axis's on-call-boundary keep/drop policy
// to every finite user-lattice cell.
func (s State) ApplyUserCallBoundary(reg *axis.Registry) State {
	if !s.laneEnabled(laneUserLatticesBit) || reg == nil {
		return s
	}
	lane, changed := s.userLattices.applyCallBoundary(userlattice.RuntimeFor(reg))
	if !changed {
		return s
	}
	out := s.reachable()
	out.userLattices = lane
	return out
}

// PropagateUserAssignment applies the on-assign hooks from sourceKey to
// targetKey for every registered user axis.
func (s State) PropagateUserAssignment(reg *axis.Registry, ks *keyspace.KeySpace, targetKey, sourceKey pathaddr.StateKey) State {
	return s.PropagateUserAssignmentFrom(reg, ks, targetKey, s, sourceKey)
}

// PropagateUserAssignmentFrom applies assignment hooks while reading sourceKey
// from sourceState and publishing the target writes onto s.
func (s State) PropagateUserAssignmentFrom(reg *axis.Registry, ks *keyspace.KeySpace, targetKey pathaddr.StateKey, sourceState State, sourceKey pathaddr.StateKey) State {
	if !s.laneEnabled(laneUserLatticesBit) || reg == nil || ks == nil || targetKey == "" || sourceKey == "" {
		return s
	}
	target, ok := ks.InternStateKey(targetKey)
	if !ok {
		return s
	}
	source, ok := ks.InternStateKey(sourceKey)
	if !ok {
		return s
	}
	rt := userlattice.RuntimeFor(reg)
	out := s
	for i := 0; i < rt.Len(); i++ {
		axis := rt.AxisAt(i)
		out = out.writeUserElement(axis, target, axis.Assign(sourceState.userLattices.read(axis, source)))
	}
	return out
}
