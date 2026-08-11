package topology

// This file deliberately contains the mathematical model used by the public
// topology laws.  It is not an implementation of Graph: it recomputes every
// observable from the complete edge relation, so an incremental Graph cannot
// accidentally share its algorithm or its failure mode with its oracle.

import "sort"

type modelEdge struct {
	from, to           int
	ordinary, boundary bool
}

type modelGraph struct {
	nodes int
	edges map[[2]int]modelEdge
}

func newModel(nodes int) modelGraph {
	return modelGraph{nodes: nodes, edges: make(map[[2]int]modelEdge)}
}

func (graph modelGraph) clone() modelGraph {
	result := newModel(graph.nodes)
	for key, edge := range graph.edges {
		result.edges[key] = edge
	}
	return result
}

// insert is the semantic edge relation: duplicate endpoint records merge
// provenance, and a residual ordinary cycle is rejected atomically.  A
// boundary-bearing edge is removed from the residual graph only when it has
// no ordinary provenance; an edge can legitimately have both sources.
func (graph modelGraph) insert(edge modelEdge) (next modelGraph, accepted, changed bool) {
	if edge.from < 0 || edge.from >= graph.nodes || edge.to < 0 || edge.to >= graph.nodes || (!edge.ordinary && !edge.boundary) {
		return graph, false, false
	}
	result := graph.clone()
	key := [2]int{edge.from, edge.to}
	prior, exists := result.edges[key]
	if exists {
		edge.ordinary = edge.ordinary || prior.ordinary
		edge.boundary = edge.boundary || prior.boundary
		if edge == prior {
			return graph, true, false
		}
	}
	result.edges[key] = edge
	if !result.residualAcyclic() {
		return graph, false, false
	}
	return result, true, true
}

func (graph modelGraph) residualAcyclic() bool {
	indegree := make([]int, graph.nodes)
	successors := make([][]int, graph.nodes)
	for _, edge := range graph.edges {
		if !edge.ordinary {
			continue
		}
		successors[edge.from] = append(successors[edge.from], edge.to)
		indegree[edge.to]++
	}
	ready := make([]int, 0, graph.nodes)
	for node, degree := range indegree {
		if degree == 0 {
			ready = append(ready, node)
		}
	}
	sort.Ints(ready)
	seen := 0
	for len(ready) != 0 {
		node := ready[0]
		ready = ready[1:]
		seen++
		for _, next := range successors[node] {
			indegree[next]--
			if indegree[next] == 0 {
				at := sort.SearchInts(ready, next)
				ready = append(ready, 0)
				copy(ready[at+1:], ready[at:])
				ready[at] = next
			}
		}
	}
	return seen == graph.nodes
}

// components is a deliberately simple reachability definition of the full
// SCC partition.  It neither knows nor uses the incremental condensation
// algorithm under test.
func (graph modelGraph) components() [][]int {
	reachable := make([][]bool, graph.nodes)
	for source := 0; source < graph.nodes; source++ {
		reachable[source] = make([]bool, graph.nodes)
		stack := []int{source}
		reachable[source][source] = true
		for len(stack) != 0 {
			node := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			for _, edge := range graph.edges {
				if edge.from != node || reachable[source][edge.to] {
					continue
				}
				reachable[source][edge.to] = true
				stack = append(stack, edge.to)
			}
		}
	}
	used := make([]bool, graph.nodes)
	components := make([][]int, 0, graph.nodes)
	for node := 0; node < graph.nodes; node++ {
		if used[node] {
			continue
		}
		component := make([]int, 0, graph.nodes)
		for other := node; other < graph.nodes; other++ {
			if reachable[node][other] && reachable[other][node] {
				used[other] = true
				component = append(component, other)
			}
		}
		components = append(components, component)
	}
	sort.Slice(components, func(left, right int) bool { return components[left][0] < components[right][0] })
	return components
}

// residualSweep is the only canonical residual order: lexical Kahn order.
// It exists only when residualAcyclic holds.
func (graph modelGraph) residualSweep() ([]int, bool) {
	indegree := make([]int, graph.nodes)
	successors := make([][]int, graph.nodes)
	for _, edge := range graph.edges {
		if !edge.ordinary {
			continue
		}
		successors[edge.from] = append(successors[edge.from], edge.to)
		indegree[edge.to]++
	}
	for node := range successors {
		sort.Ints(successors[node])
	}
	ready := make([]int, 0, graph.nodes)
	for node, degree := range indegree {
		if degree == 0 {
			ready = append(ready, node)
		}
	}
	sort.Ints(ready)
	order := make([]int, 0, graph.nodes)
	for len(ready) != 0 {
		node := ready[0]
		ready = ready[1:]
		order = append(order, node)
		for _, next := range successors[node] {
			indegree[next]--
			if indegree[next] == 0 {
				at := sort.SearchInts(ready, next)
				ready = append(ready, 0)
				copy(ready[at+1:], ready[at:])
				ready[at] = next
			}
		}
	}
	return order, len(order) == graph.nodes
}
