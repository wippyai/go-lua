package tuple

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/state/read"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// ProjectInto is the logical source-to-target projection seam. The complete
// mapping vector is an arrangement-owned sealed binding; tuple code cannot
// receive a caller-created projection list or rediscover target membership.
// It carries the destination tuple's authenticated source RowIDs into the
// result while retaining only the relabelled projected cells. Destination
// payload cells are not appended, so a mapped target column cannot be
// duplicated.
func ProjectInto(mounted witness.Mounted, reader read.Reader, source, destination Tuple, bound arrangement.ProjectBinding) (Tuple, bool) {
	if !source.ValidFor(mounted) || !destination.ValidFor(mounted) || !reader.Available() || !reader.Layout().ValidFor(mounted.Fence()) || !bound.Available() {
		return Tuple{}, false
	}
	scope, scopeOK := reader.Conjoin(source.scope, destination.scope)
	if !scopeOK || !scope.ValidFor(mounted.RuntimeFence()) {
		return Tuple{}, false
	}
	lineageAuthority, lineageAuthorityOK := mounted.Lineage()
	if !lineageAuthorityOK || lineageAuthority == nil {
		return Tuple{}, false
	}
	lineage, lineageOK := lineageAuthority.Join(source.lineage, destination.lineage)
	if !lineageOK {
		return Tuple{}, false
	}
	target := bound.Target().Access().Relation()
	destinationSource, sourceOK := sourceOrdinal(destination, target)
	if !sourceOK {
		return Tuple{}, false
	}
	cells, ok := projectedBindingCells(source, reader, bound, uint32(source.SourceLen()+destinationSource))
	if !ok {
		return Tuple{}, false
	}
	sources := make([]model.RowID, 0, source.SourceLen()+destination.SourceLen())
	sources = append(sources, source.sources...)
	sources = append(sources, destination.sources...)
	return newTuple(mounted, scope, lineage, sources, cells)
}

// ProjectColumns retains the exact ordered cell positions sealed by a
// ColumnProject binding. It preserves the child's row-source spine, scope,
// and lineage; it never locates a cell by ColumnID at evaluation time.
//
// This is deliberately distinct from ProjectInto: ColumnProject does not
// construct a target row or introduce a destination occurrence. It is the
// narrow carrier used to pass a destination fact's semantic writable subset
// into a vertical Merge while the source row continues to authenticate its
// address/key authority.
func ProjectColumns(mounted witness.Mounted, source Tuple, bound arrangement.ColumnProjectBinding) (Tuple, bool) {
	if !mounted.Available() || !source.ValidFor(mounted) || !bound.Available() {
		return Tuple{}, false
	}
	slots := bound.SlotCount()
	if slots == 0 {
		return Tuple{}, false
	}
	values := bound.Values().Columns()
	if len(values) != slots {
		return Tuple{}, false
	}
	projected := make([]Cell, slots)
	for index := 0; index < slots; index++ {
		slot, slotOK := bound.SlotAt(index)
		if !slotOK || slot.Column() != values[index] {
			return Tuple{}, false
		}
		cell, cellOK := source.At(int(slot.Cell()))
		if !cellOK || cell.Column() != slot.Column() {
			return Tuple{}, false
		}
		projected[index] = cell
	}
	return newTuple(mounted, source.Scope(), source.Lineage(), source.Sources(), projected)
}

// projectedBindingCells redeems the sealed authored target vector directly.
// Mapping order is therefore the order guaranteed by ProjectBinding, and
// target type authority comes from the already-bound destination Reader. No
// temporary projection vector, target map, or mounted column catalogue is
// built on the evaluator path.
func projectedBindingCells(source Tuple, reader read.Reader, bound arrangement.ProjectBinding, targetSource uint32) ([]Cell, bool) {
	cells := make([]Cell, bound.MappingCount())
	for index := 0; index < bound.MappingCount(); index++ {
		mapping, mappingOK := bound.MappingAt(index)
		if !mappingOK {
			return nil, false
		}
		input, inputOK := source.CellFor(mapping.Source())
		if !inputOK {
			return nil, false
		}
		targetType, typeOK := reader.Type(mapping.Target())
		if !typeOK || targetType != input.Type() {
			return nil, false
		}
		cells[index] = Cell{column: mapping.Target(), typeID: input.Type(), value: input.Value(), presence: input.Presence(), source: targetSource}
	}
	return cells, true
}

// sourceOrdinal resolves one unique source row occurrence without collapsing a
// self-join by nominal relation. Operators that need a target row must carry a
// unique target relation in their sealed binding; an ambiguous destination is
// malformed rather than silently choosing its first row.
func sourceOrdinal(value Tuple, relation model.RelationID) (int, bool) {
	if !value.Available() || !relation.Available() {
		return 0, false
	}
	result := -1
	for index := 0; index < value.SourceLen(); index++ {
		row, ok := value.SourceAt(index)
		if !ok || row.Relation() != relation {
			continue
		}
		if result >= 0 {
			return 0, false
		}
		result = index
	}
	return result, result >= 0
}
