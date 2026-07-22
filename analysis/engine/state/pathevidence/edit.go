package pathevidence

import (
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// Edit batches point-local path-evidence writes against one Lane snapshot. It
// is semantically equivalent to repeated Lane write calls, including reachable
// bottom-marker clearing, but clones each value sublane at most once.
type Edit struct {
	lane Lane
	reg  *axis.Registry

	refinements       map[keyspace.KeyHandle]product.Value
	refinementsCloned bool
	staticMembers     map[keyspace.KeyHandle]product.Value
	staticCloned      bool
	changed           bool
}

// EditLane opens a path-evidence edit transaction.
func EditLane(reg *axis.Registry, lane Lane) Edit {
	return Edit{lane: lane, reg: reg}
}

// ReadPathKey reads the current value for pathKey, including staged writes.
func (e *Edit) ReadPathKey(pathKey keyspace.Key) product.Value {
	if pathKey.Kind == keyspace.KindInvalid {
		return product.Bottom(e.reg)
	}
	if e.refinementsCloned {
		if value, ok := e.refinements[pathKey.Handle()]; ok {
			return value
		}
		return product.Bottom(e.reg)
	}
	return e.lane.ReadPathKey(e.reg, pathKey)
}

// WritePathKey stages a path-refinement write.
func (e *Edit) WritePathKey(pathKey keyspace.Key, value product.Value) bool {
	if pathKey.Kind == keyspace.KindInvalid {
		return false
	}
	valueDomain := product.Domain(e.reg)
	bottom := valueDomain.Bottom()
	current := e.ReadPathKey(pathKey)
	if valueDomain.Equal(value, bottom) {
		if valueDomain.Equal(current, bottom) && !e.lane.refinementsBottom {
			return false
		}
		e.ensureRefinements()
		delete(e.refinements, pathKey.Handle())
		e.markReachable()
		return true
	}
	if valueDomain.Equal(current, value) {
		return false
	}
	e.ensureRefinements()
	if e.refinements == nil {
		e.refinements = make(map[keyspace.KeyHandle]product.Value, 1)
	}
	e.refinements[pathKey.Handle()] = value
	e.markReachable()
	return true
}

// WritePathStaticMember stages a static-member evidence write.
func (e *Edit) WritePathStaticMember(pathKey keyspace.Key, value product.Value) bool {
	if pathKey.Kind == keyspace.KindInvalid {
		return false
	}
	if !e.lane.staticMembersBottom {
		if e.staticCloned {
			if existing, ok := e.staticMembers[pathKey.Handle()]; ok && existing == value {
				return false
			}
		} else if existing, ok := e.lane.staticMembers[pathKey.Handle()]; ok && existing == value {
			return false
		}
	}
	e.ensureStaticMembers()
	if e.staticMembers == nil {
		e.staticMembers = make(map[keyspace.KeyHandle]product.Value, 1)
	}
	e.staticMembers[pathKey.Handle()] = value
	e.markReachable()
	return true
}

// Done returns the original lane if no effective write occurred, otherwise the
// lane with the staged path-evidence maps published.
func (e *Edit) Done() (Lane, bool) {
	if !e.changed {
		return e.lane, false
	}
	if e.refinementsCloned {
		e.lane.refinements = e.refinements
	}
	if e.staticCloned {
		e.lane.staticMembers = e.staticMembers
	}
	return e.lane, true
}

func (e *Edit) ensureRefinements() {
	if e.refinementsCloned {
		return
	}
	e.refinements = cloneLocalValueHandleMap(e.lane.refinements)
	e.refinementsCloned = true
}

func (e *Edit) ensureStaticMembers() {
	if e.staticCloned {
		return
	}
	e.staticMembers = cloneLocalValueHandleMap(e.lane.staticMembers)
	e.staticCloned = true
}

func (e *Edit) markReachable() {
	e.lane = e.lane.Reachable()
	e.changed = true
}
