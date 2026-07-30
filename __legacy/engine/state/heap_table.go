package state

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
)

func (s State) ReadHeapTableObject(reg *axis.Registry, id identity.ID) heapidentity.TableObject {
	if !s.laneEnabled(laneHeapTableIdentityBit) {
		return heapidentity.ObjectDomain(reg).Bottom()
	}
	return s.heapTableIdentity.read(reg, id)
}

func (s State) WriteHeapTableObject(reg *axis.Registry, id identity.ID, object heapidentity.TableObject) State {
	if id == (identity.ID{}) || !s.laneEnabled(laneHeapTableIdentityBit) {
		return s
	}
	requireFiniteLaneForWrite(s.heapTableIdentity.top, "finite-write", "heap table object", "heap-identity")
	domain := heapidentity.ObjectDomain(reg)
	if domain.Equal(object, domain.Bottom()) {
		objects, changed := s.heapTableIdentity.without(id)
		if !changed {
			return s
		}
		out := s.reachable()
		out.heapTableIdentity = objects
		return out
	}
	if domain.Equal(s.heapTableIdentity.read(reg, id), object) {
		return s
	}
	out := s.reachable()
	out.heapTableIdentity = s.heapTableIdentity.with(id, object)
	return out
}
