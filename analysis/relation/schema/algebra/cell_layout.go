package algebra

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// CellLayout is the closed physical cell order produced by one relational
// expression.  It is schema data rather than a runtime tuple: lowering,
// checking, and mount use the same small contract to agree on the immutable
// coordinates later named by SlotSource.
//
// Sources retain row-occurrence order.  Cells retain their owning source
// ordinal, so a legal self-join is never reduced to a nominal relation lookup.
type CellLayout struct {
	sources []model.RelationID
	cells   []CellLayoutCell
}

// CellLayoutCell is one positional cell in a CellLayout.
type CellLayoutCell struct {
	column model.ColumnID
	source uint32
}

// NewCellLayoutCell names one column owned by one source occurrence.  The
// enclosing layout checks that the source ordinal is in range.
func NewCellLayoutCell(column model.ColumnID, source uint32) CellLayoutCell {
	return CellLayoutCell{column: column, source: source}
}

func (cell CellLayoutCell) Column() model.ColumnID { return cell.column }
func (cell CellLayoutCell) Source() uint32         { return cell.source }

// NewCellLayout seals an ordered source/cell vector.  The constructor permits
// equal nominal columns from distinct source occurrences (a self-join), but a
// source occurrence may not publish the same column twice.
func NewCellLayout(sources []model.RelationID, cells []CellLayoutCell) (CellLayout, bool) {
	if len(sources) == 0 || cells == nil {
		return CellLayout{}, false
	}
	copySources := append([]model.RelationID(nil), sources...)
	for _, source := range copySources {
		if !source.Available() {
			return CellLayout{}, false
		}
	}
	// Preserve an authenticated zero-width projection as a non-nil empty
	// vector. Nil is the unavailable sentinel; an empty vector is a valid
	// relation occurrence carrying row identity but no payload cells.
	copyCells := make([]CellLayoutCell, len(cells))
	copy(copyCells, cells)
	seen := make(map[struct {
		source uint32
		column model.ColumnID
	}]struct{}, len(copyCells))
	for _, cell := range copyCells {
		if !cell.column.Available() || int(cell.source) >= len(copySources) || cell.column.Relation() != copySources[cell.source] {
			return CellLayout{}, false
		}
		key := struct {
			source uint32
			column model.ColumnID
		}{source: cell.source, column: cell.column}
		if _, duplicate := seen[key]; duplicate {
			return CellLayout{}, false
		}
		seen[key] = struct{}{}
	}
	return CellLayout{sources: copySources, cells: copyCells}, true
}

// InputCellLayout constructs one exact relation-row layout.
func InputCellLayout(relation model.RelationID, columns []model.ColumnID) (CellLayout, bool) {
	if !relation.Available() {
		return CellLayout{}, false
	}
	cells := make([]CellLayoutCell, len(columns))
	for index, column := range columns {
		cells[index] = NewCellLayoutCell(column, 0)
	}
	return NewCellLayout([]model.RelationID{relation}, cells)
}

// Available reports whether this is a constructor-sealed physical cell
// coordinate.  Constructors validate the complete width-sensitive contract;
// hot callers redeem only the immutable value.
func (layout CellLayout) Available() bool {
	if len(layout.sources) == 0 || layout.cells == nil {
		return false
	}
	for _, source := range layout.sources {
		if !source.Available() {
			return false
		}
	}
	seen := make(map[struct {
		source uint32
		column model.ColumnID
	}]struct{}, len(layout.cells))
	for _, cell := range layout.cells {
		if !cell.column.Available() || int(cell.source) >= len(layout.sources) || cell.column.Relation() != layout.sources[cell.source] {
			return false
		}
		key := struct {
			source uint32
			column model.ColumnID
		}{source: cell.source, column: cell.column}
		if _, duplicate := seen[key]; duplicate {
			return false
		}
		seen[key] = struct{}{}
	}
	return true
}

func (layout CellLayout) SourceLen() int {
	if !layout.Available() {
		return 0
	}
	return len(layout.sources)
}

func (layout CellLayout) SourceAt(index int) (model.RelationID, bool) {
	if !layout.Available() || index < 0 || index >= len(layout.sources) {
		return model.RelationID{}, false
	}
	return layout.sources[index], true
}

func (layout CellLayout) Sources() []model.RelationID {
	if !layout.Available() {
		return nil
	}
	return append([]model.RelationID(nil), layout.sources...)
}

func (layout CellLayout) Len() int {
	if !layout.Available() {
		return 0
	}
	return len(layout.cells)
}

func (layout CellLayout) CellAt(index int) (CellLayoutCell, bool) {
	if !layout.Available() || index < 0 || index >= len(layout.cells) {
		return CellLayoutCell{}, false
	}
	return layout.cells[index], true
}

func (layout CellLayout) Cells() []CellLayoutCell {
	if !layout.Available() {
		return nil
	}
	return append([]CellLayoutCell(nil), layout.cells...)
}

// Equal compares the complete source/cell coordinate, including occurrence
// order.  It is intentionally stricter than nominal relation equality.
func (layout CellLayout) Equal(other CellLayout) bool {
	if !layout.Available() || !other.Available() || len(layout.sources) != len(other.sources) || len(layout.cells) != len(other.cells) {
		return false
	}
	for index, source := range layout.sources {
		if source != other.sources[index] {
			return false
		}
	}
	for index, cell := range layout.cells {
		if cell != other.cells[index] {
			return false
		}
	}
	return true
}

// Digest returns the stable identity of this exact source/cell coordinate.
// It is deliberately independent of an expression digest: the same physical
// output contract can be sealed by the compiler, checker, and mounted node
// without any layer reinterpreting ordinal positions.
func (layout CellLayout) Digest() identity.ContentID {
	if !layout.Available() {
		return identity.ContentID{}
	}
	parts := appendLength(nil, len(layout.sources))
	for _, source := range layout.sources {
		parts = appendRelation(parts, source)
	}
	parts = appendLength(parts, len(layout.cells))
	for _, cell := range layout.cells {
		parts = appendColumn(parts, cell.column)
		parts = appendUint32(parts, cell.source)
	}
	return derive("analysis/relation/schema/algebra/cell-layout/v1", parts)
}

// JoinCellLayouts is the physical cell law of Join and Append: retain every
// left occurrence, then append every right occurrence with its source shifted
// by the left source width.
func JoinCellLayouts(left, right CellLayout) (CellLayout, bool) {
	if !left.Available() || !right.Available() {
		return CellLayout{}, false
	}
	sources := append(left.Sources(), right.Sources()...)
	cells := make([]CellLayoutCell, 0, left.Len()+right.Len())
	cells = append(cells, left.Cells()...)
	offset := uint32(left.SourceLen())
	for _, cell := range right.Cells() {
		cells = append(cells, NewCellLayoutCell(cell.Column(), cell.Source()+offset))
	}
	return NewCellLayout(sources, cells)
}

// CompleteCellLayout is Complete's one output law.  It preserves every child
// cell, then appends exactly the denominator-relation columns which are absent
// from that child, in the sealed relation-contract order.  Reapplying the law
// is idempotent: once the complete vector is present it adds no second copy.
//
// Extending a sparse child needs one unambiguous denominator row occurrence.
// A structurally representable self-join with already-complete duplicate rows
// is retained unchanged (there is no extension to choose); a missing cell in
// that shape is refused rather than silently assigning it to one occurrence.
func CompleteCellLayout(child CellLayout, denominator model.DenominatorRef, columns []model.ColumnID) (CellLayout, bool) {
	if !child.Available() || !denominator.Available() || len(columns) == 0 {
		return CellLayout{}, false
	}
	denominatorSource := -1
	denominatorSources := 0
	for index, source := range child.sources {
		if source != denominator.Relation() {
			continue
		}
		denominatorSource = index
		denominatorSources++
	}
	if denominatorSource < 0 {
		return CellLayout{}, false
	}
	declared := make(map[model.ColumnID]struct{}, len(columns))
	for _, column := range columns {
		if !column.Available() || column.Relation() != denominator.Relation() {
			return CellLayout{}, false
		}
		if _, duplicate := declared[column]; duplicate {
			return CellLayout{}, false
		}
		declared[column] = struct{}{}
	}
	existing := make(map[model.ColumnID]CellLayoutCell, child.Len())
	present := make(map[model.ColumnID]bool, len(columns))
	for _, cell := range child.cells {
		if prior, duplicate := existing[cell.column]; duplicate {
			// Repeated source occurrences are legal structural Join output. A
			// Complete extension cannot select between them, but an already
			// complete child needs no selection and remains its own layout.
			if denominatorSources == 1 || prior.source == cell.source {
				return CellLayout{}, false
			}
		} else {
			existing[cell.column] = cell
		}
		if cell.column.Relation() == denominator.Relation() {
			present[cell.column] = true
		}
	}
	if denominatorSources != 1 {
		for _, column := range columns {
			if !present[column] {
				return CellLayout{}, false
			}
		}
		return NewCellLayout(child.sources, child.cells)
	}
	result := CellLayout{sources: append([]model.RelationID(nil), child.sources...), cells: append([]CellLayoutCell(nil), child.cells...)}
	for _, column := range columns {
		if cell, present := existing[column]; present {
			if cell.source != uint32(denominatorSource) {
				return CellLayout{}, false
			}
			continue
		}
		result.cells = append(result.cells, NewCellLayoutCell(column, uint32(denominatorSource)))
	}
	return NewCellLayout(result.sources, result.cells)
}

// ColumnProjectCellLayout retains exactly the child cells named by a
// ColumnProject contract. Sources remain intact because a projected cell's
// source ordinal is still an occurrence in the child tuple.
func ColumnProjectCellLayout(child CellLayout, slots []ColumnSlot) (CellLayout, bool) {
	if !child.Available() || len(slots) == 0 {
		return CellLayout{}, false
	}
	result := make([]CellLayoutCell, len(slots))
	seenColumns := make(map[model.ColumnID]struct{}, len(slots))
	seenCells := make(map[uint32]struct{}, len(slots))
	for index, slot := range slots {
		if _, duplicate := seenColumns[slot.Column()]; duplicate {
			return CellLayout{}, false
		}
		if _, duplicate := seenCells[slot.Cell()]; duplicate {
			return CellLayout{}, false
		}
		cell, ok := child.CellAt(int(slot.Cell()))
		if !ok || cell.Column() != slot.Column() {
			return CellLayout{}, false
		}
		seenColumns[slot.Column()] = struct{}{}
		seenCells[slot.Cell()] = struct{}{}
		result[index] = cell
	}
	return NewCellLayout(child.Sources(), result)
}

// ProjectCellLayout is ProjectInto's explicit output law. Project retains
// the child source spine, appends one target-row occurrence, and emits only
// the mapped target cells in authored mapping order. This mirrors tuple
// ProjectInto: destination payload is a lookup authority, not an appended
// duplicate output vector.
func ProjectCellLayout(child CellLayout, target model.RelationID, mappings []ColumnMapping) (CellLayout, bool) {
	if !child.Available() || !target.Available() || len(mappings) == 0 {
		return CellLayout{}, false
	}
	sources := append(child.Sources(), target)
	targetSource := uint32(child.SourceLen())
	cells := make([]CellLayoutCell, len(mappings))
	seenTargets := make(map[model.ColumnID]struct{}, len(mappings))
	for index, mapping := range mappings {
		source, destination := mapping.Source(), mapping.Target()
		if !source.Available() || !destination.Available() || destination.Relation() != target {
			return CellLayout{}, false
		}
		if _, duplicate := seenTargets[destination]; duplicate {
			return CellLayout{}, false
		}
		matches := 0
		for _, cell := range child.cells {
			if cell.column == source {
				matches++
			}
		}
		// ProjectInto redeems source cells by one exact nominal mapping. A
		// duplicated source column would make tuple.CellFor ambiguous, so it
		// has no honest physical output layout.
		if matches != 1 {
			return CellLayout{}, false
		}
		seenTargets[destination] = struct{}{}
		cells[index] = NewCellLayoutCell(destination, targetSource)
	}
	return NewCellLayout(sources, cells)
}

// ProjectColumnsCellLayout is retained as a compact compatibility spelling
// for callers that mean the positional ColumnProject operator. New code
// should use ColumnProjectCellLayout so ProjectInto remains explicit.
func ProjectColumnsCellLayout(child CellLayout, slots []ColumnSlot) (CellLayout, bool) {
	return ColumnProjectCellLayout(child, slots)
}
