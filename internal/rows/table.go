package rows

import (
	"iter"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// Rows is a sealed dense row list with no canonical term identity. It is the
// representation for a relation whose denominator is its own row count, such
// as a sparse sidecar that carries its anchor inside the row.
type Rows[Row any] struct{ rows []Row }

// NewRows seals a copy of values.
func NewRows[Row any](values []Row) Rows[Row] {
	if len(values) == 0 {
		return Rows[Row]{}
	}
	return Rows[Row]{rows: append(make([]Row, 0, len(values)), values...)}
}

// Count is the sealed row denominator.
func (list Rows[Row]) Count() int { return len(list.rows) }

// At returns one row by dense index.
func (list Rows[Row]) At(index int) (row Row, ok bool) {
	if index < 0 || index >= len(list.rows) {
		return row, false
	}
	return list.rows[index], true
}

// All iterates every row in sealed order.
func (list Rows[Row]) All() iter.Seq2[int, Row] {
	return func(yield func(int, Row) bool) {
		for index, row := range list.rows {
			if !yield(index, row) {
				return
			}
		}
	}
}

// Table is a Rows list addressed by the canonical one-based ordinal of one
// keyspace family. The family travels with the storage, so a term read is
// self-checking and a foreign term cannot reach a row it does not name.
type Table[Row any] struct {
	Rows[Row]
	family keyspace.Family
}

// NewTable seals a copy of values under family. It fails closed on a family
// outside the closed keyspace inventory and on a row count that cannot be
// addressed by a canonical ordinal.
func NewTable[Row any](family keyspace.Family, values []Row) (Table[Row], bool) {
	if family <= keyspace.FamilyInvalid || family >= keyspace.FamilyCount || !keyspace.TermOrdinalFits(len(values)) {
		return Table[Row]{}, false
	}
	return Table[Row]{Rows: NewRows(values), family: family}, true
}

// Family is the canonical family this table numbers.
func (table Table[Row]) Family() keyspace.Family { return table.family }

// Term is the canonical term of one dense index.
func (table Table[Row]) Term(index int) (keyspace.Term, bool) {
	if index < 0 || index >= len(table.rows) {
		return 0, false
	}
	return keyspace.MakeTerm(table.family, uint32(index+1)), true
}

// Row returns the row one canonical term names. A term of another family, a
// zero ordinal, and an ordinal past the sealed count all fail closed.
func (table Table[Row]) Row(term keyspace.Term) (row Row, ok bool) {
	if !keyspace.ValidTerm(term, table.family, len(table.rows)) {
		return row, false
	}
	return table.rows[keyspace.TermOrdinal(term)-1], true
}

// Terms iterates every row against its canonical term in sealed order. It is
// the read a family owner uses when it must name the row it is emitting.
func (table Table[Row]) Terms() iter.Seq2[keyspace.Term, Row] {
	return func(yield func(keyspace.Term, Row) bool) {
		for index, row := range table.rows {
			if !yield(keyspace.MakeTerm(table.family, uint32(index+1)), row) {
				return
			}
		}
	}
}

// RowsBuilder accumulates one row list and seals it. It exists so a
// compaction loop appends rows directly into the storage it will seal instead
// of copying a scratch slice at the boundary, and so a relation that carries
// no canonical term still gets the same one-shot seal discipline.
type RowsBuilder[Row any] struct {
	rows   []Row
	sealed bool
}

// Append adds one row and returns its dense index.
func (builder *RowsBuilder[Row]) Append(row Row) (int, bool) {
	if builder == nil || builder.sealed || !keyspace.TermOrdinalFits(len(builder.rows)+1) {
		return 0, false
	}
	builder.rows = append(builder.rows, row)
	return len(builder.rows) - 1, true
}

// At reads one row already accumulated. A builder is not a query surface: this
// exists only for a compaction pass that must revisit a row it just placed.
func (builder *RowsBuilder[Row]) At(index int) (row Row, ok bool) {
	if builder == nil || index < 0 || index >= len(builder.rows) {
		return row, false
	}
	return builder.rows[index], true
}

// Len is the row count accumulated so far.
func (builder *RowsBuilder[Row]) Len() int {
	if builder == nil {
		return 0
	}
	return len(builder.rows)
}

// Seal hands the accumulated rows over as an immutable list and closes the
// builder. The builder retains no write path into the sealed values.
func (builder *RowsBuilder[Row]) Seal() Rows[Row] {
	if builder == nil {
		return Rows[Row]{}
	}
	builder.sealed = true
	values := builder.rows
	builder.rows = nil
	return Rows[Row]{rows: values}
}

// TableBuilder is a RowsBuilder that numbers what it accumulates under a fixed
// canonical family, exactly as Table is a Rows that does.
type TableBuilder[Row any] struct {
	RowsBuilder[Row]
	family keyspace.Family
}

// NewTableBuilder opens a builder for family. A family outside the closed
// keyspace inventory yields a builder that refuses every append.
func NewTableBuilder[Row any](family keyspace.Family) *TableBuilder[Row] {
	if family <= keyspace.FamilyInvalid || family >= keyspace.FamilyCount {
		return &TableBuilder[Row]{RowsBuilder: RowsBuilder[Row]{sealed: true}}
	}
	return &TableBuilder[Row]{family: family}
}

// Append adds one row and returns the canonical term that now names it.
func (builder *TableBuilder[Row]) Append(row Row) (keyspace.Term, bool) {
	if builder == nil {
		return 0, false
	}
	index, ok := builder.RowsBuilder.Append(row)
	if !ok {
		return 0, false
	}
	return keyspace.MakeTerm(builder.family, uint32(index+1)), true
}

// Seal hands the accumulated rows over as an immutable Table and closes the
// builder.
func (builder *TableBuilder[Row]) Seal() Table[Row] {
	if builder == nil || builder.family <= keyspace.FamilyInvalid || builder.family >= keyspace.FamilyCount {
		return Table[Row]{}
	}
	return Table[Row]{Rows: builder.RowsBuilder.Seal(), family: builder.family}
}
