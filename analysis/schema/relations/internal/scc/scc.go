// Package scc owns the single strongly-connected-component decomposition the
// relations catalog verifies parent cycles with. Both the sealed schema and
// the cold catalog generator answer the same question over the same parent
// relation, so the traversal lives here once and neither side restates it.
package scc

import "sort"

// Components returns the strongly connected components of the parent relation
// that successors reports over nodes, using Tarjan's algorithm.
//
// Order is total and derived only from less: nodes are visited in less order,
// each node's successors are followed in less order, every component is
// returned in less order, and components are returned ordered by their least
// member. successors may report nodes absent from nodes; they join the
// traversal exactly as an unvisited node does.
func Components[N comparable](nodes []N, successors func(N) []N, less func(N, N) bool) [][]N {
	ordered := append([]N(nil), nodes...)
	sort.Slice(ordered, func(left, right int) bool { return less(ordered[left], ordered[right]) })

	index := make(map[N]int, len(ordered))
	lowlink := make(map[N]int, len(ordered))
	onStack := make(map[N]bool, len(ordered))
	stack := make([]N, 0, len(ordered))
	components := make([][]N, 0, len(ordered))
	next := 0
	var visit func(N)
	visit = func(node N) {
		index[node] = next
		lowlink[node] = next
		next++
		stack = append(stack, node)
		onStack[node] = true

		edges := append([]N(nil), successors(node)...)
		sort.Slice(edges, func(left, right int) bool { return less(edges[left], edges[right]) })
		for _, edge := range edges {
			if _, seen := index[edge]; !seen {
				visit(edge)
				if lowlink[edge] < lowlink[node] {
					lowlink[node] = lowlink[edge]
				}
			} else if onStack[edge] && index[edge] < lowlink[node] {
				lowlink[node] = index[edge]
			}
		}

		if lowlink[node] != index[node] {
			return
		}
		component := make([]N, 0, 1)
		for {
			last := len(stack) - 1
			member := stack[last]
			stack = stack[:last]
			onStack[member] = false
			component = append(component, member)
			if member == node {
				break
			}
		}
		sort.Slice(component, func(left, right int) bool { return less(component[left], component[right]) })
		components = append(components, component)
	}
	for _, node := range ordered {
		if _, seen := index[node]; !seen {
			visit(node)
		}
	}
	sort.Slice(components, func(left, right int) bool {
		return less(components[left][0], components[right][0])
	})
	return components
}
