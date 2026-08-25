package relcompile

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
)

// footprint is one lowered dependency's relational footprint: the relations it
// reads and the relations it publishes to.
type footprint struct {
	id     model.DependencyID
	reads  []model.RelationID
	writes []model.RelationID
}

// declareComponents derives the dependency graph's strongly connected
// components from the lowered footprints and declares one component per
// vertex set.
//
// The graph is the one the schema already states: a dependency that publishes
// a relation another dependency reads is that reader's producer, and a
// dependency that reads a relation it publishes itself is its own. Recurrence
// is therefore a property of the plan and not a further declaration - a
// component with a cycle through it recurs, and one without it does not - so
// the compiler states it rather than asking an author to restate what the
// footprints already say.
//
// No widening head is declared. Widening is a policy attached to a certified
// head, and a head nobody asked for would be the compiler choosing where a
// lattice stops ascending.
func declareComponents(footprints []footprint) []plan.SCC {
	edges := deriveEdges(footprints)
	components := stronglyConnected(footprints, edges)
	declared := make([]plan.SCC, 0, len(components))
	for _, component := range components {
		members := make([]plan.DependencyRef, 0, len(component))
		memberSet := make(map[model.DependencyID]struct{}, len(component))
		for _, id := range component {
			members = append(members, plan.DefineDependencyRef(id))
			memberSet[id] = struct{}{}
		}
		internal := make([]plan.DependencyEdge, 0)
		cyclic := len(component) > 1
		for _, edge := range edges {
			from, to := edge.From().ID(), edge.To().ID()
			_, fromMember := memberSet[from]
			_, toMember := memberSet[to]
			if !fromMember || !toMember {
				continue
			}
			internal = append(internal, edge)
			cyclic = cyclic || from == to
		}
		kind := plan.Acyclic
		if cyclic {
			kind = plan.Positive
		}
		declared = append(declared, plan.DefineSCC(members, internal, plan.DefineRecurrence(kind, nil)))
	}
	return declared
}

// deriveEdges states the producer relation: one edge from the dependency that
// publishes a relation to every dependency that reads it, itself included.
func deriveEdges(footprints []footprint) []plan.DependencyEdge {
	edges := make([]plan.DependencyEdge, 0)
	for _, producer := range footprints {
		if len(producer.writes) == 0 {
			continue
		}
		published := make(map[model.RelationID]struct{}, len(producer.writes))
		for _, relation := range producer.writes {
			published[relation] = struct{}{}
		}
		for _, consumer := range footprints {
			for _, relation := range consumer.reads {
				if _, ok := published[relation]; !ok {
					continue
				}
				edges = append(edges, plan.DefineDependencyEdge(
					plan.DefineDependencyRef(producer.id),
					plan.DefineDependencyRef(consumer.id),
				))
				break
			}
		}
	}
	return edges
}

// stronglyConnected returns the vertex sets of the graph's components in a
// deterministic order. It is an iterative Kosaraju walk so a deep chain of
// dependencies cannot exhaust the stack.
func stronglyConnected(footprints []footprint, edges []plan.DependencyEdge) [][]model.DependencyID {
	ids := make([]model.DependencyID, 0, len(footprints))
	forward := make(map[model.DependencyID][]model.DependencyID, len(footprints))
	backward := make(map[model.DependencyID][]model.DependencyID, len(footprints))
	for _, value := range footprints {
		ids = append(ids, value.id)
		forward[value.id] = nil
		backward[value.id] = nil
	}
	for _, edge := range edges {
		from, to := edge.From().ID(), edge.To().ID()
		forward[from] = append(forward[from], to)
		backward[to] = append(backward[to], from)
	}
	sortIdentities(ids)
	for id := range forward {
		sortIdentities(forward[id])
	}
	for id := range backward {
		sortIdentities(backward[id])
	}

	visited := make(map[model.DependencyID]bool, len(ids))
	order := make([]model.DependencyID, 0, len(ids))
	for _, start := range ids {
		if visited[start] {
			continue
		}
		visited[start] = true
		stack := []walk{{id: start}}
		for len(stack) != 0 {
			last := len(stack) - 1
			frame := &stack[last]
			if frame.next < len(forward[frame.id]) {
				next := forward[frame.id][frame.next]
				frame.next++
				if !visited[next] {
					visited[next] = true
					stack = append(stack, walk{id: next})
				}
				continue
			}
			order = append(order, frame.id)
			stack = stack[:last]
		}
	}

	assigned := make(map[model.DependencyID]bool, len(ids))
	components := make([][]model.DependencyID, 0, len(ids))
	for index := len(order) - 1; index >= 0; index-- {
		start := order[index]
		if assigned[start] {
			continue
		}
		assigned[start] = true
		members := make([]model.DependencyID, 0, 1)
		stack := []model.DependencyID{start}
		for len(stack) != 0 {
			last := len(stack) - 1
			id := stack[last]
			stack = stack[:last]
			members = append(members, id)
			for _, previous := range backward[id] {
				if assigned[previous] {
					continue
				}
				assigned[previous] = true
				stack = append(stack, previous)
			}
		}
		sortIdentities(members)
		components = append(components, members)
	}
	sort.SliceStable(components, func(left, right int) bool {
		return identityLess(components[left][0], components[right][0])
	})
	return components
}

type walk struct {
	id   model.DependencyID
	next int
}

func sortIdentities(values []model.DependencyID) {
	sort.Slice(values, func(left, right int) bool { return identityLess(values[left], values[right]) })
}

func identityLess(left, right model.DependencyID) bool {
	leftContent, rightContent := left.Content(), right.Content()
	for index := range leftContent {
		if leftContent[index] != rightContent[index] {
			return leftContent[index] < rightContent[index]
		}
	}
	leftOwner, rightOwner := left.Owner().Content(), right.Owner().Content()
	for index := range leftOwner {
		if leftOwner[index] != rightOwner[index] {
			return leftOwner[index] < rightOwner[index]
		}
	}
	return false
}
