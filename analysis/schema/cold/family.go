package cold

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

// Ordinal is the position of one row inside its cold family. A cold family is
// a dense sequence the compiler emitted in one order, and that order is part
// of what the artifact's content identity commits to, so the coordinate is
// the position and not a derived identity.
type Ordinal uint32

// Row is what every cold family's element answers: whether it names a proof.
// A row that is missing any identity it needs proves nothing, so a family
// never seals one and a consumer never reads one.
type Row interface {
	Available() bool
}

// Family is one cold column's whole declaration: the slot it occupies, the
// derivation its key universe is named by, and every operation over it. The
// families differ only in their row type, their slot and their name, so they
// are one declaration parameterised rather than one copy per family -- a new
// family is a line, and none of them can drift from the others in how it
// seals, sizes or reads.
type Family[V Row] struct {
	slot uint32
	name string
}

// Axis is the address of this family's column in a cold catalog.
func (family Family[V]) Axis(catalog identity.ContentID) snapshot.Axis[Ordinal, V] {
	return snapshot.Axis[Ordinal, V]{SchemaID: catalog, Slot: family.slot}
}

// Denominator is the identity of this family's key universe within one cold
// catalog. The universe is the family's own ordinal range, so its identity is
// derived from the catalog and the family name alone.
func (family Family[V]) Denominator(catalog identity.ContentID) (identity.ContentID, bool) {
	if !catalog.Available() {
		return identity.ContentID{}, false
	}
	return identity.DeriveContentID(catalogDomain+"/"+family.name, catalog[:])
}

// Content seals an emitted sequence into the column's payload. The
// denominator's membership is that sequence's ordinal range, so the column is
// total over exactly what it publishes and an ordinal past the end is a
// proven absence rather than a missing row.
//
// A sequence containing an unavailable row seals nothing: a compiled program
// either proved every row it emitted or it did not compile.
func (family Family[V]) Content(rows []V, catalog identity.ContentID) (snapshot.Content[Ordinal, V], bool) {
	denominator, derived := family.Denominator(catalog)
	if !derived {
		return snapshot.Content[Ordinal, V]{}, false
	}
	sealed := make(map[Ordinal]V, len(rows))
	members := make([]Ordinal, 0, len(rows))
	for index, row := range rows {
		if !row.Available() {
			return snapshot.Content[Ordinal, V]{}, false
		}
		ordinal := Ordinal(index)
		sealed[ordinal] = row
		members = append(members, ordinal)
	}
	return snapshot.Content[Ordinal, V]{Rows: sealed, Denominator: denominator, Members: members}, true
}

// Put seals this family into a publication under construction.
func (family Family[V]) Put(builder *snapshot.FrozenBuilder, rows []V, catalog identity.ContentID) bool {
	content, sealed := family.Content(rows, catalog)
	if !sealed {
		return false
	}
	return snapshot.PutFrozenColumn(builder, family.Axis(catalog), content) == nil
}

// Count is the sealed width of this family: the cardinality of the key
// universe the column is total over. A catalog the publication does not hold
// reports nothing rather than an empty family.
func (family Family[V]) Count(frozen *snapshot.Frozen, catalog identity.ContentID) (int, bool) {
	denominator, derived := family.Denominator(catalog)
	if !derived || frozen == nil {
		return 0, false
	}
	return frozen.Denominators().Size(denominator)
}

// At returns one row by its position in the emitted sequence. An ordinal
// outside the sealed family, and a publication that holds no such column at
// all, both report nothing.
func (family Family[V]) At(frozen *snapshot.Frozen, catalog identity.ContentID, index int) (V, bool) {
	var absent V
	if index < 0 {
		return absent, false
	}
	row, status := snapshot.ReadFrozen(frozen, family.Axis(catalog), Ordinal(index))
	if status != snapshot.ReadHit {
		return absent, false
	}
	return row, true
}

// Span returns the rows a parent row names by offset and count. A span that
// runs past the sealed family is not a short read: the parent named rows the
// publication does not hold, so it reports nothing at all.
func (family Family[V]) Span(frozen *snapshot.Frozen, catalog identity.ContentID, offset, count uint32) ([]V, bool) {
	width, published := family.Count(frozen, catalog)
	if !published || uint64(offset)+uint64(count) > uint64(width) {
		return nil, false
	}
	if count == 0 {
		return nil, true
	}
	rows := make([]V, 0, count)
	for index := uint32(0); index < count; index++ {
		row, held := family.At(frozen, catalog, int(offset+index))
		if !held {
			return nil, false
		}
		rows = append(rows, row)
	}
	return rows, true
}

// The dense slot each cold family occupies in a compiled program's
// publication. Slots are append-only: a family added later takes the next
// slot, and no family ever moves, because a slot is half of the address every
// consumer holds.
const (
	slotCallTarget uint32 = iota
	slotHeapAllocation
	slotHeapField
	slotValues
	slotValuesMember
	slotHeapIndex
)

// The declarations below are the complete cold family catalog. Each accessor
// returns one typed Family value, not a second per-family implementation or a
// registry; callers use its Axis, Content, Count, At, and Span methods
// directly. The values remain private-field declarations, so callers cannot
// replace a family or mutate the catalog's slot and name.
//
// Slots are append-only and names are part of the denominator derivation, so
// both are kept here beside the row type they address. A newly published
// family gets one declaration and cannot accidentally drift in sealing or
// reading semantics from the other families.
func CallTargetFamily() Family[CallTarget] {
	return Family[CallTarget]{slot: slotCallTarget, name: "call-target"}
}

func HeapAllocationFamily() Family[HeapAllocation] {
	return Family[HeapAllocation]{slot: slotHeapAllocation, name: "heap-allocation"}
}

func HeapFieldFamily() Family[HeapField] {
	return Family[HeapField]{slot: slotHeapField, name: "heap-field"}
}

func ValuesFamily() Family[Values] {
	return Family[Values]{slot: slotValues, name: "values"}
}

func ValuesMemberFamily() Family[ValuesMember] {
	return Family[ValuesMember]{slot: slotValuesMember, name: "values-member"}
}

func HeapIndexFamily() Family[HeapIndex] {
	return Family[HeapIndex]{slot: slotHeapIndex, name: "heap-index"}
}
