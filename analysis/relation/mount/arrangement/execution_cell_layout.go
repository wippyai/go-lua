package arrangement

import (
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// executionCellLayout redeems a mounted node's one canonical physical output
// coordinate. It runs only while Derive seals a node (and while cold
// availability validation verifies that seal); evaluators never invoke it.
//
// The definition intentionally follows tuple construction, not logical
// relation names: Join/Expand append rows, Complete appends only its missing
// denominator cells, Project introduces its mapped target row explicitly,
// and ColumnProject retains addressed child cells.
func executionCellLayout(node *executionNode) (algebra.CellLayout, bool) {
	if node == nil {
		return algebra.CellLayout{}, false
	}
	child := func(index int) (algebra.CellLayout, bool) {
		if index < 0 || index >= len(node.children) || node.children[index] == nil || !node.children[index].cells.Available() {
			return algebra.CellLayout{}, false
		}
		return node.children[index].cells, true
	}
	oneChild := func() (algebra.CellLayout, bool) {
		if len(node.children) != 1 {
			return algebra.CellLayout{}, false
		}
		return child(0)
	}

	switch node.kind {
	case algebra.KindInput:
		if !node.input.Available() {
			return algebra.CellLayout{}, false
		}
		return algebra.InputCellLayout(node.input.relation, node.input.values.Columns())
	case algebra.KindSelect, algebra.KindGroup, algebra.KindPublish:
		return oneChild()
	case algebra.KindComplete:
		value, ok := oneChild()
		if !ok || !node.complete.Available() {
			return algebra.CellLayout{}, false
		}
		return algebra.CompleteCellLayout(value, node.complete.Denominator(), node.complete.Columns())
	case algebra.KindJoin:
		left, leftOK := child(0)
		right, rightOK := child(1)
		if !leftOK || !rightOK || len(node.children) != 2 || !node.join.Available() {
			return algebra.CellLayout{}, false
		}
		return algebra.JoinCellLayouts(left, right)
	case algebra.KindExpand:
		left, leftOK := oneChild()
		if !leftOK || !node.expand.Available() {
			return algebra.CellLayout{}, false
		}
		right, rightOK := algebra.InputCellLayout(node.expand.reader.Access().Relation(), node.expand.reader.Columns())
		if !rightOK {
			return algebra.CellLayout{}, false
		}
		return algebra.JoinCellLayouts(left, right)
	case algebra.KindColumnProject:
		value, ok := oneChild()
		if !ok || !node.columnProject.Available() {
			return algebra.CellLayout{}, false
		}
		slots := make([]algebra.ColumnSlot, node.columnProject.SlotCount())
		for index := range slots {
			slot, slotOK := node.columnProject.SlotAt(index)
			if !slotOK {
				return algebra.CellLayout{}, false
			}
			slots[index] = slot
		}
		return algebra.ColumnProjectCellLayout(value, slots)
	case algebra.KindProject:
		value, ok := oneChild()
		if !ok || !node.project.Available() || !node.project.target.Available() {
			return algebra.CellLayout{}, false
		}
		mappings := node.project.Mappings()
		contract := make([]algebra.ColumnMapping, len(mappings))
		for index, mapping := range mappings {
			if !mapping.Available() {
				return algebra.CellLayout{}, false
			}
			contract[index] = algebra.NewColumnMapping(mapping.Source(), mapping.Target())
		}
		return algebra.ProjectCellLayout(value, node.project.target.Access().Relation(), contract)
	case algebra.KindMerge:
		if len(node.children) == 0 || !node.merge.Available() {
			return algebra.CellLayout{}, false
		}
		result, ok := child(0)
		if !ok {
			return algebra.CellLayout{}, false
		}
		for index := 1; index < len(node.children); index++ {
			candidate, candidateOK := child(index)
			if !candidateOK || !result.Equal(candidate) {
				return algebra.CellLayout{}, false
			}
		}
		return result, true
	case algebra.KindApply:
		if !node.apply.Available() || node.apply.ChildCount() != len(node.children) {
			return algebra.CellLayout{}, false
		}
		return node.apply.OutputCells(), node.apply.OutputCells().Available()
	default:
		return algebra.CellLayout{}, false
	}
}

// applyOutputCellLayout turns one sealed semantic signature output row into
// Apply's physical result coordinate. The signature does not get consulted by
// evaluators; bindApply calls this once at mount and stores the result in the
// ApplyBinding.
func applyOutputCellLayout(outputs []model.ColumnID, relation model.RelationID) (algebra.CellLayout, bool) {
	return algebra.InputCellLayout(relation, outputs)
}
