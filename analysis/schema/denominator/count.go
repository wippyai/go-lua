package denominator

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/schema"
)

// CountRow is one immutable cardinality fact for a sealed denominator entry.
// The row carries only the neutral entry identity and its count; ownership and
// derivation remain with the component that produced it.
type CountRow struct {
	id    schema.EntryID
	count uint64
}

// NewCountRow admits one count for an available denominator entry.
func NewCountRow(id schema.EntryID, count uint64) (CountRow, bool) {
	if !id.Available() {
		return CountRow{}, false
	}
	return CountRow{id: id, count: count}, true
}

// ID returns the denominator entry identity carried by the row.
func (row CountRow) ID() schema.EntryID { return row.id }

// Count returns the admitted cardinality.
func (row CountRow) Count() uint64 { return row.count }

// CountRows is an immutable, canonical set of count rows. Its constructor
// copies and sorts the input, rejects unavailable identities and duplicates,
// and never retains caller-owned storage.
type CountRows struct{ rows []CountRow }

// NewCountRows seals one owner-local count set.
func NewCountRows(rows []CountRow) (CountRows, bool) {
	if rows == nil {
		return CountRows{}, false
	}
	copyRows := make([]CountRow, len(rows))
	copy(copyRows, rows)
	sort.Slice(copyRows, func(left, right int) bool {
		return bytesLess(copyRows[left].id, copyRows[right].id)
	})
	for index, row := range copyRows {
		if !row.id.Available() || index > 0 && copyRows[index-1].id == row.id {
			return CountRows{}, false
		}
	}
	return CountRows{rows: copyRows}, true
}

// Available reports whether the count set was sealed. An empty set is a
// valid sealed set, which lets owners with no declared rows remain neutral.
func (rows CountRows) Available() bool { return rows.rows != nil }

// Count reports the number of immutable count rows.
func (rows CountRows) Count() int {
	if !rows.Available() {
		return 0
	}
	return len(rows.rows)
}

// At returns one canonical identity-ordered row.
func (rows CountRows) At(index int) (CountRow, bool) {
	if !rows.Available() || index < 0 || index >= len(rows.rows) {
		return CountRow{}, false
	}
	return rows.rows[index], true
}

// Value resolves one entry identity without exposing the backing slice.
func (rows CountRows) Value(id schema.EntryID) (uint64, bool) {
	if !rows.Available() || !id.Available() {
		return 0, false
	}
	index := sort.Search(len(rows.rows), func(index int) bool {
		return !bytesLess(rows.rows[index].id, id)
	})
	if index == len(rows.rows) || rows.rows[index].id != id {
		return 0, false
	}
	return rows.rows[index].count, true
}

// MergeCountRows combines disjoint owner-local sets into one immutable set.
// Duplicate identities are rejected instead of silently choosing an owner.
func MergeCountRows(parts ...CountRows) (CountRows, bool) {
	var total int
	for _, part := range parts {
		if !part.Available() {
			return CountRows{}, false
		}
		if len(part.rows) > int(^uint(0)>>1)-total {
			return CountRows{}, false
		}
		total += len(part.rows)
	}
	merged := make([]CountRow, 0, total)
	for _, part := range parts {
		merged = append(merged, part.rows...)
	}
	return NewCountRows(merged)
}

// SumCountRows combines owner-local cardinalities into one canonical set.
// Unlike MergeCountRows, repeated identities are intentional: one reusable
// Program can be mounted more than once, so the same declared relation may be
// contributed by several mounts. Counts are added with checked uint64
// arithmetic; an overflow is rejected instead of wrapping into a smaller
// published cardinality.
func SumCountRows(parts ...CountRows) (CountRows, bool) {
	counts := make(map[schema.EntryID]uint64)
	for _, part := range parts {
		if !part.Available() {
			return CountRows{}, false
		}
		for index := 0; index < part.Count(); index++ {
			row, ok := part.At(index)
			if !ok {
				return CountRows{}, false
			}
			prior := counts[row.ID()]
			if ^uint64(0)-prior < row.Count() {
				return CountRows{}, false
			}
			counts[row.ID()] = prior + row.Count()
		}
	}
	rows := make([]CountRow, 0, len(counts))
	for id, count := range counts {
		row, ok := NewCountRow(id, count)
		if !ok {
			return CountRows{}, false
		}
		rows = append(rows, row)
	}
	return NewCountRows(rows)
}

// GeneratedCountRowsComplete reports whether rows cover exactly the generated
// relation catalog. A complete set contains one row for every declaration,
// including rows whose count is zero, and no identity outside that catalog.
// The helper is intentionally tied to the generated catalog: it is the one
// denominator coverage law shared by cold owners and Snapshot publication.
func GeneratedCountRowsComplete(rows CountRows) bool {
	return GeneratedCountRowsCompleteForOwners(rows,
		RelationOwnerProgramSource,
		RelationOwnerProgramFlow,
		RelationOwnerProgramStatic,
		RelationOwnerProgramModule,
		RelationOwnerTarget,
		RelationOwnerLinkProject,
		RelationOwnerLinkBoundary,
		RelationOwnerLinkModule,
		RelationOwnerLinkStatic,
		RelationOwnerLinkHost,
	)
}

// GeneratedCountRowsCompleteForOwners reports exact coverage for the selected
// generated owners. It lets a cold owner validate only its own rows while the
// Link/Snapshot boundary validates the complete catalog, without restating
// relation keys in each component.
func GeneratedCountRowsCompleteForOwners(rows CountRows, owners ...RelationOwner) bool {
	if !rows.Available() || len(owners) == 0 {
		return false
	}
	selected := make(map[RelationOwner]struct{}, len(owners))
	for _, owner := range owners {
		if !owner.Available() {
			return false
		}
		selected[owner] = struct{}{}
	}
	expected := make(map[schema.EntryID]struct{})
	for owner := range selected {
		ids := generatedOwnerIDs(owner)
		if len(ids) == 0 {
			return false
		}
		for _, id := range ids {
			if !id.Available() {
				return false
			}
			if _, duplicate := expected[id]; duplicate {
				return false
			}
			expected[id] = struct{}{}
		}
	}
	if len(expected) == 0 || rows.Count() != len(expected) {
		return false
	}
	seen := make(map[schema.EntryID]struct{}, rows.Count())
	for index := 0; index < rows.Count(); index++ {
		row, ok := rows.At(index)
		if !ok {
			return false
		}
		if _, duplicate := seen[row.ID()]; duplicate {
			return false
		}
		if _, declared := expected[row.ID()]; !declared {
			return false
		}
		seen[row.ID()] = struct{}{}
	}
	return len(seen) == len(expected)
}

// SumInts adds owner-local cardinalities with overflow and sign checks.
func SumInts(values ...int) (int, bool) {
	total := 0
	for _, value := range values {
		if value < 0 || value > int(^uint(0)>>1)-total {
			return 0, false
		}
		total += value
	}
	return total, true
}

func bytesLess(left, right schema.EntryID) bool {
	for index := range left {
		if left[index] == right[index] {
			continue
		}
		return left[index] < right[index]
	}
	return false
}
