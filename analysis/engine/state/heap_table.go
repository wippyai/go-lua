package state

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
)

func (s State) ReadHeapTableObject(reg *axis.Registry, id identity.ID) heapidentity.TableObject {
	if id == (identity.ID{}) {
		return heapidentity.BottomObject(reg)
	}
	if s.heapTableIdentityTop {
		return heapidentity.TopObject()
	}
	if object, ok := s.heapTableIdentity[id]; ok {
		return heapidentity.CloneObject(object)
	}
	return heapidentity.BottomObject(reg)
}

func (s State) WriteHeapTableObject(reg *axis.Registry, id identity.ID, object heapidentity.TableObject) State {
	if id == (identity.ID{}) {
		return s
	}
	if s.heapTableIdentityTop {
		panic("state: cannot finite-write heap table object into top heap-identity lane")
	}
	domain := heapidentity.ObjectDomain(reg)
	if domain.Equal(object, domain.Bottom()) {
		objects, changed := heapidentity.DeleteEntry(s.heapTableIdentity, id)
		if !changed {
			return s
		}
		out := s.reachable()
		out.heapTableIdentity = objects
		return out
	}
	objects := heapidentity.CloneMap(s.heapTableIdentity)
	if objects == nil {
		objects = make(map[identity.ID]heapidentity.TableObject, 1)
	}
	objects[id] = heapidentity.CloneObject(object)
	out := s.reachable()
	out.heapTableIdentity = objects
	return out
}
