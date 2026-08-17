package imports

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
)

var errModuleCounts = errors.New("program/imports: invalid denominator counts")

// CountRows derives Module's native denominator rows from its immutable
// import and entry projections. The rows are neutral identities so Link and
// Snapshot can mount them without importing Module internals.
func CountRows(view View) (denominator.CountRows, error) {
	if !view.ContentID().Available() {
		return denominator.CountRows{}, errModuleCounts
	}
	importsCount := view.Count()
	entry := view.Entry()
	returns := entry.ReturnCount()
	rootCells, rootFunctions, members := 0, 0, entry.MemberTotal()
	for index := 0; index < returns; index++ {
		returned, ok := entry.ReturnAt(index)
		if !ok {
			return denominator.CountRows{}, errModuleCounts
		}
		rootCount, ok := entry.RootCount(returned)
		if !ok {
			return denominator.CountRows{}, errModuleCounts
		}
		for rootIndex := 0; rootIndex < rootCount; rootIndex++ {
			if _, ok := entry.RootCell(returned, rootIndex); ok {
				rootCells++
			}
			if _, ok := entry.RootFunction(returned, rootIndex); ok {
				rootFunctions++
			}
		}
	}
	ids := denominator.GeneratedProgramModuleIDs()
	values := []struct {
		id    schema.EntryID
		value int
	}{
		{ids.ProgramModuleImport, importsCount},
		{ids.ProgramModuleRequest, importsCount},
		{ids.ProgramModuleEntry, returns},
		{ids.ProgramModuleEntryRootCell, rootCells},
		{ids.ProgramModuleEntryMember, members},
		{ids.ProgramModuleEntryRootFunction, rootFunctions},
	}
	rows := make([]denominator.CountRow, 0, len(values))
	for _, value := range values {
		row, ok := moduleCountRow(value.id, value.value)
		if !ok {
			return denominator.CountRows{}, errModuleCounts
		}
		rows = append(rows, row)
	}
	sealed, ok := denominator.NewCountRows(rows)
	if !ok {
		return denominator.CountRows{}, errModuleCounts
	}
	return sealed, nil
}

func moduleCountRow(id schema.EntryID, value int) (denominator.CountRow, bool) {
	if value < 0 || uint64(value) > uint64(keyspace.MaxTermOrdinal) {
		return denominator.CountRow{}, false
	}
	return denominator.NewCountRow(id, uint64(value))
}
