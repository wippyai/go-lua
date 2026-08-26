package read

import (
	"bytes"
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// RowChange is one callback-borrowed predecessor/successor row transition.
// It is issued only by ChangeReader.ScanChanges
// and must be consumed synchronously by that callback. The two optional rows
// are deliberately independent: a false presence result means that the
// sparse state has no row under this exact changed extent; it does not mint an
// explicit ProvenAbsent cell.
//
// The event retains both exact aggregate roots and both Reader authorities.
// Consequently a row cannot be replaced with a row from a merely equal
// layout, runtime fence, or relation. The physical support region remains
// private to state; Scope is its sole normalized public projection.
type RowChange struct {
	base          database.Version
	next          database.Version
	baseReader    *reader
	nextReader    *reader
	layout        arrangement.Layout
	id            model.RowID
	key           geometry.Key
	region        support.Mask
	scope         witness.Scope
	before        *row
	after         *row
	beforePresent bool
	afterPresent  bool
	semantic      bool
	lineage       bool
}

// Available authenticates both roots, both Reader owners, the exact layout,
// and every optional row against one normalized changed extent.
func (change RowChange) Available() bool {
	if !change.base.Available() || !change.next.Available() || !change.next.SuccessorOf(change.base) ||
		change.baseReader == nil || change.nextReader == nil || !change.baseReader.available() || !change.nextReader.available() ||
		!change.baseReader.root.Same(change.base) || !change.nextReader.root.Same(change.next) ||
		!change.layout.Available() || !change.baseReader.layout.Equal(change.layout) || !change.nextReader.layout.Equal(change.layout) ||
		!change.region.Valid() || support.Empty(change.region) || change.region.Manager() != change.baseReader.manager || change.region.Manager() != change.nextReader.manager ||
		!change.scope.ValidFor(change.baseReader.fence) || !change.scope.ValidFor(change.nextReader.fence) || (!change.semantic && !change.lineage) {
		return false
	}
	normalized, ok := change.baseReader.view.Normalize(change.region)
	if !ok || !normalized.Same(change.scope) || !change.id.Available() || change.id.Relation() != change.layout.Access().Relation() {
		return false
	}
	if change.beforePresent {
		if change.before == nil || !change.before.Available() || change.before.owner != change.baseReader || change.before.id != change.id || change.before.key != change.key || !change.before.mask.Equal(change.region) || !change.before.scope.Same(change.scope) {
			return false
		}
	} else if change.before != nil {
		return false
	}
	if change.afterPresent {
		if change.after == nil || !change.after.Available() || change.after.owner != change.nextReader || change.after.id != change.id || change.after.key != change.key || !change.after.mask.Equal(change.region) || !change.after.scope.Same(change.scope) {
			return false
		}
	} else if change.after != nil {
		return false
	}
	return change.beforePresent || change.afterPresent
}

// Base returns the exact predecessor aggregate root for this transition.
func (change RowChange) Base() database.Version {
	if !change.Available() {
		return database.Version{}
	}
	return change.base
}

// Next returns the exact successor aggregate root for this transition.
func (change RowChange) Next() database.Version {
	if !change.Available() {
		return database.Version{}
	}
	return change.next
}

// Layout returns the exact sealed arrangement redeemed by both sides.
func (change RowChange) Layout() arrangement.Layout {
	if !change.Available() {
		return arrangement.Layout{}
	}
	return change.layout
}

// ID returns the owner-issued logical row identity. It is the Mounted
// directory identity, never a geometry coordinate-derived surrogate.
func (change RowChange) ID() model.RowID {
	if !change.Available() {
		return model.RowID{}
	}
	return change.id
}

// Key returns the arrangement-local physical coordinate authenticated by the
// mounted directory inverse. It is useful only for state-owned diagnostics;
// logical consumers should use ID.
func (change RowChange) Key() geometry.Key {
	if !change.Available() {
		return 0
	}
	return change.key
}

// Scope returns the normalized runtime cofiber for the exact changed extent.
// It is issued by Geometry and is shared by any present Before/After row.
func (change RowChange) Scope() witness.Scope {
	if !change.Available() {
		return witness.Scope{}
	}
	return change.scope
}

// Before returns the predecessor row and its row-level presence. False is a
// sparse undefined row under this extent, not an explicit ProvenAbsent cell.
// A returned row is borrowed until the callback returns.
func (change RowChange) Before() (Row, bool) {
	if !change.Available() || !change.beforePresent {
		return nil, false
	}
	return change.before, true
}

// After returns the successor row and its row-level presence. False is a
// sparse undefined row under this extent, not an explicit ProvenAbsent cell.
// A returned row is borrowed until the callback returns.
func (change RowChange) After() (Row, bool) {
	if !change.Available() || !change.afterPresent {
		return nil, false
	}
	return change.after, true
}

// SemanticChanged reports the exact semantic contribution covering this
// event's support extent. It is not the aggregate Delta flag: a mixed
// semantic/lineage transition can carry either value on different events.
func (change RowChange) SemanticChanged() bool { return change.Available() && change.semantic }

// LineageChanged reports the exact lineage contribution covering this
// event's support extent. It is independent from SemanticChanged.
func (change RowChange) LineageChanged() bool { return change.Available() && change.lineage }

// ScanChanges emits exact base/successor row transitions in canonical
// key/extent order. It reads only the changed extents through the two bound
// Readers; it never invokes Reader.Scan, materializes a replacement cache, or
// supplies a default row for a sparse side. Returning false from visit stops
// with (false,true); malformed authority returns (false,false).
func (handle ChangeReader) ScanChanges(visit func(RowChange) bool) (completed, valid bool) {
	if !handle.available() || visit == nil {
		return false, false
	}
	for _, extent := range handle.frontier {
		changes, ok := handle.rowChangesFor(extent)
		if !ok {
			return false, false
		}
		for _, change := range changes {
			if !change.Available() {
				return false, false
			}
			if !visit(change) {
				return false, true
			}
		}
	}
	return true, true
}

type rowChangePiece struct {
	region support.Mask
	before *row
	after  *row
}

// rowChangesFor expands one coalesced frontier extent into exact paired row
// fibers. The initial rows are already bounded by the extent. Their exact
// support partitions form one finite common refinement; each resulting piece
// is re-read only when a side's original row is wider than that piece. A trim
// must resolve to exactly one row over the requested piece. Refusing any
// non-exact result is preferable to widening a row or manufacturing a sparse
// side.
func (handle ChangeReader) rowChangesFor(extent changeExtent) ([]RowChange, bool) {
	if !handle.available() || !extent.region.Valid() || support.Empty(extent.region) || extent.region.Manager() != handle.base.value.manager || extent.region.Manager() != handle.reader.value.manager {
		return nil, false
	}
	return handle.rowChangesForRegion(extent.key, extent.region, extent.semanticChanged, extent.lineageChanged)
}

func (handle ChangeReader) rowChangesForRegion(key geometry.Key, within support.Mask, semantic, lineage bool) ([]RowChange, bool) {
	if !within.Valid() || support.Empty(within) || (!semantic && !lineage) {
		return nil, false
	}
	baseRows, baseOK := handle.base.value.rowsForChange(key, within)
	if !baseOK {
		return nil, false
	}
	nextRows, nextOK := handle.reader.value.rowsForChange(key, within)
	if !nextOK {
		return nil, false
	}
	if !validRowsForExtent(baseRows, handle.base.value, key, within) || !validRowsForExtent(nextRows, handle.reader.value, key, within) {
		return nil, false
	}
	pieces, piecesOK := pairPieces(baseRows, nextRows)
	if !piecesOK {
		return nil, false
	}
	result := make([]RowChange, 0, len(pieces))
	for _, piece := range pieces {
		before, ok := trimChangeRow(handle.base.value, key, piece.region, piece.before)
		if !ok {
			return nil, false
		}
		after, ok := trimChangeRow(handle.reader.value, key, piece.region, piece.after)
		if !ok {
			return nil, false
		}
		change, ok := handle.makeRowChange(key, piece.region, before, after, semantic, lineage)
		if !ok {
			return nil, false
		}
		result = append(result, change)
	}
	sort.SliceStable(result, func(left, right int) bool {
		leftID, leftOK := result[left].region.Identity()
		rightID, rightOK := result[right].region.Identity()
		if !leftOK || !rightOK {
			return false
		}
		return bytes.Compare(leftID[:], rightID[:]) < 0
	})
	return result, true
}

// trimChangeRow redeems a row on one exact refinement piece. A wider row is
// read again with the narrower support; multiple rows, a missing row, or a
// non-exact mask is malformed rather than an invitation to choose a fallback.
func trimChangeRow(owner *reader, key geometry.Key, region support.Mask, candidate *row) (*row, bool) {
	if candidate == nil {
		return nil, true
	}
	if !candidate.Available() || candidate.owner != owner || candidate.key != key || !candidate.mask.Entails(region) {
		return nil, false
	}
	if candidate.mask.Equal(region) {
		return candidate, true
	}
	rows, ok := owner.rowsForChange(key, region)
	if !ok || len(rows) != 1 || rows[0] == nil || !rows[0].Available() || rows[0].owner != owner || rows[0].key != key || !rows[0].mask.Equal(region) {
		return nil, false
	}
	return rows[0], true
}

func (handle ChangeReader) makeRowChange(key geometry.Key, region support.Mask, before, after *row, semantic, lineage bool) (RowChange, bool) {
	if (before == nil && after == nil) || !region.Valid() || support.Empty(region) || (!semantic && !lineage) || before != nil && (before.owner != handle.base.value || !before.Available()) || after != nil && (after.owner != handle.reader.value || !after.Available()) {
		return RowChange{}, false
	}
	scope, ok := handle.base.value.view.Normalize(region)
	if !ok || !scope.ValidFor(handle.base.value.fence) || !scope.ValidFor(handle.reader.value.fence) {
		return RowChange{}, false
	}
	id := model.RowID{}
	if before != nil {
		id = before.id
	}
	if after != nil {
		if id.Available() && after.id != id {
			return RowChange{}, false
		}
		id = after.id
	}
	if !id.Available() || id.Relation() != handle.reader.value.layout.Access().Relation() {
		return RowChange{}, false
	}
	change := RowChange{
		base: handle.delta.Base(), next: handle.delta.Next(),
		baseReader: handle.base.value, nextReader: handle.reader.value,
		layout: handle.reader.value.layout, id: id, key: key, region: region, scope: scope,
		before: before, after: after, beforePresent: before != nil, afterPresent: after != nil,
		semantic: semantic, lineage: lineage,
	}
	if !change.Available() {
		return RowChange{}, false
	}
	return change, true
}

func validRowsForExtent(rows []*row, owner *reader, key geometry.Key, within support.Mask) bool {
	if owner == nil || !owner.available() || !within.Valid() || within.Manager() != owner.manager {
		return false
	}
	for index, candidate := range rows {
		if candidate == nil || !candidate.Available() || candidate.owner != owner || candidate.key != key || !candidate.mask.Entails(within) {
			return false
		}
		for prior := 0; prior < index; prior++ {
			overlap, ok := support.Intersect(rows[prior].mask, candidate.mask)
			if !ok || !support.Empty(overlap) {
				return false
			}
		}
	}
	return true
}

func pairPieces(baseRows, nextRows []*row) ([]rowChangePiece, bool) {
	pieces := make([]rowChangePiece, 0, len(baseRows)+len(nextRows))
	for _, before := range baseRows {
		remaining := before.mask
		for _, after := range nextRows {
			split, ok := support.Three(remaining, after.mask)
			if !ok {
				return nil, false
			}
			if !support.Empty(split.Overlap()) {
				pieces = append(pieces, rowChangePiece{region: split.Overlap(), before: before, after: after})
			}
			remaining = split.LeftOnly()
			if support.Empty(remaining) {
				break
			}
		}
		if !support.Empty(remaining) {
			pieces = append(pieces, rowChangePiece{region: remaining, before: before})
		}
	}
	for _, after := range nextRows {
		remaining := after.mask
		for _, before := range baseRows {
			split, ok := support.Three(remaining, before.mask)
			if !ok {
				return nil, false
			}
			remaining = split.LeftOnly()
			if support.Empty(remaining) {
				break
			}
		}
		if !support.Empty(remaining) {
			pieces = append(pieces, rowChangePiece{region: remaining, after: after})
		}
	}
	sort.SliceStable(pieces, func(left, right int) bool {
		leftID, leftOK := pieces[left].region.Identity()
		rightID, rightOK := pieces[right].region.Identity()
		if !leftOK || !rightOK {
			return false
		}
		return bytes.Compare(leftID[:], rightID[:]) < 0
	})
	return pieces, true
}
