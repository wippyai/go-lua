package state

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/lattice/lift"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
)

type frozenTableLane struct {
	bottom bool
	values map[identity.ID]struct{}
}

func frozenTableDomain() lattice.Lattice[frozenTableLane] {
	domain := lift.MustSet[identity.ID]()
	return lattice.Lattice[frozenTableLane]{
		Bottom: func() frozenTableLane {
			return frozenTableLane{bottom: true}
		},
		Top: func() frozenTableLane {
			return frozenTableLane{}
		},
		Equal: func(a, b frozenTableLane) bool {
			return domain.Equal(a.asMustSet(), b.asMustSet())
		},
		LessOrEq: func(a, b frozenTableLane) bool {
			return domain.LessOrEq(a.asMustSet(), b.asMustSet())
		},
		Join: func(a, b frozenTableLane) frozenTableLane {
			return frozenTableLaneFromMustSet(domain.Join(a.asMustSet(), b.asMustSet()))
		},
		Widen: func(prev, next frozenTableLane) frozenTableLane {
			return frozenTableLaneFromMustSet(domain.Widen(prev.asMustSet(), next.asMustSet()))
		},
	}
}

func (l frozenTableLane) asMustSet() lift.MustSetLane[identity.ID] {
	if l.bottom {
		return lift.MustSetBottom[identity.ID]()
	}
	return lift.MustSetValues(l.values)
}

func frozenTableLaneFromMustSet(l lift.MustSetLane[identity.ID]) frozenTableLane {
	return frozenTableLane{
		bottom: l.Bottom(),
		values: cloneFrozenTableSet(l.Values()),
	}
}

func (l frozenTableLane) reachable() frozenTableLane {
	l.bottom = false
	return l
}

func (l frozenTableLane) isFrozen(id identity.ID) bool {
	if id == (identity.ID{}) || l.bottom {
		return false
	}
	_, ok := l.values[id]
	return ok
}

func (l frozenTableLane) freeze(id identity.ID) (frozenTableLane, bool) {
	if id == (identity.ID{}) {
		return l, false
	}
	if !l.bottom {
		if _, ok := l.values[id]; ok {
			return l, false
		}
	}
	values := cloneFrozenTableSet(l.values)
	if values == nil {
		values = make(map[identity.ID]struct{}, 1)
	}
	values[id] = struct{}{}
	l = l.reachable()
	l.values = values
	return l, true
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

func cloneFrozenTableSet(in map[identity.ID]struct{}) map[identity.ID]struct{} {
	if len(in) == 0 {
		return nil
	}
	out := make(map[identity.ID]struct{}, len(in))
	for k := range in {
		out[k] = struct{}{}
	}
	return out
}
