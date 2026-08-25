package relationoracle

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// SelectByScope applies the logical scope-entailment filter. Filtering runs
// before Apply or Publish; this package never decodes a scope formula.
func SelectByScope(input Relation, requested Scope, scopes ScopeAlgebra) Relation {
	if !input.Available() || !requested.Available() {
		return Relation{}
	}
	if scopes == nil {
		scopes = ExactScope{}
	}
	rows := make([]Row, 0, len(input.rows))
	for _, row := range input.rows {
		if scopes.Entails(row.scope, requested) {
			rows = append(rows, row)
		}
	}
	result, ok := NewRelation(input.id, rows)
	if !ok {
		return Relation{}
	}
	return result
}

func (relation Relation) Select(requested Scope, scopes ScopeAlgebra) Relation {
	return SelectByScope(relation, requested, scopes)
}

// DenominatorEntry is one authenticated logical denominator row. It carries
// only model.RowID and canonical formula identity, not mounted membership.
type DenominatorEntry struct {
	id    model.RowID
	scope Scope
}

func NewDenominatorEntry(id model.RowID, scope Scope) (DenominatorEntry, bool) {
	if !id.Available() || !scope.Available() {
		return DenominatorEntry{}, false
	}
	return DenominatorEntry{id: id, scope: scope}, true
}

func (entry DenominatorEntry) Available() bool {
	return entry.id.Available() && entry.scope.Available()
}
func (entry DenominatorEntry) ID() model.RowID { return entry.id }
func (entry DenominatorEntry) Scope() Scope    { return entry.scope }

type Denominator struct {
	relation model.RelationID
	entries  []DenominatorEntry
	valid    bool
}

func NewDenominator(relation model.RelationID, entries []DenominatorEntry) (Denominator, bool) {
	if !relation.Available() {
		return Denominator{}, false
	}
	copyOf := append([]DenominatorEntry(nil), entries...)
	for _, entry := range copyOf {
		if !entry.Available() || entry.id.Relation() != relation {
			return Denominator{}, false
		}
	}
	sort.Slice(copyOf, func(left, right int) bool { return rowIDLess(copyOf[left].id, copyOf[right].id) })
	for index := 1; index < len(copyOf); index++ {
		if copyOf[index-1].id == copyOf[index].id {
			return Denominator{}, false
		}
	}
	if copyOf == nil {
		copyOf = make([]DenominatorEntry, 0)
	}
	return Denominator{relation: relation, entries: copyOf, valid: true}, true
}

func (denominator Denominator) Available() bool {
	return denominator.valid && denominator.relation.Available() && denominator.entries != nil
}
func (denominator Denominator) Relation() model.RelationID { return denominator.relation }
func (denominator Denominator) Entries() []DenominatorEntry {
	return append([]DenominatorEntry(nil), denominator.entries...)
}

// ColumnType names a target column and its declared TypeID for Complete.
// TypeID is required because a denominator can contain no present row from
// which a target type could be inferred.
type ColumnType struct {
	column model.ColumnID
	typeID model.TypeID
}

func NewColumnType(column model.ColumnID, typeID model.TypeID) ColumnType {
	return ColumnType{column: column, typeID: typeID}
}
func (column ColumnType) Column() model.ColumnID { return column.column }
func (column ColumnType) Type() model.TypeID     { return column.typeID }
func (column ColumnType) Available() bool {
	return column.column.Available() && column.typeID.Available()
}

// Complete closes input against an authenticated denominator and materializes
// every requested missing cell as ProvenAbsent. Present (including opaque)
// cells remain distinct from absence.
func Complete(input Relation, denominator Denominator, columns []ColumnType) Relation {
	if !input.Available() || !denominator.Available() || input.id != denominator.relation || !validColumnTypes(columns) {
		return Relation{}
	}
	rows := make([]Row, 0, len(denominator.entries))
	for _, entry := range denominator.entries {
		existing, found := input.Row(entry.id)
		if !found {
			cells := make([]Cell, 0, len(columns))
			for _, column := range columns {
				cell, ok := AbsentCell(column.column, column.typeID)
				if !ok {
					return Relation{}
				}
				cells = append(cells, cell)
			}
			row, ok := NewRow(entry.id, entry.scope, cells)
			if !ok {
				return Relation{}
			}
			rows = append(rows, row)
			continue
		}
		cells := existing.Cells()
		for _, column := range columns {
			if _, ok := existing.Cell(column.column); ok {
				continue
			}
			cell, ok := AbsentCell(column.column, column.typeID)
			if !ok {
				return Relation{}
			}
			cells = append(cells, cell)
		}
		row, ok := NewRow(existing.id, existing.scope, cells)
		if !ok {
			return Relation{}
		}
		rows = append(rows, row)
	}
	result, ok := NewRelation(denominator.relation, rows)
	if !ok {
		return Relation{}
	}
	return result
}

func validColumnTypes(columns []ColumnType) bool {
	seen := make(map[model.ColumnID]struct{}, len(columns))
	for _, column := range columns {
		if !column.Available() {
			return false
		}
		if _, duplicate := seen[column.column]; duplicate {
			return false
		}
		seen[column.column] = struct{}{}
	}
	return true
}
