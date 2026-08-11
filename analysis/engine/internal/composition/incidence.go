package composition

import "sort"

func deriveIncidence(rules []Rule) []Incidence {
	set := make(map[Incidence]struct{})
	for _, rule := range rules {
		if rule.OutputKind != FactorOutput {
			continue
		}
		for _, read := range rule.Reads {
			set[Incidence{Read: read.Factor, Write: rule.Output}] = struct{}{}
		}
		for _, carry := range rule.Carries {
			set[Incidence{Read: carry.Factor, Write: rule.Output}] = struct{}{}
		}
	}
	result := make([]Incidence, 0, len(set))
	for edge := range set {
		result = append(result, edge)
	}
	sort.Slice(result, func(i, j int) bool {
		if c := compareKey(result[i].Read, result[j].Read); c != 0 {
			return c < 0
		}
		return lessKey(result[i].Write, result[j].Write)
	})
	return result
}

func canonicalComponents(factors []Factor, edges []Incidence) ([]Component, bool) {
	adj := make(map[Key][]Key, len(factors))
	known := make(map[Key]struct{}, len(factors))
	for _, factor := range factors {
		known[factor.Key] = struct{}{}
	}
	for _, edge := range edges {
		if _, ok := known[edge.Read]; !ok {
			return nil, false
		}
		if _, ok := known[edge.Write]; !ok {
			return nil, false
		}
		adj[edge.Read] = append(adj[edge.Read], edge.Write)
	}
	for key := range adj {
		sort.Slice(adj[key], func(i, j int) bool { return lessKey(adj[key][i], adj[key][j]) })
	}
	index, stack, onStack := 0, []Key(nil), make(map[Key]bool)
	indices, low := make(map[Key]int), make(map[Key]int)
	parts := make([][]Key, 0)
	var visit func(Key)
	visit = func(node Key) {
		indices[node], low[node] = index, index
		index++
		stack = append(stack, node)
		onStack[node] = true
		for _, next := range adj[node] {
			if _, seen := indices[next]; !seen {
				visit(next)
				if low[next] < low[node] {
					low[node] = low[next]
				}
			} else if onStack[next] && indices[next] < low[node] {
				low[node] = indices[next]
			}
		}
		if low[node] == indices[node] {
			part := []Key(nil)
			for {
				last := len(stack) - 1
				next := stack[last]
				stack = stack[:last]
				onStack[next] = false
				part = append(part, next)
				if next == node {
					break
				}
			}
			sort.Slice(part, func(i, j int) bool { return lessKey(part[i], part[j]) })
			parts = append(parts, part)
		}
	}
	for _, factor := range factors {
		if _, seen := indices[factor.Key]; !seen {
			visit(factor.Key)
		}
	}
	sort.Slice(parts, func(i, j int) bool { return lessKey(parts[i][0], parts[j][0]) })
	owner := make(map[Key]int)
	for i, part := range parts {
		for _, key := range part {
			owner[key] = i
		}
	}
	result := make([]Component, len(parts))
	for i, part := range parts {
		result[i].Factors = append([]Key(nil), part...)
	}
	successors := make([]map[int]struct{}, len(result))
	for i := range successors {
		successors[i] = make(map[int]struct{})
	}
	for _, edge := range edges {
		from, to := owner[edge.Read], owner[edge.Write]
		if from != to {
			successors[from][to] = struct{}{}
		}
	}
	for i := range result {
		for target := range successors[i] {
			result[i].Successors = append(result[i].Successors, parts[target][0])
		}
		sort.Slice(result[i].Successors, func(a, b int) bool { return lessKey(result[i].Successors[a], result[i].Successors[b]) })
	}
	return result, true
}
