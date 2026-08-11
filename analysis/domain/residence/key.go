// Package residence owns the one Link-scoped residence/lifetime relation.
// It does not own escape reachability, ownership duty, continuation liveness,
// allocation policy, or a generic Link boundary union.
package residence

import (
	"github.com/wippyai/go-lua/program/keyspace"
	linkhost "github.com/wippyai/go-lua/program/link/host"
	linkmodule "github.com/wippyai/go-lua/program/link/module"
)

// BoundaryKind is Residence's closed tagged sum of exact structural boundary
// families. Each arm remains its original Link relation.
type BoundaryKind uint8

const (
	BoundaryInvalid BoundaryKind = iota
	BoundaryCapture
	BoundaryStore
	BoundaryReturn
	BoundaryCallback
	BoundarySuspension
	BoundaryTransfer
	BoundaryResume
	BoundaryModuleEntry
	BoundaryModuleCoordinate
	BoundaryGlobal
)

// Key is one owner-issued exact structural boundary. AnalysisRoot is State,
// not key identity; Schema.AdmitsAt supplies its factored admission relation.
type Key struct {
	owner *schema
	id    uint32
}

func (key Key) valid() bool {
	return key.owner != nil && key.id != 0 && int(key.id) <= len(key.owner.boundaries)
}

func (key Key) Kind() BoundaryKind {
	if !key.valid() {
		return BoundaryInvalid
	}
	return key.owner.boundaries[key.id-1].kind
}

// ContentID is the cold stable identity of this exact structural boundary.
func (key Key) ContentID() (keyspace.ContentID, bool) {
	if !key.valid() {
		return keyspace.ContentID{}, false
	}
	return key.owner.boundaries[key.id-1].id, true
}

func (key Key) ModuleEntry() (linkmodule.ModuleCacheEntry, bool) {
	if !key.valid() || key.Kind() != BoundaryModuleEntry {
		return linkmodule.ModuleCacheEntry{}, false
	}
	return key.owner.boundaries[key.id-1].entry, true
}
func (key Key) ModuleCoordinate() (linkmodule.ModuleCoordinate, bool) {
	if !key.valid() || key.Kind() != BoundaryModuleCoordinate {
		return linkmodule.ModuleCoordinate{}, false
	}
	return key.owner.boundaries[key.id-1].coordinate, true
}
func (key Key) Global() (linkhost.GlobalBinding, bool) {
	if !key.valid() || key.Kind() != BoundaryGlobal {
		return linkhost.GlobalBinding{}, false
	}
	return key.owner.boundaries[key.id-1].global, true
}
