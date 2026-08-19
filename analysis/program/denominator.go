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

func combineProgramCountRows(sourceRows, flowRows, staticRows, moduleRows denominator.CountRows) (denominator.CountRows, error) {
	rows, ok := denominator.MergeCountRows(sourceRows, flowRows, staticRows, moduleRows)
	if !ok || !denominator.GeneratedCountRowsCompleteForOwners(rows, programOwners()...) {
		return denominator.CountRows{}, errCountRows
	}
	return rows, nil
}

// programOwners is the owner-group membership the four Program-interior
// rows are combined and validated over, read off RelationOwner.Program
// rather than restated as a second list.
func programOwners() []denominator.RelationOwner {
	var owners []denominator.RelationOwner
	for owner := denominator.RelationOwnerProgramSource; owner.Available(); owner++ {
		if owner.Program() {
			owners = append(owners, owner)
		}
	}
	return owners
}
