// Package index owns Source's immutable sparse sealed position table.
package index

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// Row is one validated Source position row supplied by the Source owner.
// Construction is intentionally opaque so the table remains the only owner
// of retained row storage.
type Row struct {
	term           keyspace.Term
	root           keyspace.Term
	body           keyspace.Term
	offset         uint32
	cursor         uint32
	frontierBody   keyspace.Term
	frontierCursor uint32
}

// NewRow creates one immutable table input row. Source performs the
// authority-specific direct-root and frontier checks before issuing rows to
// Seal; this package owns only canonical table shape and retention.
func NewRow(term, root, body keyspace.Term, offset, cursor uint32, frontierBody keyspace.Term, frontierCursor uint32) Row {
	return Row{
		term: term, root: root, body: body, offset: offset, cursor: cursor,
		frontierBody: frontierBody, frontierCursor: frontierCursor,
	}
}

// Slot is the immutable position projection returned by Table lookup.
type Slot struct {
	root           keyspace.Term
	body           keyspace.Term
	offset         uint32
	cursor         uint32
	frontierBody   keyspace.Term
	frontierCursor uint32
}

func (slot Slot) Root() keyspace.Term         { return slot.root }
func (slot Slot) Body() keyspace.Term         { return slot.body }
func (slot Slot) Offset() uint32              { return slot.offset }
func (slot Slot) Cursor() uint32              { return slot.cursor }
func (slot Slot) FrontierBody() keyspace.Term { return slot.frontierBody }
func (slot Slot) FrontierCursor() uint32      { return slot.frontierCursor }

type entry struct {
	ordinal uint32
	slot    Slot
}

type familyRows []entry

func (rows familyRows) lookup(ordinal uint32) (Slot, bool) {
	low, high := 0, len(rows)
	for low < high {
		middle := low + (high-low)/2
		if rows[middle].ordinal < ordinal {
			low = middle + 1
			continue
		}
		high = middle
	}
	if low == len(rows) || rows[low].ordinal != ordinal {
		return Slot{}, false
	}
	return rows[low].slot, true
}

// Table is the one immutable sparse position table issued by Source Commit.
// It retains only positioned Terms, with exact-capacity family slices over one
// backing allocation; the authored Source authority remains the identity and
// ownership fence around this projection.
type Table struct {
	rows   [keyspace.FamilyCount]familyRows
	sealed bool
}

// Seal copies canonical rows into one immutable sparse table. The Source root
// validates ownership, direct roots, and frontier geometry before this call;
// this child closes the table's own family/ordinal ordering and duplicate
// invariants without retaining the caller's batch.
func Seal(rows []Row) (*Table, error) {
	entries := make([]entry, len(rows))
	var previousFamily keyspace.Family
	var previousOrdinal uint32
	for position, row := range rows {
		family, ordinal := keyspace.TermFamily(row.term), keyspace.TermOrdinal(row.term)
		if family <= keyspace.FamilyInvalid || family >= keyspace.FamilyCount ||
			family == keyspace.FamilyOutcome || ordinal == 0 {
			return nil, errors.New("program/source/index: invalid position term")
		}
		if previousFamily != keyspace.FamilyInvalid &&
			(family < previousFamily || family == previousFamily && ordinal <= previousOrdinal) {
			return nil, errors.New("program/source/index: noncanonical position order")
		}
		previousFamily, previousOrdinal = family, ordinal
		entries[position] = entry{
			ordinal: ordinal,
			slot: Slot{
				root: row.root, body: row.body, offset: row.offset, cursor: row.cursor,
				frontierBody: row.frontierBody, frontierCursor: row.frontierCursor,
			},
		}
	}
	table := &Table{sealed: true}
	if len(entries) == 0 {
		return table, nil
	}
	// Carve exact-capacity views over the one retained backing allocation.
	start := 0
	family := keyspace.TermFamily(rows[0].term)
	for position := 1; position <= len(entries); position++ {
		if position == len(entries) || keyspace.TermFamily(rows[position].term) != family {
			table.rows[family] = familyRows(entries[start:position:position])
			if position < len(entries) {
				start = position
				family = keyspace.TermFamily(rows[position].term)
			}
		}
	}
	return table, nil
}

// Lookup returns one exact sealed position slot without allocation.
func (table *Table) Lookup(term keyspace.Term) (Slot, bool) {
	if table == nil || !table.sealed {
		return Slot{}, false
	}
	family, ordinal := keyspace.TermFamily(term), keyspace.TermOrdinal(term)
	if family <= keyspace.FamilyInvalid || family >= keyspace.FamilyCount || ordinal == 0 || family == keyspace.FamilyOutcome {
		return Slot{}, false
	}
	return table.rows[family].lookup(ordinal)
}

// Count returns the number of retained rows for one Term family.
func (table *Table) Count(family keyspace.Family) int {
	if table == nil || !table.sealed || family <= keyspace.FamilyInvalid || family >= keyspace.FamilyCount {
		return 0
	}
	return len(table.rows[family])
}
