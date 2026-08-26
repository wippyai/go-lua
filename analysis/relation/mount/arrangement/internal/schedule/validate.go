package schedule

import "github.com/wippyai/go-lua/analysis/relation/schema/model"

// valid is the cold proof retained by Schedule.Available. It verifies the
// table/index correspondence once at construction; hot consumers perform
// only O(1) sealed lookups.
func valid(data *data) bool {
	if data == nil || !data.sealed || data.entries == nil || data.byID == nil || data.byRelation == nil || data.byColumn == nil || data.components == nil || len(data.entries) != len(data.byID) {
		return false
	}
	for index, entry := range data.entries {
		if !entry.Available() || data.byID[entry.id] != index || entry.component >= uint32(len(data.components)) {
			return false
		}
		component := data.components[entry.component]
		if !component.Available() || !containsDependency(component.members, entry.id) {
			return false
		}
	}
	if !validRelationWakes(data.byRelation, data.entries) || !validColumnWakes(data.byColumn, data.entries) {
		return false
	}
	for index, component := range data.components {
		if !component.Available() || component.order != uint32(index) {
			return false
		}
	}
	return true
}

func containsDependency(values []model.DependencyID, wanted model.DependencyID) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func validRelationWakes(wakes map[model.RelationID][]int, entries []Entry) bool {
	for relation, indices := range wakes {
		if !relation.Available() || indices == nil {
			return false
		}
		last := -1
		for _, index := range indices {
			if index <= last || index < 0 || index >= len(entries) || !containsRelation(entries[index].reads, relation) {
				return false
			}
			last = index
		}
	}
	return true
}

func validColumnWakes(wakes map[model.ColumnID][]int, entries []Entry) bool {
	for column, indices := range wakes {
		if !column.Available() || indices == nil {
			return false
		}
		last := -1
		for _, index := range indices {
			if index <= last || index < 0 || index >= len(entries) || !containsColumn(entries[index].columns, column) {
				return false
			}
			last = index
		}
	}
	return true
}

func containsColumn(values []model.ColumnID, wanted model.ColumnID) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
