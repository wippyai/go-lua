package tuple

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/state/read"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// Input is the sole read.Row-to-Tuple boundary. It accepts only a row owned
// by the exact Reader, verifies that the reader's mounted layout belongs to
// the supplied runtime, and requires every delivered cell to live in exactly
// the row's normalized cofiber. A merely-entailing cell scope is refused: the
// Reader must split it into a common fiber before it reaches evaluation.
func Input(mounted witness.Mounted, reader read.Reader, row read.Row) (Tuple, bool) {
	if !mounted.Available() || !reader.Available() || row == nil || !reader.Owns(row) || !row.Available() {
		return Tuple{}, false
	}
	layout := reader.Layout()
	if !layout.Available() || !layout.ValidFor(mounted.Fence()) || row.ID().Relation() != layout.Access().Relation() || !row.Scope().ValidFor(mounted.RuntimeFence()) {
		return Tuple{}, false
	}
	declared := layout.Columns()
	provided := row.Cells()
	if provided == nil {
		provided = []read.Cell{}
	}
	if len(provided) != len(declared) {
		return Tuple{}, false
	}
	cells := make([]Cell, len(provided))
	for index, source := range provided {
		if !source.Available() || source.Column() != declared[index] || source.Column().Relation() != row.ID().Relation() || !source.Scope().Same(row.Scope()) {
			return Tuple{}, false
		}
		typeID, ok := reader.Type(source.Column())
		if !ok || typeID != source.Type() {
			return Tuple{}, false
		}
		cells[index] = Cell{column: source.Column(), typeID: source.Type(), value: source.Value(), presence: source.Presence(), source: 0}
	}
	return newTuple(mounted, row.Scope(), row.Lineage(), []model.RowID{row.ID()}, cells)
}
