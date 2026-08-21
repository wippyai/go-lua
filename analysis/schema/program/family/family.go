// Package family owns the neutral operations over one cold Program family.
// It knows only the row availability contract, the opaque catalog definition,
// and the snapshot substrate; semantic Program row types stay in the parent
// package.
package family

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/program/catalog"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

// Ordinal is the position of one row inside its cold family. A cold family is
// a dense sequence emitted in one order, and that order is part of the
// artifact content identity.
type Ordinal uint32

// Row is what every cold family's element answers: whether it names a proof.
// A row missing an identity it needs proves nothing, so a family never seals
// one and a consumer never reads one.
type Row interface {
	Available() bool
}

// Family is one cold column's whole declaration. Its slot and denominator
// name come from one opaque catalog definition; row operations are shared by
// every typed family.
type Family[V Row] struct {
	definition catalog.Definition
}

// New binds a typed family to one canonical catalog definition. The
// definition remains opaque to callers; only the catalog package can author
// its slot/name pair.
func New[V Row](definition catalog.Definition) Family[V] {
	return Family[V]{definition: definition}
}

// Definition returns the opaque declaration bound to this family. Its fields
// remain private to catalog, so callers can compare or pass the declaration
// without manufacturing one.
func (family Family[V]) Definition() catalog.Definition { return family.definition }

// Axis is the address of this family's column in a cold catalog.
func (family Family[V]) Axis(catalogID identity.ContentID) snapshot.Axis[Ordinal, V] {
	return snapshot.Axis[Ordinal, V]{SchemaID: catalogID, Slot: family.definition.Slot()}
}

// Denominator is the identity of this family's key universe within one cold
// catalog.
func (family Family[V]) Denominator(catalogID identity.ContentID) (identity.ContentID, bool) {
	return family.definition.Denominator(catalogID)
}

// Content seals an emitted sequence into the column's payload. The
// denominator's membership is that sequence's ordinal range, so the column
// is total over exactly what it publishes.
func (family Family[V]) Content(rows []V, catalogID identity.ContentID) (snapshot.Content[Ordinal, V], bool) {
	denominator, derived := family.Denominator(catalogID)
	if !derived {
		return snapshot.Content[Ordinal, V]{}, false
	}
	for _, row := range rows {
		if !row.Available() {
			return snapshot.Content[Ordinal, V]{}, false
		}
	}
	if rows == nil {
		rows = []V{}
	}
	return snapshot.Content[Ordinal, V]{Sequence: rows, Denominator: denominator}, true
}

// Put seals this family into a publication under construction.
func (family Family[V]) Put(builder *snapshot.FrozenBuilder, rows []V, catalogID identity.ContentID) bool {
	content, sealed := family.Content(rows, catalogID)
	if !sealed {
		return false
	}
	return snapshot.PutFrozenColumn(builder, family.Axis(catalogID), content) == nil
}

// Count is the sealed width of this family. A catalog the publication does
// not hold reports nothing rather than an empty family.
func (family Family[V]) Count(frozen *snapshot.Frozen, catalogID identity.ContentID) (int, bool) {
	denominator, derived := family.Denominator(catalogID)
	if !derived || frozen == nil {
		return 0, false
	}
	return frozen.Denominators().Size(denominator)
}

// At returns one row by its position in the emitted sequence.
func (family Family[V]) At(frozen *snapshot.Frozen, catalogID identity.ContentID, index int) (V, bool) {
	var absent V
	if index < 0 {
		return absent, false
	}
	row, status := snapshot.ReadFrozen(frozen, family.Axis(catalogID), Ordinal(index))
	if status != snapshot.ReadHit {
		return absent, false
	}
	return row, true
}

// Span returns the rows a parent row names by offset and count. The returned
// rows borrow the sealed plane and a span past the family is not a short read.
func (family Family[V]) Span(frozen *snapshot.Frozen, catalogID identity.ContentID, offset, count uint32) ([]V, bool) {
	return snapshot.ReadSpan(frozen, family.Axis(catalogID), offset, count)
}
