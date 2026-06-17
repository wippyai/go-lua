package state

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/lattice/lift"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
)

type frozenTableLane struct {
	mustSetLane[identity.ID]
}

func frozenTableDomain() lattice.Lattice[frozenTableLane] {
	return wrapDomain(lift.MustSet[identity.ID](), frozenTableLaneFromMustSet, frozenTableLane.asMustSet)
}

func frozenTableLaneFromMustSet(l lift.MustSetLane[identity.ID]) frozenTableLane {
	return frozenTableLane{mustSetLaneFromLift(l)}
}

func (l frozenTableLane) reachable() frozenTableLane {
	return frozenTableLane{l.mustSetLane.reachable()}
}

func (l frozenTableLane) isFrozen(id identity.ID) bool {
	if id == (identity.ID{}) {
		return false
	}
	return l.contains(id)
}

func (l frozenTableLane) freeze(id identity.ID) (frozenTableLane, bool) {
	if id == (identity.ID{}) {
		return l, false
	}
	lane, changed := l.mustSetLane.insert(id)
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
	if s.frozenTables.bottom {
		return FrozenTablesSnapshot{Bottom: true}
	}
	tables := frozenTableIDsFromSet(s.frozenTables.values)
	return FrozenTablesSnapshot{
		Top:    len(tables) == 0,
		Tables: tables,
	}
}

// IsTableFrozen reports whether every incoming path proves this table identity
// frozen at this point.
func (s State) IsTableFrozen(id identity.ID) bool {
	return s.frozenTables.isFrozen(id)
}

// FreezeTable records a shallow, identity-keyed frozen-table proof.
func (s State) FreezeTable(id identity.ID) State {
	frozenTables, changed := s.frozenTables.freeze(id)
	if !changed {
		return s
	}
	out := s.reachable()
	out.frozenTables = frozenTables
	return out
}

func frozenTableIDsFromSet(in map[identity.ID]struct{}) []identity.ID {
	if len(in) == 0 {
		return nil
	}
	out := make([]identity.ID, 0, len(in))
	for id := range in {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool {
		return identityIDLess(out[i], out[j])
	})
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
