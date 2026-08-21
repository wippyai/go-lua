// Package index owns Source's immutable sparse sealed position table.
package index

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

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

// builderState is shared by copied Builder values. The state fence makes the
// ownership transfer in Seal one-shot even if a caller copies the capability;
// no post-seal Add can mutate the backing allocation retained by Table.
type builderState struct {
	entries         []entry
	starts          [keyspace.FamilyCount]int
	counts          [keyspace.FamilyCount]int
	previousFamily  keyspace.Family
	previousOrdinal uint32
	sealed          bool
}

// Builder is the owner-issued construction capability for one sparse Source
// position table. It retains private entries while Source-specific validation
// runs, then transfers that backing allocation directly to Table at Seal.
// There is no caller-owned row DTO for Table to copy.
type Builder struct {
	state *builderState
}

// NewBuilder creates an empty position-table builder with capacity for the
// expected retained rows. Capacity is only an allocation hint; sparse family
// ordinals remain the sole table denominator.
func NewBuilder(capacity int) *Builder {
	if capacity < 0 {
		capacity = 0
	}
	return &Builder{state: &builderState{entries: make([]entry, 0, capacity)}}
}

// Add appends one canonical position row. Source owns semantic validation of
// roots, Bodies, and Repeat frontier geometry; this package owns only the
// table's family/ordinal order and immutable scalar retention.
func (builder *Builder) Add(term, root, body keyspace.Term, offset, cursor uint32, frontierBody keyspace.Term, frontierCursor uint32) error {
	if builder == nil || builder.state == nil {
		return errors.New("program/source/index: invalid position builder")
	}
	state := builder.state
	if state.sealed {
		return errors.New("program/source/index: position builder is sealed")
	}
	family, ordinal := keyspace.TermFamily(term), keyspace.TermOrdinal(term)
	if family <= keyspace.FamilyInvalid || family >= keyspace.FamilyCount ||
		family == keyspace.FamilyOutcome || ordinal == 0 {
		return errors.New("program/source/index: invalid position term")
	}
	if state.previousFamily != keyspace.FamilyInvalid &&
		(family < state.previousFamily || family == state.previousFamily && ordinal <= state.previousOrdinal) {
		return errors.New("program/source/index: noncanonical position order")
	}
	if state.counts[family] == 0 {
		state.starts[family] = len(state.entries)
	}
	state.counts[family]++
	state.previousFamily, state.previousOrdinal = family, ordinal
	state.entries = append(state.entries, entry{
		ordinal: ordinal,
		slot: Slot{
			root: root, body: body, offset: offset, cursor: cursor,
			frontierBody: frontierBody, frontierCursor: frontierCursor,
		},
	})
	return nil
}

// Table is the one immutable sparse position table issued by Source Commit.
// It retains only positioned Terms, with exact-capacity family slices over one
// backing allocation; the authored Source authority remains the identity and
// ownership fence around this projection.
type Table struct {
	rows   [keyspace.FamilyCount]familyRows
	sealed bool
}

// Seal transfers the builder's private backing allocation into one immutable
// sparse table. The builder is terminal and releases its slice header, so the
// published Table is the sole owner of the retained entries.
func (builder *Builder) Seal() (*Table, error) {
	if builder == nil || builder.state == nil {
		return nil, errors.New("program/source/index: invalid position builder")
	}
	state := builder.state
	if state.sealed {
		return nil, errors.New("program/source/index: position builder is sealed")
	}
	state.sealed = true
	entries := state.entries
	state.entries = nil
	table := &Table{sealed: true}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		count := state.counts[family]
		if count == 0 {
			continue
		}
		start := state.starts[family]
		end := start + count
		if start < 0 || end < start || end > len(entries) {
			return nil, errors.New("program/source/index: invalid position builder ranges")
		}
		table.rows[family] = familyRows(entries[start:end:end])
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
