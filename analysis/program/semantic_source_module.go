package program

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/imports"
)

func programModuleCounts(view imports.View) ([6]int, error) {
	var counts [6]int
	if !view.ContentID().Available() {
		return counts, errors.New("unavailable Module view")
	}
	importsCount := view.Count()
	entry := view.Entry()
	returns := entry.ReturnCount()
	rootCells, rootFunctions, members := 0, 0, entry.MemberTotal()
	for index := 0; index < returns; index++ {
		returned, ok := entry.ReturnAt(index)
		if !ok {
			return counts, errors.New("invalid Module return column")
		}
		rootCount, ok := entry.RootCount(returned)
		if !ok {
			return counts, errors.New("invalid Module root column")
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
	if !programSemanticSourceCountsFit(importsCount, returns, rootCells, members, rootFunctions) {
		return counts, errors.New("invalid Module semantic cardinality")
	}
	return [...]int{importsCount, importsCount, returns, rootCells, members, rootFunctions}, nil
}
