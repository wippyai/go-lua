// Package relation owns the neutral bind-time surface of generated axis
// relations. Declaration vocabulary stays in the parent member package;
// construction imports this child only when it must reduce sealed owner rows
// to dense coordinates.
package relation

import "github.com/wippyai/go-lua/analysis/identity"

// SourceColumn is an immutable, typed dense column published by a generated
// relation owner at schema materialization time. Its backing storage is
// private; execution can only index an owner-issued ordinal through At.
type SourceColumn[V any] struct {
	values []V
	sealed bool
}

// NewSourceColumn seals one owner-materialized value slice by taking an
// independent copy. The returned column has no mutation operation.
func NewSourceColumn[V any](values []V) SourceColumn[V] {
	return SourceColumn[V]{values: append([]V(nil), values...), sealed: true}
}

// Valid distinguishes a deliberately sealed empty column from a missing
// materialization.  That distinction is load-bearing: an empty source family
// is a valid closed-world fact, while an omitted relation is not a source
// column at all.
func (column SourceColumn[V]) Valid() bool { return column.sealed }

// Count is the exact sealed dense width.
func (column SourceColumn[V]) Count() int {
	if !column.sealed {
		return 0
	}
	return len(column.values)
}

// At indexes the column directly by the owner-issued dense candidate ordinal.
func (column SourceColumn[V]) At(index uint32) (V, bool) {
	if !column.sealed || uint64(index) >= uint64(len(column.values)) {
		var zero V
		return zero, false
	}
	return column.values[index], true
}

// Clone returns an independent sealed column for a Program-bound runtime
// factor.  It makes the ownership break explicit even though the source
// column is immutable: runtime data has no backing slice retained by the
// cold relation owner.
func (column SourceColumn[V]) Clone() SourceColumn[V] {
	if !column.sealed {
		return SourceColumn[V]{}
	}
	return NewSourceColumn(column.values)
}

// SourceColumns is a bind-only typed view implemented by generated relation
// owners that materialize zero-input facts.  The engine copies these sealed
// values into the bound Factor once; it never retains this provider or calls
// back into an owner while solving.
//
// RelationCount is the owner-issued relation ordinal extent, not the number
// of materialized columns.  A sparse materialization (for example relation 2
// only) therefore remains unambiguous.
type SourceColumns[V any] interface {
	RelationCount() int
	SourceFactColumn(relationOrdinal uint32) (SourceColumn[V], bool)
}

// Owner resolves an occurrence into its owner-issued dense candidate and
// projects that candidate into an axis-local coordinate. Domain values never
// cross this boundary, and implementations are not retained after Program
// construction.
type Owner interface {
	Candidate(relationOrdinal uint32, mount, occurrence identity.ContentID) (uint32, bool)
	Project(relationOrdinal, projectionOrdinal, candidateOrdinal uint32) (uint32, bool)
}
