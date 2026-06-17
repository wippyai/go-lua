package state

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/lattice/lift"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
)

// StoreRelation records exact evidence that Source is stored into Into on all
// paths reaching this state. Behavior stays modeled by escape and mutation
// facts; this lane preserves ownership relation evidence for summaries.
type StoreRelation struct {
	Source pathdom.PathKey
	Into   pathdom.PathKey
}

type storeRelationLane struct {
	mustSetLane[StoreRelation]
}

func storeRelationDomain() lattice.Lattice[storeRelationLane] {
	return wrapDomain(lift.MustSet[StoreRelation](), storeRelationLaneFromMustSet, storeRelationLane.asMustSet)
}

func storeRelationLaneFromMustSet(l lift.MustSetLane[StoreRelation]) storeRelationLane {
	return storeRelationLane{mustSetLaneFromLift(l)}
}

func (l storeRelationLane) reachable() storeRelationLane {
	return storeRelationLane{l.mustSetLane.reachable()}
}

func (l storeRelationLane) has(relation StoreRelation) bool {
	if relation.Source == "" || relation.Into == "" {
		return false
	}
	return l.contains(relation)
}

func (l storeRelationLane) add(relation StoreRelation) (storeRelationLane, bool) {
	if relation.Source == "" || relation.Into == "" {
		return l, false
	}
	lane, changed := l.mustSetLane.insert(relation)
	return storeRelationLane{lane}, changed
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
