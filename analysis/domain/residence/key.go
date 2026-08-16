// Package residence owns the one Link-scoped residence/lifetime relation.
package residence

import "github.com/wippyai/go-lua/program/keyspace"

// BoundaryKind is Residence's closed tagged sum of exact structural boundary
// families. The sealed Key exports only its scalar ContentID, never a foreign
// Link coordinate which could retain the Link composition graph.
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
