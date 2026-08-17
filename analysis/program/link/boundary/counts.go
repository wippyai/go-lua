package boundary

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/schema/denominator"
)

var errCountRows = errors.New("link/boundary: invalid denominator counts")

// CountRows returns Boundary's sealed native denominator column. The only
// declared Boundary row is the factorized Application×Operation predicate;
// its cardinality is derived once while Build seals the exact authority.
func (c *Component) CountRows() (denominator.CountRows, bool) {
	if c == nil || c.authority == nil || c.authority.component != c || !c.authority.countRows.Available() {
		return denominator.CountRows{}, false
	}
	return c.authority.countRows, true
}

func (c *Component) buildCountRows() (denominator.CountRows, error) {
	if c == nil || c.authority == nil || c.authority.component != c {
		return denominator.CountRows{}, errCountRows
	}
	cardinality, ok := c.Cardinality()
	if !ok || cardinality < 0 {
		return denominator.CountRows{}, errCountRows
	}
	row, ok := denominator.NewCountRow(denominator.GeneratedLinkBoundaryIDs().LinkBoundary, uint64(cardinality))
	if !ok {
		return denominator.CountRows{}, errCountRows
	}
	rows, ok := denominator.NewCountRows([]denominator.CountRow{row})
	if !ok {
		return denominator.CountRows{}, errCountRows
	}
	return rows, nil
}
