package read

import (
	"bytes"
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/store"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// ChangeReader is the differential counterpart of Reader.  It owns the
// predecessor and successor Readers and a private, disjoint physical frontier
// derived from one exact database Delta. ScanChanges is its only enumeration
// surface: a successor-only scan cannot represent removal or replacement and
// would create a second, lossy differential ABI.
//
// The frontier is deliberately retained as support masks only inside state.
// Row identity is recovered from Mounted and logical scope is issued by
// Geometry when the frontier is consumed.
type ChangeReader struct {
	delta    database.Delta
	base     Reader
	reader   Reader
	frontier []changeExtent
}

type changeExtent struct {
	key             geometry.Key
	region          support.Mask
	semanticChanged bool
	lineageChanged  bool
}

// BindChanges authenticates one exact aggregate transition, sealed layout,
// geometry, and reusable read scratch.  It binds the successor Reader once,
// then projects only change extents relevant to that layout and relation.
//
// No index scan, full Reader scan, schema reopening, or old ColumnDelta
// compatibility path is used.  A valid delta with no relevant extents yields
// an available reader whose ScanChanges completes with no rows.
func BindChanges(delta database.Delta, layout arrangement.Layout, view geometry.Geometry, scratch *store.ReadScratch) (ChangeReader, bool) {
	if !delta.Available() || !layout.Available() || !view.Available() || scratch == nil || !scratch.Available() {
		return ChangeReader{}, false
	}
	base, next := delta.Base(), delta.Next()
	if !base.Available() || !next.Available() || !next.SuccessorOf(base) || !delta.Source().Available() {
		return ChangeReader{}, false
	}
	if !delta.Source().Base().Same(base.Store()) || !delta.Source().Next().Same(next.Store()) || !delta.Source().Base().Fence().Same(base.Fence()) {
		return ChangeReader{}, false
	}
	mounted := next.Mounted()
	if !mounted.Available() || !view.ValidFor(mounted) || !layout.ValidFor(mounted.Fence()) || !delta.Base().Fence().Same(next.Fence()) || scratch.Manager() != view.Manager() {
		return ChangeReader{}, false
	}
	// Bind is the one owner door for both sides of a row transition. In
	// particular, it proves that layout is the exact sealed arrangement member
	// of each aggregate root before any change extent is admitted. Keeping the
	// two Readers here is important: a terminal replacement/removal cannot be
	// recovered from the successor side alone.
	predecessor, ok := Bind(base, layout, view, scratch)
	if !ok || !predecessor.Available() || !predecessor.value.root.Same(base) {
		return ChangeReader{}, false
	}
	successor, ok := Bind(next, layout, view, scratch)
	if !ok || !successor.Available() || !successor.value.root.Same(next) {
		return ChangeReader{}, false
	}
	frontier, ok := changeFrontier(delta, layout, view.Manager())
	if !ok {
		return ChangeReader{}, false
	}
	result := ChangeReader{delta: delta, base: predecessor, reader: successor, frontier: frontier}
	if !result.available() {
		return ChangeReader{}, false
	}
	return result, true
}

// Available reports whether this handle still owns the exact predecessor,
// successor, layout, fence, geometry, scratch, and frontier it redeemed.
func (handle ChangeReader) Available() bool {
	return handle.available()
}

func (handle ChangeReader) available() bool {
	if !handle.delta.Available() || !handle.base.Available() || !handle.reader.Available() || handle.base.value == nil || handle.reader.value == nil || !handle.base.value.root.Available() || !handle.reader.value.root.Available() {
		return false
	}
	base, next := handle.delta.Base(), handle.delta.Next()
	if !base.Available() || !next.Available() || !next.SuccessorOf(base) || !handle.base.value.root.Same(base) || !handle.reader.value.root.Same(next) || !handle.base.Layout().Equal(handle.reader.Layout()) {
		return false
	}
	source := handle.delta.Source()
	if !source.Available() || !source.Base().Same(base.Store()) || !source.Next().Same(next.Store()) || !source.Base().Fence().Same(base.Fence()) {
		return false
	}
	if len(handle.frontier) == 0 {
		return true
	}
	manager := handle.reader.value.manager
	if manager == nil {
		return false
	}
	var priorKey geometry.Key
	var priorRegion support.Mask
	for index, extent := range handle.frontier {
		if !extent.region.Valid() || extent.region.Manager() != manager || support.Empty(extent.region) || (!extent.semanticChanged && !extent.lineageChanged) {
			return false
		}
		if index > 0 {
			if extent.key < priorKey {
				return false
			}
			if extent.key == priorKey {
				overlap, ok := support.Intersect(priorRegion, extent.region)
				if !ok || !support.Empty(overlap) {
					return false
				}
			}
		}
		priorKey, priorRegion = extent.key, extent.region
	}
	return true
}

// Delta returns the exact aggregate transition retained by this reader.
func (handle ChangeReader) Delta() database.Delta {
	if !handle.available() {
		return database.Delta{}
	}
	return handle.delta
}

// Base returns the exact predecessor root of Delta.
func (handle ChangeReader) Base() database.Version {
	if !handle.available() {
		return database.Version{}
	}
	return handle.delta.Base()
}

// Next returns the exact successor root retained by this transition reader.
func (handle ChangeReader) Next() database.Version {
	if !handle.available() {
		return database.Version{}
	}
	return handle.delta.Next()
}

// Reader returns the canonical successor Reader that owns every Row emitted
// by this handle.  It is useful at tuple.Input boundaries and does not expose
// a second row representation.
func (handle ChangeReader) Reader() Reader {
	if !handle.available() {
		return Reader{}
	}
	return handle.reader
}

// BaseReader returns the exact predecessor Reader that owns every Before row
// issued by ScanChanges.  It is a capability projection, not a new reader or
// a rescan door; callers use it to copy a trimmed callback row into their own
// immutable representation before the callback returns.
func (handle ChangeReader) BaseReader() Reader {
	if !handle.available() {
		return Reader{}
	}
	return handle.base
}

// Layout returns the exact sealed layout redeemed by the successor Reader.
func (handle ChangeReader) Layout() arrangement.Layout {
	if !handle.available() {
		return arrangement.Layout{}
	}
	return handle.reader.Layout()
}

// changeFrontier filters the canonical ColumnChange stream and partitions the
// relevant masks into a deterministic disjoint cover.  Each column's atomic
// entries are considered once; overlapping entries from several columns are
// coalesced by exact support algebra rather than by a row or scope hash.
func changeFrontier(delta database.Delta, layout arrangement.Layout, manager *guard.Manager) ([]changeExtent, bool) {
	if !delta.Available() || !layout.Available() || manager == nil || !manager.Valid(manager.True()) {
		return nil, false
	}
	relation := layout.Access().Relation()
	keyColumns := layout.KeyColumns()
	delivered := layout.Columns()
	relationDirectory := len(keyColumns) == 0 && len(delivered) == 0
	relevantColumns := make(map[model.ColumnID]struct{}, len(keyColumns)+len(delivered))
	for _, column := range keyColumns {
		if !column.Available() || column.Relation() != relation {
			return nil, false
		}
		relevantColumns[column] = struct{}{}
	}
	for _, column := range delivered {
		if !column.Available() || column.Relation() != relation {
			return nil, false
		}
		relevantColumns[column] = struct{}{}
	}
	base, next := delta.Base(), delta.Next()
	if !base.Available() || !next.Available() || !next.SuccessorOf(base) {
		return nil, false
	}
	frontier := make([]changeExtent, 0)
	for _, change := range delta.Changes() {
		if !change.Available() || !change.Base().Same(base.Store()) || !change.Next().Same(next.Store()) || !change.Fence().Same(base.Fence()) {
			return nil, false
		}
		if change.RelationID() != relation {
			continue
		}
		if !relationDirectory {
			if _, relevant := relevantColumns[change.ColumnID()]; !relevant {
				continue
			}
		}
		for position := 0; position < change.Len(); position++ {
			entry, ok := change.At(position)
			if !ok || !entry.Region().Valid() || support.Empty(entry.Region()) || entry.Region().Manager() != manager || (!entry.SemanticChanged() && !entry.LineageChanged()) {
				return nil, false
			}
			var mergeOK bool
			frontier, mergeOK = mergeChangeExtent(frontier, changeExtent{
				key: entry.Key(), region: entry.Region(),
				semanticChanged: entry.SemanticChanged(), lineageChanged: entry.LineageChanged(),
			})
			if !mergeOK {
				return nil, false
			}
		}
	}
	sort.SliceStable(frontier, func(left, right int) bool {
		if frontier[left].key != frontier[right].key {
			return frontier[left].key < frontier[right].key
		}
		leftID, leftOK := frontier[left].region.Identity()
		rightID, rightOK := frontier[right].region.Identity()
		if !leftOK || !rightOK {
			return false
		}
		return bytes.Compare(leftID[:], rightID[:]) < 0
	})
	return frontier, true
}

// mergeChangeExtent overlays one atomic changed support with the existing
// same-key frontier. Existing and incoming flags are ORed only over their
// exact overlap; semantic-only and lineage-only contributions therefore
// remain distinct event extents instead of being mislabeled by an aggregate
// flag. Same-flag pieces are coalesced again after the overlay so a
// multi-column semantic change retains the historical one-row frontier.
func mergeChangeExtent(frontier []changeExtent, incoming changeExtent) ([]changeExtent, bool) {
	if !incoming.region.Valid() || support.Empty(incoming.region) || (!incoming.semanticChanged && !incoming.lineageChanged) {
		return nil, false
	}
	remaining := incoming.region
	result := make([]changeExtent, 0, len(frontier)+1)
	for _, prior := range frontier {
		if prior.key != incoming.key || support.Empty(remaining) {
			result = append(result, prior)
			continue
		}
		split, ok := support.Three(prior.region, remaining)
		if !ok {
			return nil, false
		}
		if !support.Empty(split.LeftOnly()) {
			left := prior
			left.region = split.LeftOnly()
			result = append(result, left)
		}
		if !support.Empty(split.Overlap()) {
			overlap := prior
			overlap.region = split.Overlap()
			overlap.semanticChanged = prior.semanticChanged || incoming.semanticChanged
			overlap.lineageChanged = prior.lineageChanged || incoming.lineageChanged
			result = append(result, overlap)
		}
		remaining = split.RightOnly()
	}
	if !support.Empty(remaining) {
		incoming.region = remaining
		result = append(result, incoming)
	}
	return coalesceChangeExtents(result)
}

func coalesceChangeExtents(frontier []changeExtent) ([]changeExtent, bool) {
	result := make([]changeExtent, 0, len(frontier))
	for _, candidate := range frontier {
		if !candidate.region.Valid() || support.Empty(candidate.region) || (!candidate.semanticChanged && !candidate.lineageChanged) {
			return nil, false
		}
		merged := false
		for index := range result {
			prior := &result[index]
			if prior.key != candidate.key || prior.semanticChanged != candidate.semanticChanged || prior.lineageChanged != candidate.lineageChanged {
				continue
			}
			union, ok := support.Union(prior.region, candidate.region)
			if !ok {
				return nil, false
			}
			prior.region = union
			merged = true
			break
		}
		if !merged {
			result = append(result, candidate)
		}
	}
	return result, true
}

// rowsForChange reads only the successor state at one changed key and one
// disjoint frontier mask.  It refines the frontier with every column needed
// to prove a complete successor row, while exposing only the layout's
// delivered columns as Cells.
func (value *reader) rowsForChange(key geometry.Key, within support.Mask) ([]*row, bool) {
	if !value.available() || !within.Valid() || within.Manager() != value.manager || support.Empty(within) {
		return nil, false
	}
	relation := value.layout.Access().Relation()
	id, ok := changedRowAt(value.mounted, relation, key)
	if !ok {
		return nil, false
	}
	keyColumns := value.layout.KeyColumns()
	delivered := value.layout.Columns()
	if len(keyColumns) == 0 && len(delivered) == 0 {
		return value.directoryRowsForChange(key, within, id)
	}
	allColumns := make([]model.ColumnID, 0, len(keyColumns)+len(delivered))
	seenColumns := make(map[model.ColumnID]struct{}, len(keyColumns)+len(delivered))
	for _, column := range keyColumns {
		if _, exists := seenColumns[column]; exists {
			continue
		}
		seenColumns[column] = struct{}{}
		allColumns = append(allColumns, column)
	}
	for _, column := range delivered {
		if _, exists := seenColumns[column]; exists {
			continue
		}
		seenColumns[column] = struct{}{}
		allColumns = append(allColumns, column)
	}
	fibers, ok := value.commonFibersFor(key, within, allColumns)
	if !ok || len(fibers) == 0 {
		return nil, ok
	}
	rows := make([]*row, 0, len(fibers))
	for _, fiber := range fibers {
		if len(fiber.parts) != len(allColumns) {
			return nil, false
		}
		scope, scopeOK := value.view.Normalize(fiber.region)
		if !scopeOK || !scope.ValidFor(value.fence) {
			return nil, false
		}
		positions := make(map[model.ColumnID]int, len(allColumns))
		for index, column := range allColumns {
			positions[column] = index
		}
		cells := make([]Cell, len(delivered))
		for index, column := range delivered {
			partPosition, partOK := positions[column]
			if !partOK || partPosition < 0 || partPosition >= len(fiber.parts) {
				return nil, false
			}
			part := fiber.parts[partPosition]
			cells[index] = Cell{
				owner: value, column: part.Column(), typeID: part.Type(), value: part.Value(),
				presence: part.Presence(), scope: scope, lineage: part.Lineage(),
			}
		}
		var rowLineage model.LineageRef
		if len(delivered) == 0 {
			rowLineage, ok = joinPartsLineage(value, fiber.parts)
		} else {
			rowLineage, ok = value.joinLineage(key, fiber.region, cells)
		}
		if !ok {
			return nil, false
		}
		candidate := &row{owner: value, id: id, key: key, mask: fiber.region, scope: scope, lineage: rowLineage, cells: cells}
		if !candidate.Available() {
			return nil, false
		}
		rows = append(rows, candidate)
	}
	sortRows(rows)
	return rows, true
}

func (value *reader) directoryRowsForChange(key geometry.Key, within support.Mask, id model.RowID) ([]*row, bool) {
	relation := value.layout.Access().Relation()
	var membership support.Mask
	foundColumn := false
	for _, declaration := range value.mounted.Columns() {
		if declaration.Relation() != relation {
			continue
		}
		foundColumn = true
		completed, valid := value.root.Store().Read(declaration.ID(), key, within, value.scratch, func(part store.ReadPart) bool {
			if !value.validPart(part, declaration.ID(), key, within) {
				return false
			}
			if !part.Presence().Is(model.Present) && !part.Presence().Is(model.AuthenticatedOpaque) {
				return true
			}
			if !membership.Valid() {
				membership = part.Region()
				return true
			}
			var ok bool
			membership, ok = support.Union(membership, part.Region())
			return ok
		})
		if !completed || !valid {
			return nil, false
		}
	}
	if !foundColumn || !membership.Valid() || support.Empty(membership) {
		return nil, true
	}
	scope, scopeOK := value.view.Normalize(membership)
	if !scopeOK || !scope.ValidFor(value.fence) {
		return nil, false
	}
	lineage, lineageOK := value.directoryLineage(id)
	if !lineageOK {
		return nil, false
	}
	candidate := &row{owner: value, id: id, key: key, mask: membership, scope: scope, lineage: lineage}
	if !candidate.Available() {
		return nil, false
	}
	return []*row{candidate}, true
}

func joinPartsLineage(value *reader, parts []store.ReadPart) (model.LineageRef, bool) {
	if value == nil || value.lineageAuthority == nil || len(parts) == 0 {
		return model.LineageRef{}, false
	}
	result := parts[0].Lineage()
	if !value.lineageAuthority.Validate(result) {
		return model.LineageRef{}, false
	}
	for _, part := range parts[1:] {
		if !value.lineageAuthority.Validate(part.Lineage()) {
			return model.LineageRef{}, false
		}
		joined, ok := value.lineageAuthority.Join(result, part.Lineage())
		if !ok {
			return model.LineageRef{}, false
		}
		result = joined
	}
	return result, result.Available()
}

func changedRowAt(mounted witness.Mounted, relation model.RelationID, key geometry.Key) (model.RowID, bool) {
	if !mounted.Available() || !relation.Available() {
		return model.RowID{}, false
	}
	maxInt := uint64(^uint(0) >> 1)
	if uint64(key) > maxInt {
		return model.RowID{}, false
	}
	row, ok := mounted.RowAt(relation, int(key))
	if !ok || !row.Available() || row.Relation() != relation {
		return model.RowID{}, false
	}
	// The inverse check closes the stale-coordinate hole without ever deriving
	// a RowID from the geometry key.
	back, backOK := mounted.RowIndex(relation, row)
	return row, backOK && back == int(key)
}

func sortRows(rows []*row) {
	sort.SliceStable(rows, func(left, right int) bool {
		if rows[left].key != rows[right].key {
			return rows[left].key < rows[right].key
		}
		leftID, leftOK := rows[left].mask.Identity()
		rightID, rightOK := rows[right].mask.Identity()
		if !leftOK || !rightOK {
			return false
		}
		return bytes.Compare(leftID[:], rightID[:]) < 0
	})
}
