package tuple

import (
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// Merge reduces one ordered set of alternative tuples into one tuple.  The
// alternatives must already be one sealed range: they carry the same mounted
// runtime, cofiber scope, source-row spine, and cell shape.  Merge owns the
// semantic ascent and provenance conjunction; callers do not get a callback,
// identity issuer, or a second row representation through this seam.
//
// Every input carries authenticated mount-issued lineage. Even equal
// references are passed through Mounted.Lineage's authority so a copied or
// foreign reference cannot bypass the mounted proof namespace.
func Merge(mounted witness.Mounted, alternatives []Tuple, declaredKeyColumns ...[]model.ColumnID) (Tuple, bool) {
	if !mounted.Available() || len(alternatives) == 0 {
		return Tuple{}, false
	}
	if len(declaredKeyColumns) > 1 {
		return Tuple{}, false
	}
	keyColumns := map[model.ColumnID]struct{}{}
	if len(declaredKeyColumns) == 1 {
		for _, column := range declaredKeyColumns[0] {
			if !column.Available() {
				return Tuple{}, false
			}
			keyColumns[column] = struct{}{}
		}
	}

	lineageAuthority, ok := mounted.Lineage()
	if !ok || lineageAuthority == nil {
		return Tuple{}, false
	}

	first := alternatives[0]
	if !validMergeInput(mounted, first) || !first.lineage.Available() {
		return Tuple{}, false
	}
	if !lineageAuthority.Validate(first.lineage) {
		return Tuple{}, false
	}

	// Alternatives are one range, not a collection of extents.  Retaining the
	// first scope and source vector is therefore safe only after every input is
	// checked against it byte-for-byte.
	scope := first.scope
	sources := append([]model.RowID(nil), first.sources...)
	cells := append([]Cell(nil), first.cells...)
	lineage := first.lineage
	for _, alternative := range alternatives[1:] {
		if !validMergeInput(mounted, alternative) || !alternative.lineage.Available() {
			return Tuple{}, false
		}
		if !alternative.scope.Same(scope) || !sameSources(alternative.sources, sources) || !sameCellShape(alternative.cells, cells) {
			return Tuple{}, false
		}
		if !lineageAuthority.Validate(alternative.lineage) {
			return Tuple{}, false
		}
		lineage, ok = lineageAuthority.Join(lineage, alternative.lineage)
		if !ok {
			return Tuple{}, false
		}
	}

	// Fold each declared cell in authored order.  Cell presence is a separate
	// closed status: it chooses whether a value participates, but never
	// decides equality or replaces the type-specific value algebra.
	reduced := make([]Cell, len(cells))
	for index, cell := range cells {
		column := make([]Cell, len(alternatives))
		for alternativeIndex, alternative := range alternatives {
			candidate := alternative.cells[index]
			if candidate.Column() != cell.Column() || candidate.Type() != cell.Type() {
				return Tuple{}, false
			}
			column[alternativeIndex] = candidate
		}
		_, isKey := keyColumns[cell.Column()]
		value, ok := reduceCell(mounted, column, isKey)
		if !ok {
			return Tuple{}, false
		}
		reduced[index] = value
	}

	return newTuple(mounted, scope, lineage, sources, reduced)
}

// SameKey compares declared key cells using the mounted TypeID equality. A
// key cell must be present/opaque and carry an authenticated value, but those
// presence labels are not the equality authority: semantic equality is the
// conjunction of the algebra's two directional ordering judgments.
func SameKey(mounted witness.Mounted, left, right Tuple, columns []model.ColumnID) bool {
	if !mounted.Available() || !left.ValidFor(mounted) || !right.ValidFor(mounted) || len(columns) == 0 {
		return false
	}
	for _, column := range columns {
		leftCell, leftOK := left.CellFor(column)
		rightCell, rightOK := right.CellFor(column)
		if !leftOK || !rightOK || !keyCell(leftCell, mounted) || !keyCell(rightCell, mounted) || leftCell.Type() != rightCell.Type() {
			return false
		}
		if !SemanticEqual(mounted, leftCell.Type(), leftCell.Value(), rightCell.Value()) {
			return false
		}
	}
	return true
}

// SemanticEqual compares two authenticated values through the equality
// authority sealed for typeID. ValueToken identity is only an
// authentication/fence check; domains may intentionally equate different
// opaque encodings. Ascending types expose this authority as a projection of
// their already-admitted algebra, while Equatable-only types supply their own
// owner witness.
func SemanticEqual(mounted witness.Mounted, typeID model.TypeID, left, right binding.ValueToken) bool {
	if !mounted.Available() || !typeID.Available() || !left.Available() || !right.Available() || left.Type() != typeID || right.Type() != typeID || !left.ValidFor(mounted.RuntimeFence()) || !right.ValidFor(mounted.RuntimeFence()) {
		return false
	}
	equality, ok := mounted.Equality(typeID)
	if !ok || equality == nil {
		return false
	}
	return equality.Equal(left, right)
}

func validMergeInput(mounted witness.Mounted, value Tuple) bool {
	return value.ValidFor(mounted) && value.Scope().ValidFor(mounted.RuntimeFence()) && value.SourceLen() != 0 && value.Len() != 0
}

func sameSources(left, right []model.RowID) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func sameCellShape(left, right []Cell) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].Column() != right[index].Column() || left[index].Type() != right[index].Type() || left[index].Source() != right[index].Source() {
			return false
		}
	}
	return true
}

func keyCell(cell Cell, mounted witness.Mounted) bool {
	return cell.Column().Available() && cell.Type().Available() && cell.Presence().Available() && (cell.Presence().Is(model.Present) || cell.Presence().Is(model.AuthenticatedOpaque)) && cell.Value().Available() && cell.Value().ValidFor(mounted.RuntimeFence()) && cell.Value().Type() == cell.Type()
}

func reduceCell(mounted witness.Mounted, cells []Cell, key bool) (Cell, bool) {
	if !mounted.Available() || len(cells) == 0 {
		return Cell{}, false
	}
	typeID := cells[0].Type()
	columnID := cells[0].Column()
	if !columnID.Available() || !typeID.Available() {
		return Cell{}, false
	}

	var (
		value      binding.ValueToken
		haveValue  bool
		valueCount int
		anyPresent bool
		anyOpaque  bool
		anyAbsent  bool
		anyMissing bool
	)
	for _, cell := range cells {
		if !cell.available(mounted.RuntimeFence()) || cell.Column() != columnID || cell.Type() != typeID {
			return Cell{}, false
		}
		switch {
		case cell.Presence().Is(model.Present), cell.Presence().Is(model.AuthenticatedOpaque):
			if !cell.Value().Available() || !cell.Value().ValidFor(mounted.RuntimeFence()) || cell.Value().Type() != typeID {
				return Cell{}, false
			}
			if !haveValue {
				value = cell.Value()
				haveValue = true
			} else {
				if key {
					equality, equalityOK := mounted.Equality(typeID)
					if !equalityOK || equality == nil || !equality.Equal(value, cell.Value()) {
						return Cell{}, false
					}
				} else {
					algebra, algebraOK := mounted.Algebra(typeID)
					if !algebraOK || algebra == nil {
						return Cell{}, false
					}
					joined, joinOK := algebra.Join(value, cell.Value())
					if !joinOK || !joined.Available() || !joined.ValidFor(mounted.RuntimeFence()) || joined.Type() != typeID {
						return Cell{}, false
					}
					value = joined
				}
			}
			valueCount++
			if cell.Presence().Is(model.Present) {
				anyPresent = true
			} else {
				anyOpaque = true
			}
		case cell.Presence().Is(model.ProvenAbsent):
			anyAbsent = true
		case cell.Presence().Is(model.UnprovenMissing):
			anyMissing = true
		default:
			return Cell{}, false
		}
	}

	var presence model.Presence
	var ok bool
	switch {
	case anyPresent:
		presence, ok = model.NewPresence(model.Present)
	case anyOpaque && haveValue:
		// More than one opaque alternative has already gone through Join and
		// therefore becomes a semantic result. A single opaque token remains
		// opaque, retaining the input's authenticated status.
		if key || valueCount == 1 {
			presence, ok = model.NewPresence(model.AuthenticatedOpaque)
		} else {
			presence, ok = model.NewPresence(model.Present)
		}
	case haveValue:
		presence, ok = model.NewPresence(model.Present)
	case anyAbsent:
		presence, ok = model.NewPresence(model.ProvenAbsent)
	case anyMissing:
		presence, ok = model.NewPresence(model.UnprovenMissing)
	default:
		return Cell{}, false
	}
	if !ok {
		return Cell{}, false
	}
	if !haveValue {
		value = binding.ValueToken{}
	}
	return Cell{column: columnID, typeID: typeID, value: value, presence: presence, source: cells[0].Source()}, true
}
