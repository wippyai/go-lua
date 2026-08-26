package read

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/index"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/store"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// commonFiber is one exact common refinement of all delivered-column
// partitions for one physical key.  The masks stay private to read; only the
// normalized Scope crosses the Row boundary.
type commonFiber struct {
	region support.Mask
	parts  []store.ReadPart
}

// rowsFor expands one index posting into its common cofiber rows.  An index
// region is authoritative for the key, but delivered columns may be
// partitioned differently. Every returned row is one nonempty intersection
// of one partition from each requested column; the complete refinement must
// remain a disjoint exact cover of the original posting.
func (value *reader) rowsFor(match index.Match) ([]*row, bool) {
	if !value.available() || !match.Region().Valid() || match.Region().Manager() != value.manager || support.Empty(match.Region()) {
		return nil, false
	}
	relation := value.layout.Access().Relation()
	if match.Relation() != relation {
		return nil, false
	}
	key := match.Key()
	maxInt := uint64(^uint(0) >> 1)
	if uint64(key) > maxInt {
		return nil, false
	}
	// RowID authority is the mounted relation directory, not an index posting
	// or a geometry ordinal. The second lookup proves the directory inverse.
	id, ok := value.mounted.RowAt(relation, int(key))
	if !ok || !id.Available() || id.Relation() != relation {
		return nil, false
	}
	back, backOK := value.mounted.RowIndex(relation, id)
	if !backOK || back != int(key) {
		return nil, false
	}
	if indexed := match.Row(); !indexed.Available() || indexed != id {
		return nil, false
	}
	// A relation Input is the owner-directory scan: both KeyColumns and
	// Columns are intentionally empty.  Its logical rows come from the
	// mounted row directory, not from a fabricated geometry key or a
	// semantic payload.  The directory's RowID is also the only issued source
	// identity available for lineage at this altitude; preserve its exact
	// owner/content pair as a foreign lineage atom rather than hashing a new
	// identity or manufacturing a fallback.
	//
	// A keyed arrangement can also be a pure lookup access: its sealed
	// KeyColumns are the tuple authority and Layout.Columns is intentionally
	// empty.  It has one row at the index fiber and no payload refinement; its
	// lineage is read from the owner-issued key-column sidecars below.
	if len(value.layout.Columns()) == 0 {
		scope, scopeOK := value.view.Normalize(match.Region())
		if !scopeOK || !scope.ValidFor(value.fence) {
			return nil, false
		}
		var rowLineage model.LineageRef
		var lineageOK bool
		if len(value.layout.KeyColumns()) == 0 {
			rowLineage, lineageOK = value.directoryLineage(id)
		} else {
			rowLineage, lineageOK = value.keyLineage(key, match.Region())
		}
		if !lineageOK {
			return nil, false
		}
		result := &row{owner: value, id: id, key: key, mask: match.Region(), scope: scope, lineage: rowLineage}
		if !result.Available() {
			return nil, false
		}
		return []*row{result}, true
	}

	fibers, fibersOK := value.commonFibers(key, match.Region())
	if !fibersOK || len(fibers) == 0 {
		return nil, fibersOK
	}
	rows := make([]*row, 0, len(fibers))
	for _, fiber := range fibers {
		if len(fiber.parts) != len(value.layout.Columns()) {
			return nil, false
		}
		scope, scopeOK := value.view.Normalize(fiber.region)
		if !scopeOK || !scope.ValidFor(value.fence) {
			return nil, false
		}
		cells := make([]Cell, len(fiber.parts))
		for position, part := range fiber.parts {
			cells[position] = Cell{
				owner: value, column: part.Column(), typeID: part.Type(), value: part.Value(),
				presence: part.Presence(), scope: scope, lineage: part.Lineage(),
			}
		}
		rowLineage, lineageOK := value.joinLineage(key, fiber.region, cells)
		if !lineageOK {
			return nil, false
		}
		result := &row{owner: value, id: id, key: key, mask: fiber.region, scope: scope, lineage: rowLineage, cells: cells}
		if !result.Available() {
			return nil, false
		}
		rows = append(rows, result)
	}
	return rows, true
}

// commonFibers collects and validates each column's exact partition stream,
// then intersects those streams. A column may cover only part of an index
// posting (unkeyed indexes are unions); the common support is the intersection
// of each column's support union. Its emitted fibers must exactly cover that
// common support. No missing partition is filled with an Unknown/default cell.
func (value *reader) commonFibers(key geometry.Key, within support.Mask) ([]commonFiber, bool) {
	return value.commonFibersFor(key, within, value.layout.Columns())
}

// commonFibersFor is the one physical refinement used by both cold reads and
// differential reads.  Keeping the column vector as an argument is important
// for the latter: a changed key extent must first be restricted to the
// successor's complete row support before its delivered cells are issued.
// The masks never cross the Reader boundary; callers receive only the
// normalized Scope produced after this exact refinement.
func (value *reader) commonFibersFor(key geometry.Key, within support.Mask, columns []model.ColumnID) ([]commonFiber, bool) {
	if !value.available() || !within.Valid() || within.Manager() != value.manager {
		return nil, false
	}
	if len(columns) == 0 {
		return nil, false
	}
	seen := make(map[model.ColumnID]struct{}, len(columns))
	byColumn := make([][]store.ReadPart, len(columns))
	var common support.Mask
	for position, columnID := range columns {
		if _, duplicate := seen[columnID]; duplicate {
			return nil, false
		}
		seen[columnID] = struct{}{}
		parts := make([]store.ReadPart, 0, 2)
		failed := false
		completed, valid := value.root.Store().Read(columnID, key, within, value.scratch, func(part store.ReadPart) bool {
			if !value.validPart(part, columnID, key, within) {
				failed = true
				return false
			}
			parts = append(parts, part)
			return true
		})
		if failed || !completed || !valid || !value.validPartition(parts, within) {
			return nil, false
		}
		if len(parts) == 0 {
			return nil, true
		}
		columnSupport := parts[0].Region()
		for _, part := range parts[1:] {
			var unionOK bool
			columnSupport, unionOK = support.Union(columnSupport, part.Region())
			if !unionOK {
				return nil, false
			}
		}
		if position == 0 {
			common = columnSupport
		} else {
			var intersectOK bool
			common, intersectOK = support.Intersect(common, columnSupport)
			if !intersectOK {
				return nil, false
			}
		}
		byColumn[position] = parts
	}
	if !common.Valid() || support.Empty(common) {
		return nil, true
	}

	fibers := []commonFiber{{region: common}}
	for _, parts := range byColumn {
		next := make([]commonFiber, 0, len(fibers))
		for _, prior := range fibers {
			for _, part := range parts {
				overlap, ok := support.Intersect(prior.region, part.Region())
				if !ok {
					return nil, false
				}
				if support.Empty(overlap) {
					continue
				}
				selected := make([]store.ReadPart, len(prior.parts)+1)
				copy(selected, prior.parts)
				selected[len(prior.parts)] = part
				next = append(next, commonFiber{region: overlap, parts: selected})
			}
		}
		if len(next) == 0 {
			return nil, true
		}
		fibers = next
	}
	if !value.validFiberPartition(fibers, common) {
		return nil, false
	}
	return fibers, true
}

func (value *reader) validPart(part store.ReadPart, columnID model.ColumnID, key geometry.Key, within support.Mask) bool {
	if part.Key() != key || !part.Column().Available() || part.Column() != columnID || part.Column().Relation() != value.layout.Access().Relation() || !part.Type().Available() || !part.Region().Valid() || part.Region().Manager() != value.manager || support.Empty(part.Region()) || !part.Region().Entails(within) || !part.Presence().Available() || part.Presence().Is(model.Refused) || !part.Lineage().Available() || !value.lineageAuthority.Validate(part.Lineage()) {
		return false
	}
	if part.Value().Available() {
		return part.Value().ValidFor(value.fence) && part.Value().Type() == part.Type()
	}
	return !part.Presence().Is(model.Present) && !part.Presence().Is(model.AuthenticatedOpaque)
}

func (value *reader) validPartition(parts []store.ReadPart, within support.Mask) bool {
	if len(parts) == 0 {
		return true
	}
	for index := 0; index < len(parts); index++ {
		if !parts[index].Region().Entails(within) {
			return false
		}
		for prior := 0; prior < index; prior++ {
			overlap, ok := support.Intersect(parts[prior].Region(), parts[index].Region())
			if !ok || !support.Empty(overlap) {
				return false
			}
		}
	}
	return true
}

func (value *reader) exactPartition(parts []store.ReadPart, within support.Mask) bool {
	if !value.validPartition(parts, within) || len(parts) == 0 {
		return false
	}
	covered := parts[0].Region()
	for _, part := range parts[1:] {
		var ok bool
		covered, ok = support.Union(covered, part.Region())
		if !ok {
			return false
		}
	}
	return covered.Equal(within)
}

func (value *reader) validFiberPartition(fibers []commonFiber, within support.Mask) bool {
	if len(fibers) == 0 {
		return false
	}
	covered := fibers[0].region
	for index := range fibers {
		if !fibers[index].region.Valid() || fibers[index].region.Manager() != within.Manager() || support.Empty(fibers[index].region) || !fibers[index].region.Entails(within) {
			return false
		}
		// Check all prior fibers: Cartesian refinement must be pairwise
		// disjoint, regardless of partition traversal order.
		for prior := 0; prior < index; prior++ {
			overlap, overlapOK := support.Intersect(fibers[prior].region, fibers[index].region)
			if !overlapOK || !support.Empty(overlap) {
				return false
			}
		}
		if index > 0 {
			var unionOK bool
			covered, unionOK = support.Union(covered, fibers[index].region)
			if !unionOK {
				return false
			}
		}
	}
	return covered.Equal(within)
}

func (value *reader) joinLineage(key geometry.Key, within support.Mask, cells []Cell) (model.LineageRef, bool) {
	if value.lineageAuthority == nil {
		return model.LineageRef{}, false
	}
	// A key-only lookup relation has no projected payload cells, but its key
	// columns remain the owner-issued lineage authority. Read those columns
	// directly; do not mint a synthetic lineage for a physical index row.
	if len(cells) == 0 {
		return value.keyLineage(key, within)
	}
	result := cells[0].Lineage()
	if !value.lineageAuthority.Validate(result) {
		return model.LineageRef{}, false
	}
	for _, cell := range cells[1:] {
		if !value.lineageAuthority.Validate(cell.Lineage()) {
			return model.LineageRef{}, false
		}
		joined, ok := value.lineageAuthority.Join(result, cell.Lineage())
		if !ok {
			return model.LineageRef{}, false
		}
		result = joined
	}
	return result, result.Available()
}

func (value *reader) keyLineage(key geometry.Key, within support.Mask) (model.LineageRef, bool) {
	if value.lineageAuthority == nil || len(value.layout.KeyColumns()) == 0 {
		return model.LineageRef{}, false
	}
	var result model.LineageRef
	found := false
	for _, columnID := range value.layout.KeyColumns() {
		parts := make([]store.ReadPart, 0, 2)
		failed := false
		completed, valid := value.root.Store().Read(columnID, key, within, value.scratch, func(part store.ReadPart) bool {
			if !value.validPart(part, columnID, key, within) {
				failed = true
				return false
			}
			parts = append(parts, part)
			return true
		})
		if failed || !completed || !valid || !value.exactPartition(parts, within) {
			return model.LineageRef{}, false
		}
		for _, part := range parts {
			if !found {
				result, found = part.Lineage(), true
				continue
			}
			joined, ok := value.lineageAuthority.Join(result, part.Lineage())
			if !ok {
				return model.LineageRef{}, false
			}
			result = joined
		}
	}
	return result, found && result.Available()
}

// directoryLineage resolves the exact row lineage sealed by the mounted
// relation directory. Relation Input has no key or payload column from which
// a sidecar can be read, so the mounted projection is its sole source.
func (value *reader) directoryLineage(row model.RowID) (model.LineageRef, bool) {
	if !value.available() || !row.Available() || row.Relation() != value.layout.Access().Relation() {
		return model.LineageRef{}, false
	}
	return value.mounted.RowLineage(row)
}
