package tuple

import (
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// NewAbsent constructs a closed-world tuple for one denominator member that
// is already authenticated by the mounted denominator witness. The mounted
// row directory redeems the row's canonical lineage atom; this constructor
// never derives provenance from RowID or creates a zero-lineage exception.
func NewAbsent(mounted witness.Mounted, denominator binding.DenominatorWitness, scope witness.Scope, row model.RowID, columns []model.ColumnID) (Tuple, bool) {
	if !authenticatedDenominator(mounted, denominator, row) || !scope.ValidFor(mounted.RuntimeFence()) || columns == nil {
		return Tuple{}, false
	}
	types, ok := columnTypesForRelation(mounted, row.Relation(), columns)
	if !ok {
		return Tuple{}, false
	}
	presence, ok := model.NewPresence(model.ProvenAbsent)
	if !ok {
		return Tuple{}, false
	}
	cells := make([]Cell, len(columns))
	for index, column := range columns {
		cells[index] = Cell{column: column, typeID: types[index], presence: presence, source: 0}
	}
	lineage, ok := mounted.RowLineage(row)
	if !ok {
		return Tuple{}, false
	}
	return newTuple(mounted, scope, lineage, []model.RowID{row}, cells)
}

// ExtendAbsent closes one existing tuple over the requested relation columns.
// Existing cells are preserved byte-for-byte. Only columns absent from the
// tuple receive a ProvenAbsent cell. The source tuple retains its exact
// validated lineage; the denominator witness remains the authority for the
// selected row.
func ExtendAbsent(mounted witness.Mounted, denominator binding.DenominatorWitness, source Tuple, relation model.RelationID, columns []model.ColumnID) (Tuple, bool) {
	if !source.ValidFor(mounted) || !relation.Available() || columns == nil {
		return Tuple{}, false
	}
	sourceIndex, sourceOK := sourceOrdinal(source, relation)
	row, rowOK := source.SourceAt(sourceIndex)
	if !sourceOK || !rowOK || !authenticatedDenominator(mounted, denominator, row) {
		return Tuple{}, false
	}
	types, ok := columnTypesForRelation(mounted, relation, columns)
	if !ok {
		return Tuple{}, false
	}
	result := source.Cells()
	existing := make(map[model.ColumnID]struct{}, len(result))
	for _, cell := range result {
		if !cell.available(mounted.RuntimeFence()) {
			return Tuple{}, false
		}
		if _, duplicate := existing[cell.Column()]; duplicate {
			return Tuple{}, false
		}
		existing[cell.Column()] = struct{}{}
	}
	presence, ok := model.NewPresence(model.ProvenAbsent)
	if !ok {
		return Tuple{}, false
	}
	for index, column := range columns {
		if _, found := existing[column]; found {
			cell, cellOK := source.CellFor(column)
			if !cellOK || cell.Type() != types[index] {
				return Tuple{}, false
			}
			continue
		}
		result = append(result, Cell{column: column, typeID: types[index], presence: presence, source: uint32(sourceIndex)})
		existing[column] = struct{}{}
	}
	return newTuple(mounted, source.scope, source.lineage, source.sources, result)
}

func authenticatedDenominator(mounted witness.Mounted, denominator binding.DenominatorWitness, row model.RowID) bool {
	if !mounted.Available() || !denominator.ValidFor(mounted.RuntimeFence()) || !row.Available() || denominator.Relation() != row.Relation() || !denominator.Contains(row) {
		return false
	}
	// A q-specific partition witness is issued by the binding directory and
	// intentionally is not retained in Mounted's global denominator map.  The
	// mounted row directory authenticates every member without converting a
	// RowID into a physical key or reopening a global child witness.
	position, ok := mounted.RowIndex(row.Relation(), row)
	if !ok {
		return false
	}
	redeemed, ok := mounted.RowAt(row.Relation(), position)
	return ok && redeemed == row
}

func columnTypesForRelation(mounted witness.Mounted, relation model.RelationID, columns []model.ColumnID) ([]model.TypeID, bool) {
	if !mounted.Available() || !relation.Available() || columns == nil {
		return nil, false
	}
	typesByColumn := make(map[model.ColumnID]model.TypeID)
	for _, schema := range mounted.Columns() {
		if !schema.Available() || schema.Relation() != relation {
			continue
		}
		if _, duplicate := typesByColumn[schema.ID()]; duplicate {
			return nil, false
		}
		typesByColumn[schema.ID()] = schema.Type()
	}
	result := make([]model.TypeID, len(columns))
	seen := make(map[model.ColumnID]struct{}, len(columns))
	for index, column := range columns {
		if !column.Available() || column.Relation() != relation {
			return nil, false
		}
		if _, duplicate := seen[column]; duplicate {
			return nil, false
		}
		seen[column] = struct{}{}
		typeID, found := typesByColumn[column]
		if !found || !typeID.Available() {
			return nil, false
		}
		result[index] = typeID
	}
	return result, true
}
