package static

import "github.com/wippyai/go-lua/analysis/schema/denominator"

// CountRows returns the sealed LinkStatic denominator column. The row is
// derived from this owner's native detached namespace schema; no Program
// relation, source publication, or foreign vocabulary is consulted.
func (v Cold) CountRows() (denominator.CountRows, bool) {
	if !v.live() {
		return denominator.CountRows{}, false
	}
	row, ok := denominator.NewCountRow(
		denominator.GeneratedLinkStaticIDs().LinkStatic,
		uint64(len(v.schema)),
	)
	if !ok {
		return denominator.CountRows{}, false
	}
	rows, ok := denominator.NewCountRows([]denominator.CountRow{row})
	if !ok {
		return denominator.CountRows{}, false
	}
	return rows, true
}

// CountRows returns the same sealed owner column from a finalized component.
func (c *Component) CountRows() (denominator.CountRows, bool) {
	if c == nil {
		return denominator.CountRows{}, false
	}
	return c.Cold().CountRows()
}
