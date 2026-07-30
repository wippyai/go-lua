package state

import (
	"github.com/wippyai/go-lua/__legacy/analysis/domain/lattice/lift"
	"github.com/wippyai/go-lua/__legacy/analysis/internal/mapedit"
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
)

type heapTableIdentityLane struct {
	values map[identity.Term]heapidentity.TableObject
	top    bool
}

func heapTableIdentityLaneFromMap(
	domain lattice.Lattice[map[identity.Term]heapidentity.TableObject],
	values map[identity.Term]heapidentity.TableObject,
) heapTableIdentityLane {
	if domain.Equal(values, domain.Top()) {
		return heapTableIdentityLane{top: true}
	}
	return heapTableIdentityLane{values: values}
}

func (l heapTableIdentityLane) asMap(domain lattice.Lattice[map[identity.Term]heapidentity.TableObject]) map[identity.Term]heapidentity.TableObject {
	if l.top {
		return domain.Top()
	}
	return l.values
}

func (l heapTableIdentityLane) read(reg *axis.Registry, id identity.ID) heapidentity.TableObject {
	return l.readTerm(reg, identity.ConcreteTerm(id))
}

func (l heapTableIdentityLane) readTerm(reg *axis.Registry, term identity.Term) heapidentity.TableObject {
	if !term.Valid() {
		return heapidentity.BottomObject(reg)
	}
	if l.top {
		return heapidentity.TopObject()
	}
	if object, ok := l.values[term]; ok {
		return object
	}
	return heapidentity.BottomObject(reg)
}

func (l heapTableIdentityLane) hasFinite(id identity.ID) bool {
	if l.top {
		return false
	}
	_, ok := l.values[identity.ConcreteTerm(id)]
	return ok
}

func (l heapTableIdentityLane) without(id identity.ID) (heapTableIdentityLane, bool) {
	return l.withoutTerm(identity.ConcreteTerm(id))
}

func (l heapTableIdentityLane) withoutTerm(term identity.Term) (heapTableIdentityLane, bool) {
	values, changed := mapedit.Without(l.values, term)
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
	values := make(map[identity.Term]heapidentity.TableObject, len(l.values))
	for term, object := range l.values {
		rekeyed, ok := object.Rekey(from, to)
		if !ok {
			return l, false
		}
		values[term] = rekeyed
	}
	l.values = values
	return l, true
}

func (l heapTableIdentityLane) with(id identity.ID, object heapidentity.TableObject) heapTableIdentityLane {
	return l.withTerm(identity.ConcreteTerm(id), object)
}

func (l heapTableIdentityLane) withTerm(term identity.Term, object heapidentity.TableObject) heapTableIdentityLane {
	if !term.Valid() {
		return l
	}
	values := mapedit.Clone(l.values)
	if values == nil {
		values = make(map[identity.Term]heapidentity.TableObject, 1)
	}
	values[term] = object
	l.values = values
	return l
}

func heapTermMapDomain(reg *axis.Registry) lattice.Lattice[map[identity.Term]heapidentity.TableObject] {
	return lift.Map[identity.Term, heapidentity.TableObject](heapidentity.ObjectDomain(reg))
}
