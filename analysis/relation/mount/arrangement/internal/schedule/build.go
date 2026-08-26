package schedule

import (
	"github.com/wippyai/go-lua/analysis/relation/check/certificate"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// Build lowers the certificate recurrence projection plus arrangement-issued
// physical evidence into one immutable schedule. It is mount-time work: no
// evaluator or solver can call this path.
func Build(recurrence certificate.RecurrenceData, bindings []Binding) (Schedule, bool) {
	if !recurrence.Available() || bindings == nil {
		return Schedule{}, false
	}
	projections, bindingsByID, ok := bindProjections(recurrence.Projections(), bindings)
	if !ok {
		return Schedule{}, false
	}
	components, componentByID, ok := buildComponents(recurrence, projections)
	if !ok {
		return Schedule{}, false
	}
	order, ok := dependencyOrder(components, projections)
	if !ok {
		return Schedule{}, false
	}
	heads, ok := headsByDependency(recurrence.WideningHeads(), projections)
	if !ok {
		return Schedule{}, false
	}

	entries := make([]Entry, 0, len(order))
	for _, dependency := range order {
		projection, projectionOK := projections[dependency]
		binding, bindingOK := bindingsByID[dependency]
		component, componentOK := componentByID[dependency]
		if !projectionOK || !bindingOK || !componentOK {
			return Schedule{}, false
		}
		reads := relationReads(projection.reads, binding.columns)
		entry := Entry{
			id:        dependency,
			root:      projection.expression,
			node:      binding.node,
			reads:     reads,
			columns:   append([]model.ColumnID{}, binding.columns...),
			writes:    append([]model.RelationID{}, projection.writes...),
			heads:     append([]model.RelationID{}, heads[dependency]...),
			component: component,
		}
		if !entry.Available() || !headsWrite(entry, projection.writes) {
			return Schedule{}, false
		}
		entries = append(entries, entry)
	}

	data := &data{
		entries:    entries,
		byID:       make(map[model.DependencyID]int, len(entries)),
		byRelation: make(map[model.RelationID][]int),
		byColumn:   make(map[model.ColumnID][]int),
		components: components,
	}
	for index, entry := range entries {
		if _, duplicate := data.byID[entry.id]; duplicate {
			return Schedule{}, false
		}
		data.byID[entry.id] = index
		for _, relation := range entry.reads {
			data.byRelation[relation] = appendWakeIndex(data.byRelation[relation], index)
		}
		for _, column := range entry.columns {
			data.byColumn[column] = appendWakeIndex(data.byColumn[column], index)
		}
	}
	logical, physical, digestOK := digests(data)
	if !digestOK {
		return Schedule{}, false
	}
	data.logical, data.digest, data.sealed = logical, physical, true
	if !valid(data) {
		return Schedule{}, false
	}
	return Schedule{data: data}, true
}

type projection struct {
	dependency model.DependencyID
	expression model.ExpressionID
	reads      []model.RelationID
	writes     []model.RelationID
}

// bindProjections cross-checks arrangement's physical binding against the
// certificate-issued relation footprint. The cold schedule never chooses a
// new read; it only records the already-selected exact column wake set.
func bindProjections(values []certificate.RecurrenceProjection, bindings []Binding) (map[model.DependencyID]projection, map[model.DependencyID]Binding, bool) {
	if len(values) != len(bindings) {
		return nil, nil, false
	}
	bindingsByID := make(map[model.DependencyID]Binding, len(bindings))
	for _, binding := range bindings {
		if !binding.Available() {
			return nil, nil, false
		}
		if _, duplicate := bindingsByID[binding.dependency]; duplicate {
			return nil, nil, false
		}
		bindingsByID[binding.dependency] = binding
	}
	result := make(map[model.DependencyID]projection, len(values))
	for _, value := range values {
		dependency, expression := value.Dependency(), value.Expression()
		if !dependency.Available() || !expression.Available() {
			return nil, nil, false
		}
		if _, duplicate := result[dependency]; duplicate {
			return nil, nil, false
		}
		reads, readsOK := canonicalRelations(value.Reads())
		writes, writesOK := canonicalRelations(value.Writes())
		binding, bindingOK := bindingsByID[dependency]
		if !readsOK || !writesOK || !bindingOK || binding.expression != expression {
			return nil, nil, false
		}
		boundReads, boundReadsOK := canonicalRelations(binding.reads)
		boundWrites, boundWritesOK := canonicalRelations(binding.writes)
		if !boundReadsOK || !boundWritesOK || !sameRelations(boundReads, reads) || !sameRelations(boundWrites, writes) {
			return nil, nil, false
		}
		columns, columnsOK := canonicalColumns(binding.columns)
		if !columnsOK {
			return nil, nil, false
		}
		for _, column := range columns {
			if !containsRelation(reads, column.Relation()) {
				return nil, nil, false
			}
		}
		binding.reads = append([]model.RelationID{}, reads...)
		binding.columns = columns
		binding.writes = append([]model.RelationID{}, writes...)
		bindingsByID[dependency] = binding
		result[dependency] = projection{dependency: dependency, expression: expression, reads: reads, writes: writes}
	}
	return result, bindingsByID, len(result) == len(bindingsByID)
}

func sameRelations(left, right []model.RelationID) bool {
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

func appendWakeIndex(values []int, index int) []int {
	if len(values) == 0 || values[len(values)-1] != index {
		return append(values, index)
	}
	return values
}

func relationReads(reads []model.RelationID, columns []model.ColumnID) []model.RelationID {
	covered := make(map[model.RelationID]struct{}, len(columns))
	for _, column := range columns {
		covered[column.Relation()] = struct{}{}
	}
	result := make([]model.RelationID, 0, len(reads))
	for _, relation := range reads {
		if _, exact := covered[relation]; !exact {
			result = append(result, relation)
		}
	}
	return result
}

func headsWrite(entry Entry, writes []model.RelationID) bool {
	for _, relation := range entry.heads {
		if !containsRelation(writes, relation) {
			return false
		}
	}
	return true
}

func containsRelation(values []model.RelationID, wanted model.RelationID) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func dependencyOrder(components []Component, projections map[model.DependencyID]projection) ([]model.DependencyID, bool) {
	result := make([]model.DependencyID, 0, len(projections))
	seen := make(map[model.DependencyID]struct{}, len(projections))
	for _, component := range components {
		for _, dependency := range component.members {
			if _, exists := projections[dependency]; !exists {
				return nil, false
			}
			if _, duplicate := seen[dependency]; duplicate {
				return nil, false
			}
			seen[dependency] = struct{}{}
			result = append(result, dependency)
		}
	}
	return result, len(result) == len(projections)
}

func headsByDependency(values []certificate.RecurrenceHead, projections map[model.DependencyID]projection) (map[model.DependencyID][]model.RelationID, bool) {
	result := make(map[model.DependencyID][]model.RelationID, len(projections))
	for dependency := range projections {
		result[dependency] = []model.RelationID{}
	}
	for _, value := range values {
		dependency, relation := value.Dependency(), value.Relation()
		projection, found := projections[dependency]
		if !found || !relation.Available() || !containsRelation(projection.writes, relation) {
			return nil, false
		}
		for _, prior := range result[dependency] {
			if prior == relation {
				return nil, false
			}
		}
		result[dependency] = append(result[dependency], relation)
	}
	for dependency, relations := range result {
		sortRelations(relations)
		result[dependency] = relations
	}
	return result, true
}
