// Package references owns the authored TypeRef relation: its source spelling,
// its optional canonical path, and the binder disposition that separates the
// two.
//
// The package is independent of the enclosing Static component. It validates
// and seals its own rows, exposes immutable queries, and hands the resulting
// table back to Static as a value. Resolution to a declaration target is
// authored by the binder, never inferred here.
package references

import (
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
	"github.com/wippyai/go-lua/internal/rows"
)

// Resolution preserves the authored binder result independently from its
// source spelling. It is not an inferred resolution result.
type Resolution uint8

const (
	Unresolved Resolution = iota + 1
	Declaration
	CanonicalPath
)

// TypeRef retains the complete authored spelling and its binder disposition.
// A declaration target and a canonical path are mutually exclusive.
type TypeRef struct {
	Resolution Resolution
	Target     keyspace.Term
	Root       keyspace.Term
	Source     []keyspace.Key
	Canonical  []keyspace.Key
}

// Input is the complete authored TypeRef denominator. Source and canonical
// paths retain key handles only; Source/keyspace membership is a later joint
// seal obligation.
type Input struct{ TypeRef []TypeRef }

// TypeRefRow is the sealed form of a TypeRef: both key paths live in shared
// columns and the row keeps only their windows.
type TypeRefRow struct {
	Resolution Resolution
	Target     keyspace.Term
	Root       keyspace.Term
	Source     rows.Span
	Canonical  rows.Span
}

// Resolved reports whether the disposition names something beyond the local
// spelling. It is the exact admission a publication target requires, stated
// once here rather than restated as a family test by each consumer.
func (row TypeRefRow) Resolved() bool {
	return row.Resolution == Declaration || row.Resolution == CanonicalPath
}

// Table is the sealed immutable TypeRef relation.
type Table struct {
	ref  rows.Table[TypeRefRow]
	keys rows.Pool[keyspace.Key]
}

// Count is the sealed TypeRef denominator.
func (table Table) Count() int { return table.ref.Count() }

// CountsMatch reports the native TypeRef denominator against the enclosing
// sealed family column.
func (table Table) CountsMatch(counts [keyspace.FamilyCount]uint32) bool {
	return table.Count() == int(counts[keyspace.FamilyTypeRef])
}

// CountRows publishes this typed owner's native TypeRef contribution. The
// same sealed count feeds the aggregate Static row and the dedicated schema
// TypeRef row.
func (table Table) CountRows() (denominator.CountRows, bool) {
	value := table.Count()
	if !keyspace.TermOrdinalFits(value) {
		return denominator.CountRows{}, false
	}
	ids := denominator.GeneratedProgramStaticIDs()
	primary, ok := denominator.NewCountRow(ids.ProgramStatic, uint64(value))
	if !ok {
		return denominator.CountRows{}, false
	}
	typeRef, ok := denominator.NewCountRow(ids.ProgramStaticTypeRef, uint64(value))
	if !ok {
		return denominator.CountRows{}, false
	}
	return denominator.NewCountRows([]denominator.CountRow{primary, typeRef})
}

// Ref returns the authored reference one canonical term names. It is the read
// a sibling vertical uses to admit a resolved target without reaching into
// this owner's storage or re-deriving its disposition.
func (table Table) Ref(term keyspace.Term) (TypeRefRow, bool) { return table.ref.Row(term) }
