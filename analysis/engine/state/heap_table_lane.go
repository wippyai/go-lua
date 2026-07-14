package state

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
)

type heapTableIdentityLane struct {
	values map[identity.ID]heapidentity.TableObject
	top    bool
}

func heapTableIdentityLaneFromMap(
	domain lattice.Lattice[map[identity.ID]heapidentity.TableObject],
	values map[identity.ID]heapidentity.TableObject,
) heapTableIdentityLane {
	if domain.Equal(values, domain.Top()) {
		return heapTableIdentityLane{top: true}
	}
	return heapTableIdentityLane{values: values}
}

func (l heapTableIdentityLane) asMap(domain lattice.Lattice[map[identity.ID]heapidentity.TableObject]) map[identity.ID]heapidentity.TableObject {
	if l.top {
		return domain.Top()
	}
	return l.values
}

func (l heapTableIdentityLane) read(reg *axis.Registry, id identity.ID) heapidentity.TableObject {
	if id == (identity.ID{}) {
		return heapidentity.BottomObject(reg)
	}
	if l.top {
		return heapidentity.TopObject()
	}
	if object, ok := l.values[id]; ok {
		return object
	}
	return heapidentity.BottomObject(reg)
}

func (l heapTableIdentityLane) hasFinite(id identity.ID) bool {
	if l.top {
		return false
	}
	_, ok := l.values[id]
	return ok
}

func (l heapTableIdentityLane) without(id identity.ID) (heapTableIdentityLane, bool) {
	values, changed := heapidentity.DeleteEntry(l.values, id)
	if !changed {
		return l, false
	}
	l.values = values
	return l, true
}

func (l heapTableIdentityLane) rekey(from, to *keyspace.KeySpace) (heapTableIdentityLane, bool) {
	if from != nil && !from.Valid() || to != nil && !to.Valid() {
		return l, false
	}
	if l.top || len(l.values) == 0 {
		return l, true
	}
	values := make(map[identity.ID]heapidentity.TableObject, len(l.values))
	for id, object := range l.values {
		rekeyed, ok := object.Rekey(from, to)
		if !ok {
			return l, false
		}
		values[id] = rekeyed
	}
	l.values = values
	return l, true
}

func (l heapTableIdentityLane) with(id identity.ID, object heapidentity.TableObject) heapTableIdentityLane {
	values := heapidentity.CloneMap(l.values)
	if values == nil {
		values = make(map[identity.ID]heapidentity.TableObject, 1)
	}
	values[id] = object
	l.values = values
	return l
}
