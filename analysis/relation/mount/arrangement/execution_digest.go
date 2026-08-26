package arrangement

import (
	"github.com/wippyai/go-lua/analysis/relation/mount/address"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// validateExecution is the cold proof performed exactly once by Derive. It
// intentionally does the full recursive layout/DAG walk before the immutable
// artifact is sealed; Execution.Available must never repeat this work.
func validateExecution(data *executionData) bool {
	if data == nil || !data.fence.Available() || !data.logicalDigest.Available() || !data.digest.Available() || data.entries == nil || data.byID == nil || data.byNode == nil || data.byLogical == nil || len(data.entries) != len(data.byID) || !data.dependencies.Available() {
		return false
	}
	seen := make(map[*executionNode]bool)
	for index, entry := range data.entries {
		if !entry.id.Available() || !entry.digest.Available() || entry.root == nil || !entry.derivation.Available() || entry.derivation.Root() != entry.id || data.byID[entry.id] != index || !executionNodeAvailable(entry.root, data.fence, seen) {
			return false
		}
		if data.byNode[entry.root.digest] != entry.root {
			return false
		}
	}
	for logical, node := range data.byLogical {
		if node == nil || !node.logical.Available() || node.logical != logical || data.byNode[node.digest] != node {
			return false
		}
	}
	for _, component := range data.dependencies.Components() {
		for _, dependencyID := range component.Members() {
			record, recordOK := data.dependencies.Dependency(dependencyID)
			if !recordOK || !record.Available() {
				return false
			}
			entryIndex, entryOK := data.byID[record.Root()]
			if !entryOK || entryIndex < 0 || entryIndex >= len(data.entries) || data.entries[entryIndex].root == nil || !record.Node().Available() || data.entries[entryIndex].root.digest != record.Node().Digest() {
				return false
			}
		}
	}
	return true
}

func executionNodeAvailable(node *executionNode, fence address.Fence, seen map[*executionNode]bool) bool {
	if node == nil || !node.digest.Available() || !node.cells.Available() || !node.cells.Digest().Available() {
		return false
	}
	if seen[node] {
		return true
	}
	seen[node] = true
	validChildren := func(want int) bool {
		if len(node.children) != want {
			return false
		}
		for _, child := range node.children {
			if !executionNodeAvailable(child, fence, seen) {
				return false
			}
		}
		return true
	}
	expectedCells, cellsOK := executionCellLayout(node)
	if !cellsOK || !expectedCells.Equal(node.cells) {
		return false
	}
	validLayout := func(layout Layout) bool { return layout.Available() && layout.ValidFor(fence) }
	switch node.kind {
	case algebra.KindInput:
		return validChildren(0) && node.input.Available() && validLayout(node.input.scan) && validLayout(node.input.values)
	case algebra.KindSelect:
		return validChildren(1) && node.select_.ValidFor(fence)
	case algebra.KindProject:
		if !validChildren(1) || !node.project.Available() || !validLayout(node.project.target) || !validLayout(node.project.key) {
			return false
		}
		for _, mapping := range node.project.mappings {
			if !validLayout(mapping.layout) {
				return false
			}
		}
		return true
	case algebra.KindColumnProject:
		return validChildren(1) && node.columnProject.Available() && validLayout(node.columnProject.values)
	case algebra.KindExpand:
		return validChildren(1) && node.expand.ValidFor(fence) && validLayout(node.expand.candidate) && validLayout(node.expand.reader) && validLayout(node.expand.key)
	case algebra.KindJoin:
		return validChildren(2) && node.join.Available() && validLayout(node.join.left) && validLayout(node.join.right)
	case algebra.KindMerge:
		return len(node.children) != 0 && node.merge.Available() && validLayout(node.merge.key) && validChildren(len(node.children))
	case algebra.KindGroup:
		return validChildren(1) && node.group.Available() && validLayout(node.group.key)
	case algebra.KindComplete:
		return validChildren(1) && node.complete.Available() && validLayout(node.complete.key)
	case algebra.KindApply:
		if !node.apply.Available() || !validChildren(len(node.children)) {
			return false
		}
		if len(node.children) == 0 {
			return len(node.apply.deliveries) == 0 && len(node.apply.slotSource) == 0 && node.apply.childCount == 0 && node.apply.output.IsOwnerNamed()
		}
		for _, delivery := range node.apply.deliveries {
			if !validLayout(delivery.layout) || delivery.requirement.Delivery().IsSpan() && !validLayout(delivery.order) {
				return false
			}
		}
		if node.apply.correlation.Specified() {
			replay, replayOK := node.apply.Replay()
			driver, driverOK := replay.Driver()
			if !replayOK || !driverOK || !validLayout(driver) || driver.CoordinateClass() != CoordinateClassNone || driver.Access().Key().Available() || replay.ChildCount() != len(node.children) || replay.Population() != node.apply.correlation.Population() || driver.Access().Relation() != replay.Population().Relation() || !containsColumn(driver.Columns(), replay.Correlation().Coordinate()) {
				return false
			}
			for index, child := range node.children {
				subtree, subtreeOK := replay.ChildAt(index)
				root := subtree.Root()
				if !subtreeOK || child == nil || !root.Available() || root.value != child {
					return false
				}
			}
		}
		return true
	case algebra.KindPublish:
		return validChildren(1) && node.publish.Available() && validLayout(node.publish.destination) && validLayout(node.publish.key) && validLayout(node.publish.columns)
	default:
		return false
	}
}

func executionLayouts(node executionNode) []Layout {
	result := make([]Layout, 0, 4)
	appendLayout := func(layout Layout) {
		if layout.Available() {
			result = append(result, layout)
		}
	}
	switch node.kind {
	case algebra.KindInput:
		appendLayout(node.input.scan)
		appendLayout(node.input.values)
	case algebra.KindProject:
		appendLayout(node.project.target)
		appendLayout(node.project.key)
		for _, mapping := range node.project.mappings {
			appendLayout(mapping.layout)
		}
	case algebra.KindColumnProject:
		appendLayout(node.columnProject.values)
	case algebra.KindExpand:
		appendLayout(node.expand.candidate)
		appendLayout(node.expand.reader)
		appendLayout(node.expand.key)
	case algebra.KindJoin:
		appendLayout(node.join.left)
		appendLayout(node.join.right)
	case algebra.KindMerge:
		appendLayout(node.merge.key)
	case algebra.KindGroup:
		appendLayout(node.group.key)
	case algebra.KindComplete:
		appendLayout(node.complete.key)
	case algebra.KindApply:
		for _, delivery := range node.apply.deliveries {
			appendLayout(delivery.layout)
			appendLayout(delivery.order)
		}
		if replay, replayOK := node.apply.Replay(); replayOK {
			if driver, driverOK := replay.Driver(); driverOK {
				appendLayout(driver)
			}
		}
	case algebra.KindPublish:
		appendLayout(node.publish.destination)
		appendLayout(node.publish.key)
		appendLayout(node.publish.columns)
	}
	return result
}

// executionColumns reports physical row vectors which are not themselves
// layouts. Complete is the only current node with such a vector: the exact
// relation contract is sealed at mount so the runtime operator never scans
// mounted columns to rediscover it.
func executionColumns(node executionNode) []model.ColumnID {
	if node.kind == algebra.KindComplete && node.complete.Available() {
		return append([]model.ColumnID(nil), node.complete.columns...)
	}
	if node.kind == algebra.KindExpand && node.expand.Available() {
		return append([]model.ColumnID(nil), node.expand.columns...)
	}
	return nil
}

func compareExpression(left, right model.ExpressionID) int {
	return compareNominal(left.Owner().Content(), left.Content(), right.Owner().Content(), right.Content())
}
