package contract

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/schema/denominator"
)

var errCountRows = errors.New("target: invalid denominator counts")

// CountRows returns the immutable Target denominator rows frozen at seal.
// Target's Contract is only the composition root: each sealed owner issues
// its own vector and this method joins those vectors once. It does not walk
// operation, protocol, or boot rows a second time.
func (c *Contract) CountRows() denominator.CountRows {
	if c == nil || !c.sealed {
		return denominator.CountRows{}
	}
	return c.counts
}

func (c *Contract) publishCountRows() (denominator.CountRows, error) {
	if c == nil {
		return denominator.CountRows{}, errCountRows
	}
	contractRow, ok := denominator.NewCountRow(denominator.GeneratedTargetIDs().TargetContract, 1)
	if !ok {
		return denominator.CountRows{}, errCountRows
	}
	contractRows, ok := denominator.NewCountRows([]denominator.CountRow{contractRow})
	if !ok {
		return denominator.CountRows{}, errCountRows
	}
	rows, ok := denominator.MergeCountRows(
		contractRows,
		c.Operations.CountRows(),
		c.protocols.CountRows(),
		c.Table.CountRows(),
	)
	if !ok || !denominator.GeneratedCountRowsCompleteForOwners(rows, denominator.RelationOwnerTarget) {
		return denominator.CountRows{}, errCountRows
	}
	return rows, nil
}
