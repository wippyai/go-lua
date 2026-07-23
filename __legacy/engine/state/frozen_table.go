package state

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/lattice/lift"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
)

type frozenTableLane struct {
	mustSetLane[identity.Term]
}

func frozenTableDomain() lattice.Lattice[frozenTableLane] {
	return wrapDomain(lift.MustSet[identity.Term](), frozenTableLaneFromMustSet, frozenTableLane.asMustSet)
}

func frozenTableLaneFromMustSet(l lift.MustSetLane[identity.Term]) frozenTableLane {
	return frozenTableLane{mustSetLaneFromLift(l)}
}

func (l frozenTableLane) reachable() frozenTableLane {
	return frozenTableLane{l.mustSetLane.reachable()}
}

func (l frozenTableLane) isFrozen(id identity.ID) bool {
	return l.isFrozenTerm(identity.ConcreteTerm(id))
}

func (l frozenTableLane) isFrozenTerm(term identity.Term) bool {
	if !term.Valid() {
		return false
	}
	return l.contains(term)
}

func (l frozenTableLane) freeze(id identity.ID) (frozenTableLane, bool) {
	return l.freezeTerm(identity.ConcreteTerm(id))
}

func (l frozenTableLane) freezeTerm(term identity.Term) (frozenTableLane, bool) {
	if !term.Valid() {
		return l, false
	}
	lane, changed := l.mustSetLane.insert(term)
	return frozenTableLane{lane}, changed
}

type FrozenTablesSnapshot struct {
	Bottom bool
	Top    bool
	Tables []identity.ID
}

// FrozenTablesSnapshot returns finite must-frozen table identities in stable
// order. Bottom is explicit; Top means the reachable must lane contains no
// frozen-table proofs.
func (s State) FrozenTablesSnapshot() FrozenTablesSnapshot {
	if !s.laneEnabled(laneFrozenTablesBit) {
		return FrozenTablesSnapshot{Bottom: true}
	}
	return frozenTablesSnapshot(s.frozenTables)
}

func frozenTablesSnapshot(lane frozenTableLane) FrozenTablesSnapshot {
	bottom, top, terms := lane.snapshot(identityTermLess)
	tables := make([]identity.ID, 0, len(terms))
	for _, term := range terms {
		id, concrete := term.Concrete()
		if !concrete {
			continue
		}
		tables = append(tables, id)
	}
	return FrozenTablesSnapshot{Bottom: bottom, Top: top, Tables: tables}
}

// IsTableFrozen reports whether every incoming path proves this table identity
// frozen at this point.
func (s State) IsTableFrozen(id identity.ID) bool {
	if !s.laneEnabled(laneFrozenTablesBit) {
		return false
	}
	return s.frozenTables.isFrozen(id)
}

// FreezeTable records a shallow, identity-keyed frozen-table proof.
func (s State) FreezeTable(id identity.ID) State {
	if !s.laneEnabled(laneFrozenTablesBit) {
		return s
	}
	frozenTables, changed := s.frozenTables.freeze(id)
	if !changed {
		return s
	}
	out := s.reachable()
	out.frozenTables = frozenTables
	return out
}

func identityIDLess(a, b identity.ID) bool {
	if a.Kind != b.Kind {
		return a.Kind < b.Kind
	}
	if a.Site != b.Site {
		return a.Site < b.Site
	}
	return a.Index < b.Index
}

func identityTermLess(a, b identity.Term) bool {
	return identity.Less(a, b)
}
