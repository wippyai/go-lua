package program

import (
	"errors"

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

func combineProgramCountRows(sourceRows, flowRows, staticRows denominator.CountRows) (denominator.CountRows, error) {
	rows, ok := denominator.MergeCountRows(sourceRows, flowRows, staticRows)
	if !ok || !denominator.GeneratedCountRowsCompleteForOwners(rows,
		denominator.RelationOwnerProgramSource,
		denominator.RelationOwnerProgramFlow,
		denominator.RelationOwnerProgramStatic,
	) {
		return denominator.CountRows{}, errCountRows
	}
	return rows, nil
}
