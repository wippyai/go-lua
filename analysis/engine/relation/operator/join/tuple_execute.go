package join

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/engine/relation/tuple"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// Join evaluates the tuple-native equijoin over two immutable batches. The
// arrangement binding is the sealed source of correspondence columns; the
// Geometry is the mounted cofiber authority used by tuple.Combine to normalize
// the output scope and join lineage. No state Reader is needed to conjoin
// scopes and no access-shaped reader may be fabricated for that purpose.
//
// Both inputs carry one exact cofiber scope. Their ordered tuples are matched
// in left-then-right order. The returned batch keeps that order and contains
// only tuple.Combine outputs: source RowIDs and cells are concatenated without
// sorting, and no identity is issued or reconstructed here. A contradictory
// input scope or an empty match is a valid empty batch; foreign/malformed
// authority is refused.
func Join(binding arrangement.JoinBinding, mounted witness.Mounted, view geometry.Geometry, left, right tuple.Batch) (tuple.Batch, bool) {
	if !binding.Available() || !mounted.Available() || !view.ValidFor(mounted) || !left.ValidFor(mounted) || !right.ValidFor(mounted) {
		return tuple.Batch{}, false
	}
	leftLayout, rightLayout := binding.Left(), binding.Right()
	if !leftLayout.ValidFor(mounted.Fence()) || !rightLayout.ValidFor(mounted.Fence()) || len(leftLayout.Columns()) == 0 || len(leftLayout.Columns()) != len(rightLayout.Columns()) {
		return tuple.Batch{}, false
	}
	// A self-join needs an explicit role in the sealed correspondence.  This
	// binding carries no role coordinate, so accepting two layouts over one
	// relation would make CellFor/SourceFor choose by position or first match.
	// Refuse it here instead of inventing a second local role protocol.
	if leftLayout.Access().Relation() == rightLayout.Access().Relation() {
		return tuple.Batch{}, false
	}
	leftColumns, rightColumns := leftLayout.Columns(), rightLayout.Columns()

	// The batch scope is already normalized by its owner. A failed conjunction
	// therefore means a contradictory cofiber, not an authority failure. Keep
	// the left scope on the empty result so the empty extent remains a valid
	// batch in the caller's exact fiber.
	joinedScope, scopesOverlap := view.Conjoin(left.Scope(), right.Scope())
	if !scopesOverlap {
		return tuple.PreserveRange(mounted, left, left.Scope(), []tuple.Tuple{})
	}
	if !joinedScope.ValidFor(mounted.RuntimeFence()) {
		return tuple.Batch{}, false
	}

	outputs := make([]tuple.Tuple, 0)
	leftValues, rightValues := left.Tuples(), right.Tuples()
	for _, leftValue := range leftValues {
		if !validTuple(leftValue, mounted, leftColumns) {
			return tuple.Batch{}, false
		}
		for _, rightValue := range rightValues {
			if !validTuple(rightValue, mounted, rightColumns) {
				return tuple.Batch{}, false
			}
			matched, matchOK := exactMatch(mounted, leftValue, rightValue, leftColumns, rightColumns)
			if !matchOK {
				return tuple.Batch{}, false
			}
			if !matched {
				continue
			}
			joined, combineOK := tuple.Combine(mounted, view, leftValue, rightValue)
			if !combineOK || !joined.ValidFor(mounted) || !joined.Scope().Same(joinedScope) {
				return tuple.Batch{}, false
			}
			outputs = append(outputs, joined)
		}
	}
	return tuple.PreserveRange(mounted, left, joinedScope, outputs)
}

func validTuple(value tuple.Tuple, mounted witness.Mounted, columns []model.ColumnID) bool {
	if !value.ValidFor(mounted) || len(columns) == 0 {
		return false
	}
	for _, column := range columns {
		cell, ok := value.CellFor(column)
		if !ok || !cell.Column().Available() || !cell.Type().Available() || !cell.Presence().Available() || (!cell.Presence().Is(model.Present) && !cell.Presence().Is(model.AuthenticatedOpaque)) || !cell.Value().Available() || !cell.Value().ValidFor(mounted.RuntimeFence()) || cell.Value().Type() != cell.Type() {
			return false
		}
	}
	return true
}

func exactMatch(mounted witness.Mounted, left, right tuple.Tuple, leftColumns, rightColumns []model.ColumnID) (bool, bool) {
	if len(leftColumns) == 0 || len(leftColumns) != len(rightColumns) {
		return false, false
	}
	for index, leftColumn := range leftColumns {
		leftCell, leftOK := left.CellFor(leftColumn)
		rightCell, rightOK := right.CellFor(rightColumns[index])
		if !leftOK || !rightOK || leftCell.Type() != rightCell.Type() || !leftCell.Value().Available() || !rightCell.Value().Available() {
			return false, false
		}
		// Token identity authenticates a mounted value but is not semantic
		// equality. The axis-owned algebra is the sole equality authority; the
		// engine must never inspect concrete payloads or compare token hashes.
		if !tuple.SemanticEqual(mounted, leftCell.Type(), leftCell.Value(), rightCell.Value()) {
			return false, true
		}
	}
	return true, true
}
