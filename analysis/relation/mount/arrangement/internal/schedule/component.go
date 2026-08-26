package schedule

import (
	"github.com/wippyai/go-lua/analysis/relation/check/certificate"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

type componentBuild struct {
	members    []model.DependencyID
	edges      []Edge
	recurrence RecurrenceKind
	heads      []Head
	key        string
}

// buildComponents redeems the certificate's already-proved SCC projection.
// It may order independent components deterministically, but it never
// discovers cycles or recurrence by rewalking a logical program.
func buildComponents(recurrence certificate.RecurrenceData, dependencies map[model.DependencyID]projection) ([]Component, map[model.DependencyID]uint32, bool) {
	declarations := recurrence.Components()
	components := make([]componentBuild, 0, len(declarations))
	componentByKey := make(map[string]int, len(declarations))
	componentByID := make(map[model.DependencyID]int, len(dependencies))
	for _, declaration := range declarations {
		members, membersOK := canonicalDependencies(declaration.Members())
		if !membersOK || len(members) == 0 {
			return nil, nil, false
		}
		key := componentKey(members)
		if _, duplicate := componentByKey[key]; duplicate {
			return nil, nil, false
		}
		kind := RecurrenceAcyclic
		if declaration.Cyclic() {
			kind = RecurrencePositive
		}
		for _, member := range members {
			if _, dependencyOK := dependencies[member]; !dependencyOK {
				return nil, nil, false
			}
			if _, duplicate := componentByID[member]; duplicate {
				return nil, nil, false
			}
			componentByID[member] = len(components)
		}
		edges, edgesOK := componentEdges(declaration.Edges(), members)
		if !edgesOK {
			return nil, nil, false
		}
		heads, headsOK := componentHeads(recurrence.WideningHeads(), members)
		if !headsOK || kind == RecurrenceAcyclic && len(heads) != 0 {
			return nil, nil, false
		}
		components = append(components, componentBuild{members: members, edges: edges, recurrence: kind, heads: heads, key: key})
		componentByKey[key] = len(components) - 1
	}
	if len(componentByID) != len(dependencies) || len(components) != len(declarations) {
		return nil, nil, false
	}

	// Topologically order the certificate-owned component graph. Edges inside
	// a component are retained on that component; only cross-component edges
	// affect the solve order.
	componentEdgesSet := make(map[[2]int]struct{})
	indegree := make([]int, len(components))
	for _, edge := range recurrence.Edges() {
		from, to := edge.From(), edge.To()
		fromComponent, fromOK := componentByID[from]
		toComponent, toOK := componentByID[to]
		if !fromOK || !toOK {
			return nil, nil, false
		}
		if fromComponent == toComponent {
			continue
		}
		key := [2]int{fromComponent, toComponent}
		if _, duplicate := componentEdgesSet[key]; duplicate {
			continue
		}
		componentEdgesSet[key] = struct{}{}
		indegree[toComponent]++
	}
	ready := make([]int, 0, len(components))
	for index, degree := range indegree {
		if degree == 0 {
			ready = append(ready, index)
		}
	}
	ordered := make([]int, 0, len(components))
	for len(ready) != 0 {
		sortComponentIndices(ready, components)
		current := ready[0]
		ready = ready[1:]
		ordered = append(ordered, current)
		for edge := range componentEdgesSet {
			if edge[0] != current {
				continue
			}
			indegree[edge[1]]--
			if indegree[edge[1]] == 0 {
				ready = append(ready, edge[1])
			}
		}
	}
	if len(ordered) != len(components) {
		return nil, nil, false
	}
	remap := make(map[int]uint32, len(ordered))
	result := make([]Component, len(ordered))
	for order, old := range ordered {
		remap[old] = uint32(order)
		value := components[old]
		result[order] = Component{order: uint32(order), members: append([]model.DependencyID{}, value.members...), edges: append([]Edge{}, value.edges...), recurrence: value.recurrence, heads: append([]Head{}, value.heads...)}
		if !result[order].Available() {
			return nil, nil, false
		}
	}
	resultByID := make(map[model.DependencyID]uint32, len(componentByID))
	for dependency, old := range componentByID {
		resultByID[dependency] = remap[old]
	}
	return result, resultByID, true
}

func sortComponentIndices(values []int, components []componentBuild) {
	for index := 1; index < len(values); index++ {
		for cursor := index; cursor > 0 && components[values[cursor]].key < components[values[cursor-1]].key; cursor-- {
			values[cursor], values[cursor-1] = values[cursor-1], values[cursor]
		}
	}
}

func componentEdges(values []certificate.RecurrenceEdge, members []model.DependencyID) ([]Edge, bool) {
	memberSet := make(map[model.DependencyID]struct{}, len(members))
	for _, member := range members {
		memberSet[member] = struct{}{}
	}
	result := make([]Edge, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		from, to := value.From(), value.To()
		if !from.Available() || !to.Available() {
			return nil, false
		}
		if _, found := memberSet[from]; !found {
			return nil, false
		}
		if _, found := memberSet[to]; !found {
			return nil, false
		}
		key := dependencyKey(from) + dependencyKey(to)
		if _, duplicate := seen[key]; duplicate {
			return nil, false
		}
		seen[key] = struct{}{}
		result = append(result, Edge{from: from, to: to})
	}
	sortEdges(result)
	return result, true
}

// componentHeads filters the global certificate head set to this component.
// The certificate stores heads once for the whole recurrence projection; an
// independent component with no head is valid and must not be rejected just
// because a sibling component owns one.
func componentHeads(values []certificate.RecurrenceHead, members []model.DependencyID) ([]Head, bool) {
	memberSet := make(map[model.DependencyID]struct{}, len(members))
	for _, member := range members {
		memberSet[member] = struct{}{}
	}
	result := make([]Head, 0)
	seen := make(map[string]struct{})
	for _, value := range values {
		dependency, relation := value.Dependency(), value.Relation()
		if !dependency.Available() || !relation.Available() {
			return nil, false
		}
		if _, member := memberSet[dependency]; !member {
			continue
		}
		key := dependencyKey(dependency) + relationKey(relation)
		if _, duplicate := seen[key]; duplicate {
			return nil, false
		}
		seen[key] = struct{}{}
		result = append(result, Head{dependency: dependency, relation: relation})
	}
	sortHeads(result)
	return result, true
}

func sortEdges(values []Edge) {
	for index := 1; index < len(values); index++ {
		for cursor := index; cursor > 0 && edgeLess(values[cursor], values[cursor-1]); cursor-- {
			values[cursor], values[cursor-1] = values[cursor-1], values[cursor]
		}
	}
}

func edgeLess(left, right Edge) bool {
	if compared := compareDependency(left.from, right.from); compared != 0 {
		return compared < 0
	}
	return dependencyLess(left.to, right.to)
}

func sortHeads(values []Head) {
	for index := 1; index < len(values); index++ {
		for cursor := index; cursor > 0 && headLess(values[cursor], values[cursor-1]); cursor-- {
			values[cursor], values[cursor-1] = values[cursor-1], values[cursor]
		}
	}
}

func headLess(left, right Head) bool {
	if compared := compareDependency(left.dependency, right.dependency); compared != 0 {
		return compared < 0
	}
	return relationLess(left.relation, right.relation)
}
