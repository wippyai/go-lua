package store

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/internal/column"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// ReadPart is the public state read projection for one non-empty physical
// support partition.  The internal column implementation remains private;
// operators consume this stable projection instead of importing
// state/internal or manufacturing a second relation store.
type ReadPart struct {
	key      geometry.Key
	region   support.Mask
	column   model.ColumnID
	typeID   model.TypeID
	value    binding.ValueToken
	presence model.Presence
	lineage  model.LineageRef
}

// Key returns the physical row coordinate authenticated by the state.
func (part ReadPart) Key() geometry.Key { return part.key }

// Region returns the exact support partition where the cell holds.
func (part ReadPart) Region() support.Mask { return part.region }

// Column returns the logical column identity owned by this read.
func (part ReadPart) Column() model.ColumnID { return part.column }

// Type returns the owner-issued semantic type of the column.
func (part ReadPart) Type() model.TypeID { return part.typeID }

// Value returns the authenticated semantic token.  Explicit absence carries
// the unavailable zero token; this method never manufactures a default.
func (part ReadPart) Value() binding.ValueToken { return part.value }

// Presence returns the independent logical presence state.
func (part ReadPart) Presence() model.Presence { return part.presence }

// Lineage returns the independent proof-sidecar reference.
func (part ReadPart) Lineage() model.LineageRef { return part.lineage }

// ReadScratch is caller-owned reusable storage for one or more state reads.
// It contains no state roots and is not safe for concurrent use.  Keeping the
// scratch constructor here prevents sibling engine packages from crossing the
// state/internal boundary.
type ReadScratch struct {
	guards *guard.Manager
	inner  *column.ReadScratch
}

// NewReadScratch reserves a reusable read shell for one exact guard manager.
func NewReadScratch(guards *guard.Manager) *ReadScratch {
	if guards == nil || !guards.Valid(guards.True()) {
		return nil
	}
	inner := column.NewReadScratch(guards)
	if inner == nil || !inner.Available() {
		return nil
	}
	return &ReadScratch{guards: guards, inner: inner}
}

// Available reports whether scratch is bound to a complete guard manager.
func (scratch *ReadScratch) Available() bool {
	return scratch != nil && scratch.guards != nil && scratch.guards.Valid(scratch.guards.True()) && scratch.inner != nil && scratch.inner.Available()
}

// Manager returns the exact guard universe captured by scratch. It is used
// only by aggregate construction to prove that the support universe and all
// borrowed state reads share one physical owner.
func (scratch *ReadScratch) Manager() *guard.Manager {
	if !scratch.Available() {
		return nil
	}
	return scratch.guards
}

// Reset releases borrowed partitions while retaining capacities for reuse.
func (scratch *ReadScratch) Reset() {
	if scratch != nil && scratch.inner != nil {
		scratch.inner.Reset()
	}
}

// Read streams one exact column's non-empty partitions under key and within.
// The callback receives state-owned values without exposing the internal
// column package.  A false callback result stops the stream and returns
// (false,true); invalid authority or a failed physical read returns
// (false,false).
func (version Version) Read(id model.ColumnID, key geometry.Key, within support.Mask, scratch *ReadScratch, visit func(ReadPart) bool) (completed, valid bool) {
	if !version.Available() || !id.Available() || scratch == nil || !scratch.Available() || visit == nil || !within.Valid() || within.Manager() != scratch.guards {
		return false, false
	}
	owned, ok := version.Column(id)
	if !ok || !owned.Available() || owned.ID() != id || owned.Guards() != scratch.guards {
		return false, false
	}
	projectionValid := true
	completed, valid = owned.Read(key, within, scratch.inner, func(part column.ReadPart) bool {
		if !part.Region().Valid() || part.Region().Manager() != scratch.guards || !part.Cell().Available() || !part.Lineage().Available() {
			projectionValid = false
			return false
		}
		return visit(ReadPart{
			key: part.Key(), region: part.Region(), column: id,
			typeID: owned.Type(), value: part.Cell().Value(), presence: part.Cell().Presence(), lineage: part.Lineage(),
		})
	})
	if !projectionValid {
		return false, false
	}
	return completed, valid
}

// Scan streams every committed non-empty partition in canonical key order
// for one logical column. Scratch is caller-owned and may be warmed once for
// a zero-allocation hot path. A malformed physical projection fails closed
// rather than being mistaken for an ordinary callback stop.
func (version Version) Scan(id model.ColumnID, within support.Mask, scratch *ReadScratch, visit func(ReadPart) bool) (completed, valid bool) {
	if !version.Available() || !id.Available() || scratch == nil || !scratch.Available() || visit == nil || !within.Valid() || within.Manager() != scratch.guards {
		return false, false
	}
	owned, ok := version.Column(id)
	if !ok || !owned.Available() || owned.ID() != id || owned.Guards() != scratch.guards {
		return false, false
	}
	projectionValid := true
	completed, valid = owned.Scan(within, scratch.inner, func(part column.ReadPart) bool {
		if !part.Region().Valid() || part.Region().Manager() != scratch.guards || !part.Cell().Available() || !part.Lineage().Available() {
			projectionValid = false
			return false
		}
		return visit(ReadPart{
			key: part.Key(), region: part.Region(), column: id,
			typeID: owned.Type(), value: part.Cell().Value(), presence: part.Cell().Presence(), lineage: part.Lineage(),
		})
	})
	if !projectionValid {
		return false, false
	}
	return completed, valid
}
