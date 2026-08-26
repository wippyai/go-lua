package arrangement

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// scheduleColumns walks the already-mounted physical tree once during
// Derive. It never inspects a logical expression or chooses an access path.
// The certificate read footprint fences the collected vectors, preventing
// output layouts from becoming false dependency wakes.
func scheduleColumns(root Node, allowed []model.RelationID) ([]model.ColumnID, bool) {
	if !root.Available() || allowed == nil {
		return nil, false
	}
	allowedSet := make(map[model.RelationID]struct{}, len(allowed))
	for _, relation := range allowed {
		if !relation.Available() {
			return nil, false
		}
		allowedSet[relation] = struct{}{}
	}
	values := make([]model.ColumnID, 0)
	seenNodes := make(map[identity.ContentID]struct{})
	add := func(layout Layout) bool {
		if !layout.Available() {
			return false
		}
		for _, column := range append(layout.Columns(), layout.KeyColumns()...) {
			if !column.Available() {
				return false
			}
			if _, include := allowedSet[column.Relation()]; include {
				values = append(values, column)
			}
		}
		return true
	}
	var walk func(Node) bool
	walk = func(node Node) bool {
		if !node.Available() {
			return false
		}
		if _, done := seenNodes[node.Digest()]; done {
			return true
		}
		seenNodes[node.Digest()] = struct{}{}
		for _, child := range node.Children() {
			if !walk(child) {
				return false
			}
		}
		if input, ok := node.Input(); ok {
			return add(input.Values())
		}
		if project, ok := node.Project(); ok {
			if !add(project.Target()) || !add(project.Key()) {
				return false
			}
			for _, mapping := range project.Mappings() {
				if !add(mapping.Layout()) {
					return false
				}
			}
		}
		if join, ok := node.Join(); ok && (!add(join.Left()) || !add(join.Right())) {
			return false
		}
		if merge, ok := node.Merge(); ok && !add(merge.Key()) {
			return false
		}
		if expand, ok := node.Expand(); ok {
			// Expand's R vector is a runtime read and must be an exact
			// dependency wake.  C is already collected by the child Input
			// walk above; P remains cold evidence and has no runtime layout.
			if !add(expand.Reader()) {
				return false
			}
		}
		if group, ok := node.Group(); ok && !add(group.Key()) {
			return false
		}
		if complete, ok := node.Complete(); ok && !add(complete.Key()) {
			return false
		}
		if apply, ok := node.Apply(); ok {
			for _, delivery := range apply.Deliveries() {
				if !add(delivery.Layout()) {
					return false
				}
				if order, ordered := delivery.Order(); ordered && !add(order) {
					return false
				}
			}
			if replay, replayOK := apply.Replay(); replayOK {
				// All child reads are represented by exact Input/Complete
				// extents below the mounted children already walked above.  The
				// replay header contributes only its population driver vector;
				// a partition directory is evidence, never an extra wake.
				if driver, driverOK := replay.Driver(); driverOK && !add(driver) {
					return false
				}
			}
		}
		if publish, ok := node.Publish(); ok && (!add(publish.Destination()) || !add(publish.Key())) {
			return false
		}
		return true
	}
	if !walk(root) {
		return nil, false
	}
	return canonicalScheduleColumns(values)
}

// canonicalScheduleColumns turns the physical-tree collection into the one
// stable wake set. A column can occur through an Input vector, a delivery,
// and a key layout in the same expression; those are multiple observations
// of one dependency, never multiple wakes. Sorting here also makes the mount
// schedule digest independent of harmless child traversal order.
func canonicalScheduleColumns(values []model.ColumnID) ([]model.ColumnID, bool) {
	result := make([]model.ColumnID, 0, len(values))
	seen := make(map[model.ColumnID]struct{}, len(values))
	for _, value := range values {
		if !value.Available() {
			return nil, false
		}
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Slice(result, func(left, right int) bool {
		return compareColumn(result[left], result[right]) < 0
	})
	return result, true
}
