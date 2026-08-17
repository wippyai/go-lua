package program

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
)

var errCountRows = errors.New("program: invalid owner denominator counts")

// CountRows returns the immutable Program denominator rows frozen at root
// publication. The root only combines the four owner-local sets; it never
// reconstructs a relation count from another owner's columns.
func (program *Program) CountRows() denominator.CountRows {
	if program == nil || !program.Available() {
		return denominator.CountRows{}
	}
	return program.counts
}

func combineProgramCountRows(sourceRows, flowRows, staticRows, moduleRows denominator.CountRows) (denominator.CountRows, error) {
	rows, ok := denominator.MergeCountRows(sourceRows, flowRows, staticRows, moduleRows)
	if !ok {
		return denominator.CountRows{}, errCountRows
	}
	expected := make(map[schema.EntryID]struct{})
	for _, entry := range denominator.GeneratedRelationEntries() {
		if entry == nil {
			return denominator.CountRows{}, errCountRows
		}
		switch entry.Owner() {
		case denominator.RelationOwnerProgramSource,
			denominator.RelationOwnerProgramFlow,
			denominator.RelationOwnerProgramStatic,
			denominator.RelationOwnerProgramModule:
			expected[entry.ID()] = struct{}{}
		}
	}
	if len(expected) == 0 || rows.Count() != len(expected) {
		return denominator.CountRows{}, errCountRows
	}
	for index := 0; index < rows.Count(); index++ {
		row, ok := rows.At(index)
		if !ok {
			return denominator.CountRows{}, errCountRows
		}
		if _, known := expected[row.ID()]; !known {
			return denominator.CountRows{}, errCountRows
		}
	}
	return rows, nil
}
