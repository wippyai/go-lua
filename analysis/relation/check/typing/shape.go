package typing

import "github.com/wippyai/go-lua/analysis/relation/schema/model"

// shape is the checker-only typed row shape. A derived Join can have no
// nominal relation until a Project gives it one, so relation is optional;
// columns and their TypeIDs remain fully nominal and checked.
type shape struct {
	relation model.RelationID
	columns  []columnType
	keys     []model.KeyID
}

type columnType struct {
	ID   model.ColumnID
	Type model.TypeID
}

type outputIdentity struct {
	Relation model.RelationID
	Column   model.ColumnID
}

func (value shape) valid() bool { return len(value.columns) > 0 }

func (value shape) column(id model.ColumnID) (columnType, bool) {
	for _, column := range value.columns {
		if column.ID == id {
			return column, true
		}
	}
	return columnType{}, false
}

func (value shape) hasKey(id model.KeyID) bool {
	for _, key := range value.keys {
		if key == id {
			return true
		}
	}
	return false
}

func sameShape(left, right shape) bool {
	if left.relation != right.relation || len(left.columns) != len(right.columns) {
		return false
	}
	for index := range left.columns {
		if left.columns[index] != right.columns[index] {
			return false
		}
	}
	return true
}

func uniqueColumnCount(columns []columnType) int {
	seen := make(map[model.ColumnID]struct{}, len(columns))
	for _, column := range columns {
		seen[column.ID] = struct{}{}
	}
	return len(seen)
}
