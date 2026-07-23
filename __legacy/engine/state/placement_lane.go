package state

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
)

type placementLane struct {
	mapLane[identity.Term, placement.Value]
}

func placementLaneFromMap(
	domain lattice.Lattice[map[identity.Term]placement.Value],
	values map[identity.Term]placement.Value,
) placementLane {
	return placementLane{mapLaneFromMap(domain, values)}
}

func (l placementLane) read(id identity.ID) placement.Value {
	return l.readTerm(identity.ConcreteTerm(id))
}

func (l placementLane) readTerm(term identity.Term) placement.Value {
	if !term.Valid() {
		return placement.Bottom
	}
	if l.isTop() {
		return placement.Unknown
	}
	if value, ok := l.get(term); ok {
		return value
	}
	return placement.Bottom
}

func (l placementLane) without(id identity.ID) (placementLane, bool) {
	return l.withoutTerm(identity.ConcreteTerm(id))
}

func (l placementLane) withoutTerm(term identity.Term) (placementLane, bool) {
	values, changed := l.mapLane.without(term)
	if !changed {
		return l, false
	}
	return placementLane{values}, true
}

func (l placementLane) with(id identity.ID, value placement.Value) placementLane {
	return l.withTerm(identity.ConcreteTerm(id), value)
}

func (l placementLane) withTerm(term identity.Term, value placement.Value) placementLane {
	if !term.Valid() {
		return l
	}
	requireNonBottomLaneValue(value == placement.Bottom, "placement", "placement")
	return placementLane{l.mapLane.with(term, value)}
}
