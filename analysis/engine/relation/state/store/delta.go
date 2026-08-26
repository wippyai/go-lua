package store

import (
	"bytes"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/internal/column"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// ChangeEntry is the state-owned projection of one atomic semantic or
// lineage change. It retains only the private geometry coordinate and exact
// support partition needed by the bounded runtime change consumer; RowID and
// logical Scope remain owned by Mounted and Geometry respectively.
type ChangeEntry struct {
	key             geometry.Key
	region          support.Mask
	beforeValue     binding.ValueToken
	beforePresence  model.Presence
	beforeOK        bool
	beforeLineage   model.LineageRef
	beforeLineageOK bool
	afterValue      binding.ValueToken
	afterPresence   model.Presence
	afterOK         bool
	afterLineage    model.LineageRef
	afterLineageOK  bool
}

func newChangeEntry(entry column.DeltaEntry) (ChangeEntry, bool) {
	if !entry.Region().Valid() || support.Empty(entry.Region()) {
		return ChangeEntry{}, false
	}
	before, beforeOK := entry.Before()
	after, afterOK := entry.After()
	beforeLineage, beforeLineageOK := entry.BeforeLineage()
	afterLineage, afterLineageOK := entry.AfterLineage()
	projected := ChangeEntry{
		key: entry.Key(), region: entry.Region(), beforeOK: beforeOK, afterOK: afterOK,
		beforeLineageOK: beforeLineageOK, afterLineageOK: afterLineageOK,
		beforeLineage: beforeLineage, afterLineage: afterLineage,
	}
	if beforeOK {
		if !before.Available() {
			return ChangeEntry{}, false
		}
		projected.beforeValue = before.Value()
		projected.beforePresence = before.Presence()
		if !projected.beforePresence.Available() || projected.beforePresence.Is(model.Refused) {
			return ChangeEntry{}, false
		}
	}
	if afterOK {
		if !after.Available() {
			return ChangeEntry{}, false
		}
		projected.afterValue = after.Value()
		projected.afterPresence = after.Presence()
		if !projected.afterPresence.Available() || projected.afterPresence.Is(model.Refused) {
			return ChangeEntry{}, false
		}
	}
	if beforeLineageOK && !beforeLineage.Available() || afterLineageOK && !afterLineage.Available() {
		return ChangeEntry{}, false
	}
	if !projected.semanticChanged() && !projected.lineageChanged() {
		return ChangeEntry{}, false
	}
	return projected, true
}

// Key returns the private geometry coordinate for this extent.
func (entry ChangeEntry) Key() geometry.Key { return entry.key }

// Region returns the exact support partition for this extent.
func (entry ChangeEntry) Region() support.Mask { return entry.region }

// Before returns the predecessor sparse semantic cell.
func (entry ChangeEntry) Before() (binding.ValueToken, model.Presence, bool) {
	return entry.beforeValue, entry.beforePresence, entry.beforeOK
}

// After returns the successor sparse semantic cell.
func (entry ChangeEntry) After() (binding.ValueToken, model.Presence, bool) {
	return entry.afterValue, entry.afterPresence, entry.afterOK
}

// BeforeLineage returns the predecessor lineage sidecar. False denotes a
// sparse predecessor under this exact support extent.
func (entry ChangeEntry) BeforeLineage() (model.LineageRef, bool) {
	return entry.beforeLineage, entry.beforeLineageOK
}

// AfterLineage returns the successor lineage sidecar. False denotes a sparse
// successor under this exact support extent.
func (entry ChangeEntry) AfterLineage() (model.LineageRef, bool) {
	return entry.afterLineage, entry.afterLineageOK
}

// SemanticChanged reports whether semantic value or presence changed.
func (entry ChangeEntry) SemanticChanged() bool { return entry.semanticChanged() }

func (entry ChangeEntry) semanticChanged() bool {
	if entry.beforeOK != entry.afterOK {
		return true
	}
	if !entry.beforeOK {
		return false
	}
	if entry.beforePresence != entry.afterPresence {
		return true
	}
	if entry.beforeValue.Available() || entry.afterValue.Available() {
		return entry.beforeValue.Available() && entry.afterValue.Available() && !entry.beforeValue.Same(entry.afterValue)
	}
	return false
}

// LineageChanged reports whether the lineage sidecar changed.
func (entry ChangeEntry) LineageChanged() bool { return entry.lineageChanged() }

func (entry ChangeEntry) lineageChanged() bool {
	if entry.beforeLineageOK != entry.afterLineageOK {
		return true
	}
	return entry.beforeLineageOK && entry.beforeLineage != entry.afterLineage
}

// ColumnChange is the canonical semantic+lineage projection for one changed
// column. It is nonempty for lineage-only replacements and emits one atomic
// extent when semantic and lineage changes overlap. The aggregate roots are
// retained as store-owned immutable handles; no internal column root, RowID,
// or Scope representation crosses this API.
type ColumnChange struct {
	base                Version
	next                Version
	column              model.ColumnID
	relation            model.RelationID
	fence               binding.Fence
	fromRevision        uint64
	toRevision          uint64
	fromLineageRevision uint64
	toLineageRevision   uint64
	entries             []ChangeEntry
	sealed              bool
}

// projectColumnChange converts the complete internal atomic extent stream.
// Every downstream consumer receives this one canonical representation and
// filters entries by SemanticChanged or LineageChanged as needed.
func projectColumnChange(value column.Delta) (ColumnChange, bool) {
	if !value.Available() || value.Empty() {
		return ColumnChange{}, false
	}
	base, next := value.Base(), value.Next()
	projection := ColumnChange{
		column: value.ColumnID(), relation: value.RelationID(), fence: value.Fence(),
		fromRevision: value.FromRevision(), toRevision: value.ToRevision(),
		fromLineageRevision: base.LineageRevision(), toLineageRevision: next.LineageRevision(),
	}
	if !base.Available() || !next.Available() || !next.SuccessorOf(base) || projection.column != base.ID() || projection.column != next.ID() || projection.relation != base.Relation() || projection.relation != next.Relation() || !projection.fence.Same(base.Fence()) {
		return ColumnChange{}, false
	}
	projection.entries = make([]ChangeEntry, 0, value.Len())
	for index := 0; index < value.Len(); index++ {
		entry, ok := value.At(index)
		if !ok {
			return ColumnChange{}, false
		}
		projected, ok := newChangeEntry(entry)
		if !ok {
			return ColumnChange{}, false
		}
		projection.entries = append(projection.entries, projected)
	}
	if len(projection.entries) == 0 || !projection.validProjection() {
		return ColumnChange{}, false
	}
	return projection, true
}

func (delta ColumnChange) validProjection() bool {
	if !delta.column.Available() || !delta.relation.Available() || delta.column.Relation() != delta.relation || !delta.fence.Available() || delta.fromRevision == 0 || delta.toRevision < delta.fromRevision || delta.fromLineageRevision == 0 || delta.toLineageRevision < delta.fromLineageRevision || len(delta.entries) == 0 {
		return false
	}
	for index, entry := range delta.entries {
		if !entry.region.Valid() || support.Empty(entry.region) || (index > 0 && !changeEntryLess(delta.entries[index-1], entry)) {
			return false
		}
		if entry.beforeOK && !entry.beforePresence.Available() || entry.afterOK && !entry.afterPresence.Available() {
			return false
		}
		if entry.beforeOK && entry.beforeValue.Available() && !entry.beforeValue.ValidFor(delta.fence) || entry.afterOK && entry.afterValue.Available() && !entry.afterValue.ValidFor(delta.fence) {
			return false
		}
		if entry.beforeLineageOK && !entry.beforeLineage.Available() || entry.afterLineageOK && !entry.afterLineage.Available() {
			return false
		}
		if !entry.semanticChanged() && !entry.lineageChanged() {
			return false
		}
	}
	return true
}

func changeEntryLess(left, right ChangeEntry) bool {
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

// Available authenticates the unified projection and its exact aggregate
// predecessor/successor roots.
func (delta ColumnChange) Available() bool {
	if delta.sealed {
		return true
	}
	return delta.valid()
}

func (delta ColumnChange) valid() bool {
	if !delta.base.Available() || !delta.next.Available() || !delta.next.SuccessorOf(delta.base) || delta.base.Fence().Same(delta.next.Fence()) == false || delta.base.MountedDigest() != delta.next.MountedDigest() || delta.base.ArrangementDigest() != delta.next.ArrangementDigest() {
		return false
	}
	if !delta.validProjection() {
		return false
	}
	before, beforeOK := delta.base.Column(delta.column)
	after, afterOK := delta.next.Column(delta.column)
	if !beforeOK || !afterOK || !after.SuccessorOf(before) || delta.column != before.ID() || delta.column != after.ID() || delta.fromRevision != before.Revision() || delta.toRevision != after.Revision() || delta.fromLineageRevision != before.LineageRevision() || delta.toLineageRevision != after.LineageRevision() {
		return false
	}
	return true
}

// bindRoots attaches exact aggregate roots after all column projections have
// been created. This keeps the projection immutable without exposing the
// internal column roots used to generate it.
func (delta ColumnChange) bindRoots(base, next Version) (ColumnChange, bool) {
	delta.base, delta.next = base, next
	if !delta.valid() {
		return ColumnChange{}, false
	}
	delta.sealed = true
	return delta, true
}

// Base returns the exact aggregate predecessor retained by this projection.
func (delta ColumnChange) Base() Version {
	if !delta.Available() {
		return Version{}
	}
	return delta.base
}

// Next returns the exact aggregate successor retained by this projection.
func (delta ColumnChange) Next() Version {
	if !delta.Available() {
		return Version{}
	}
	return delta.next
}

func (delta ColumnChange) Empty() bool { return delta.Available() && len(delta.entries) == 0 }

func (delta ColumnChange) ColumnID() model.ColumnID {
	if !delta.Available() {
		return model.ColumnID{}
	}
	return delta.column
}

func (delta ColumnChange) RelationID() model.RelationID {
	if !delta.Available() {
		return model.RelationID{}
	}
	return delta.relation
}

func (delta ColumnChange) Fence() binding.Fence {
	if !delta.Available() {
		return binding.Fence{}
	}
	return delta.fence
}

func (delta ColumnChange) FromRevision() uint64 {
	if !delta.Available() {
		return 0
	}
	return delta.fromRevision
}

func (delta ColumnChange) ToRevision() uint64 {
	if !delta.Available() {
		return 0
	}
	return delta.toRevision
}

func (delta ColumnChange) FromLineageRevision() uint64 {
	if !delta.Available() {
		return 0
	}
	return delta.fromLineageRevision
}

func (delta ColumnChange) ToLineageRevision() uint64 {
	if !delta.Available() {
		return 0
	}
	return delta.toLineageRevision
}

func (delta ColumnChange) Len() int {
	if !delta.Available() {
		return 0
	}
	return len(delta.entries)
}

func (delta ColumnChange) At(index int) (ChangeEntry, bool) {
	if !delta.Available() || index < 0 || index >= len(delta.entries) {
		return ChangeEntry{}, false
	}
	return delta.entries[index], true
}
