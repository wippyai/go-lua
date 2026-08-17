package control

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// CellMark identifies the start of one loop's pending per-iteration Cells.
func (w *Writer) CellMark() int {
	return w.cellLen
}

// RememberCell retains one loop Cell in declaration order.
func (w *Writer) RememberCell(cell keyspace.Term) error {
	if cell == 0 {
		return fmt.Errorf("lualower: could not retain loop Cell")
	}
	w.appendCell(cell)
	return nil
}

func (w *Writer) appendCell(cell keyspace.Term) {
	if w.cellLen < len(w.cellInline) {
		w.cellInline[w.cellLen] = cell
		w.cellLen++
		return
	}
	if w.cellLen == len(w.cellInline) {
		w.cellOverflow = append(w.cellOverflow[:0], w.cellInline[:]...)
	}
	w.cellOverflow = append(w.cellOverflow, cell)
	w.cellLen++
}

func (w *Writer) cellSlice() []keyspace.Term {
	if w.cellLen <= len(w.cellInline) {
		return w.cellInline[:w.cellLen]
	}
	return w.cellOverflow[:w.cellLen]
}

func (w *Writer) truncateCells(mark int) {
	w.cellLen = mark
	if mark <= len(w.cellInline) {
		w.cellOverflow = w.cellOverflow[:0]
		return
	}
	w.cellOverflow = w.cellOverflow[:mark]
}
