package column

import (
	"bytes"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// DeltaEntry is one atomic semantic/lineage change over a nonempty support
// region. A false
// presence flag denotes the sparse undefined terminal; it is not a synthesized
// default and cannot be confused with an explicit Cell carrying ProvenAbsent.
type DeltaEntry struct {
	key                  geometry.Key
	region               support.Mask
	before               Cell
	beforePresent        bool
	after                Cell
	afterPresent         bool
	beforeLineage        model.LineageRef
	beforeLineagePresent bool
	afterLineage         model.LineageRef
	afterLineagePresent  bool
}

// Key returns the geometry-issued row key whose atomic region changed.
func (entry DeltaEntry) Key() geometry.Key { return entry.key }

// Region returns the exact support region of this atomic change.
func (entry DeltaEntry) Region() support.Mask { return entry.region }

// Before returns the predecessor semantic cell. False means the sparse
// predecessor had no terminal under this region.
func (entry DeltaEntry) Before() (Cell, bool) { return entry.before, entry.beforePresent }

// After returns the successor semantic cell. False means the sparse successor
// has no terminal under this region.
func (entry DeltaEntry) After() (Cell, bool) { return entry.after, entry.afterPresent }

// BeforeLineage returns the predecessor lineage sidecar. False means the
// predecessor was sparse under this exact support region.
func (entry DeltaEntry) BeforeLineage() (model.LineageRef, bool) {
	return entry.beforeLineage, entry.beforeLineagePresent
}

// AfterLineage returns the successor lineage sidecar. False means the
// successor is sparse under this exact support region.
func (entry DeltaEntry) AfterLineage() (model.LineageRef, bool) {
	return entry.afterLineage, entry.afterLineagePresent
}

// SemanticChanged reports whether the semantic cell changed over this
// extent. Sparse absence remains distinct from an explicit absence Cell.
func (entry DeltaEntry) SemanticChanged() bool {
	if entry.beforePresent != entry.afterPresent {
		return true
	}
	if !entry.beforePresent {
		return false
	}
	return !entry.before.SemanticSame(entry.after)
}

// LineageChanged reports whether the independent lineage sidecar changed over
// this extent.
func (entry DeltaEntry) LineageChanged() bool {
	if entry.beforeLineagePresent != entry.afterLineagePresent {
		return true
	}
	return entry.beforeLineagePresent && entry.beforeLineage != entry.afterLineage
}

// Delta is the publication difference between two immutable column versions.
// entries is the one canonical atomic semantic+lineage stream. A semantic
// consumer filters its entries with DeltaEntry.SemanticChanged; lineage-only
// extents remain present rather than being recovered by a second projection.
type Delta struct {
	base         Version
	next         Version
	fence        binding.Fence
	guards       *guard.Manager
	fromRevision uint64
	toRevision   uint64
	entries      []DeltaEntry
	sealed       bool
}

// Available reports whether this delta carries a valid runtime fence and only
// valid semantic regions/cells. An empty delta is valid when its fence is.
func (delta Delta) Available() bool {
	if delta.sealed {
		return true
	}
	return delta.valid()
}

func (delta Delta) valid() bool {
	if !delta.fence.Available() {
		return false
	}
	if !delta.base.Available() || !delta.next.Available() || delta.base.Column() != delta.next.Column() || !delta.base.Fence().Same(delta.next.Fence()) || !delta.fence.Same(delta.base.Fence()) {
		return false
	}
	if !delta.base.Same(delta.next) && !delta.next.SuccessorOf(delta.base) {
		return false
	}
	if delta.base.Same(delta.next) && delta.fromRevision != delta.toRevision {
		return false
	}
	if delta.fromRevision != delta.base.Revision() || delta.toRevision != delta.next.Revision() || delta.guards == nil || delta.guards != delta.base.Guards() {
		return false
	}
	return validEntries(delta.entries, delta.guards)
}

func validEntries(entries []DeltaEntry, guards *guard.Manager) bool {
	for index, entry := range entries {
		if !entry.region.Valid() || support.Empty(entry.region) || entry.region.Manager() != guards {
			return false
		}
		if index > 0 && !deltaEntryLess(entries[index-1], entry) {
			return false
		}
		if entry.beforePresent && !entry.before.Available() {
			return false
		}
		if entry.afterPresent && !entry.after.Available() {
			return false
		}
		if entry.beforeLineagePresent && !entry.beforeLineage.Available() {
			return false
		}
		if entry.afterLineagePresent && !entry.afterLineage.Available() {
			return false
		}
		if !entry.SemanticChanged() && !entry.LineageChanged() {
			return false
		}
	}
	return true
}

func deltaEntryLess(left, right DeltaEntry) bool {
	if left.key != right.key {
		return left.key < right.key
	}
	leftID, leftOK := left.region.Identity()
	rightID, rightOK := right.region.Identity()
	if !leftOK || !rightOK {
		return false
	}
	return bytes.Compare(leftID[:], rightID[:]) < 0
}

func sealDelta(delta Delta) Delta {
	if delta.valid() {
		delta.sealed = true
	}
	return delta
}

// Empty reports whether no atomic semantic or lineage region changed.
func (delta Delta) Empty() bool { return delta.Available() && len(delta.entries) == 0 }

// Len reports the number of canonical atomic semantic+lineage extents.
func (delta Delta) Len() int { return len(delta.entries) }

// At returns one canonical atomic semantic+lineage extent.
func (delta Delta) At(index int) (DeltaEntry, bool) {
	if index < 0 || index >= len(delta.entries) || !delta.Available() {
		return DeltaEntry{}, false
	}
	return delta.entries[index], true
}

// Fence returns the exact runtime authority of both versions.
func (delta Delta) Fence() binding.Fence { return delta.fence }

// Base returns the exact immutable predecessor root bound by this delta.
func (delta Delta) Base() Version {
	if !delta.Available() {
		return Version{}
	}
	return delta.base
}

// Next returns the exact immutable successor root bound by this delta.
func (delta Delta) Next() Version {
	if !delta.Available() {
		return Version{}
	}
	return delta.next
}

// ColumnID returns the logical column identity authenticated by both roots.
func (delta Delta) ColumnID() model.ColumnID {
	if !delta.Available() {
		return model.ColumnID{}
	}
	return delta.next.Schema().ID()
}

// RelationID returns the logical relation identity authenticated by both roots.
func (delta Delta) RelationID() model.RelationID {
	if !delta.Available() {
		return model.RelationID{}
	}
	return delta.next.Schema().Relation()
}

// FromRevision returns the predecessor semantic revision.
func (delta Delta) FromRevision() uint64 { return delta.fromRevision }

// ToRevision returns the successor semantic revision.
func (delta Delta) ToRevision() uint64 { return delta.toRevision }
