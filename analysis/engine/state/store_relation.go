package state

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/lattice/lift"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/internal/mapedit"
)

// StoreRelation records exact evidence that Source is stored into Into on all
// paths reaching this state. Behavior stays modeled by escape and mutation
// facts; this lane preserves ownership relation evidence for summaries.
type StoreRelation struct {
	Source pathdom.PathKey
	Into   pathdom.PathKey
}

type storeRelationLane struct {
	bottom bool
	values map[StoreRelation]struct{}
}

func storeRelationDomain() lattice.Lattice[storeRelationLane] {
	return wrapDomain(lift.MustSet[StoreRelation](), storeRelationLaneFromMustSet, storeRelationLane.asMustSet)
}

func (l storeRelationLane) asMustSet() lift.MustSetLane[StoreRelation] {
	if l.bottom {
		return lift.MustSetBottom[StoreRelation]()
	}
	return lift.MustSetValues(l.values)
}

func storeRelationLaneFromMustSet(l lift.MustSetLane[StoreRelation]) storeRelationLane {
	return storeRelationLane{
		bottom: l.Bottom(),
		values: mapedit.Clone(l.Values()),
	}
}

func (l storeRelationLane) reachable() storeRelationLane {
	l.bottom = false
	return l
}

func (l storeRelationLane) has(relation StoreRelation) bool {
	if l.bottom || relation.Source == "" || relation.Into == "" {
		return false
	}
	_, ok := l.values[relation]
	return ok
}

func (l storeRelationLane) add(relation StoreRelation) (storeRelationLane, bool) {
	if relation.Source == "" || relation.Into == "" {
		return l, false
	}
	if !l.bottom {
		if _, ok := l.values[relation]; ok {
			return l, false
		}
	}
	values := mapedit.Clone(l.values)
	if values == nil {
		values = make(map[StoreRelation]struct{}, 1)
	}
	values[relation] = struct{}{}
	l = l.reachable()
	l.values = values
	return l, true
}

// StoreRelationsSnapshot is a stable snapshot of the finite must-store-relation
// lane. Bottom is explicit; Top means the reachable lane has no guaranteed
// store relations.
type StoreRelationsSnapshot struct {
	Bottom    bool
	Top       bool
	Relations []StoreRelation
}

func (s State) StoreRelationsSnapshot() StoreRelationsSnapshot {
	if s.storeRelations.bottom {
		return StoreRelationsSnapshot{Bottom: true}
	}
	relations := storeRelationsFromSet(s.storeRelations.values)
	return StoreRelationsSnapshot{
		Top:       len(relations) == 0,
		Relations: relations,
	}
}

func (s State) HasStoreRelation(relation StoreRelation) bool {
	return s.storeRelations.has(relation)
}

func (s State) AddStoreRelation(relation StoreRelation) State {
	storeRelations, changed := s.storeRelations.add(relation)
	if !changed {
		return s
	}
	out := s.reachable()
	out.storeRelations = storeRelations
	return out
}

func storeRelationsFromSet(in map[StoreRelation]struct{}) []StoreRelation {
	if len(in) == 0 {
		return nil
	}
	out := make([]StoreRelation, 0, len(in))
	for relation := range in {
		out = append(out, relation)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Source != out[j].Source {
			return out[i].Source < out[j].Source
		}
		return out[i].Into < out[j].Into
	})
	return out
}
