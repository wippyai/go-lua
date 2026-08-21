package boot

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
)

// CountRows publishes the complete boot-owner contribution to Target's
// denominator directly from the sealed dense boot planes.
func (table Table) CountRows() denominator.CountRows {
	ids := denominator.GeneratedTargetIDs()
	counts := make([]denominator.CountRow, 0, 4)
	add := func(id schema.EntryID, value int) bool {
		if value < 0 {
			return false
		}
		row, ok := denominator.NewCountRow(id, uint64(value))
		if !ok {
			return false
		}
		counts = append(counts, row)
		return true
	}
	if !add(ids.TargetBoot, table.roots.Count()) ||
		!add(ids.TargetBootEntry, table.entries.Count()) ||
		!add(ids.TargetBootMetatableAttachment, table.metatables.Count()) ||
		!add(ids.TargetBootBinding, table.bindings.Count()) {
		return denominator.CountRows{}
	}
	rows, ok := denominator.NewCountRows(counts)
	if !ok {
		return denominator.CountRows{}
	}
	return rows
}
