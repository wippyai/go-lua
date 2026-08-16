package recurrence

// NoNode is the forest sentinel used by OfflineLCAs: it marks a root in
// parents and an unanswered query in the returned answers.
const NoNode = ^uint32(0)

// OfflineLCAs answers every (left[i], right[i]) least-common-ancestor query
// over a parent-indexed forest in one iterative Tarjan union-find pass. It is
// expressed with an explicit stack so deep program nesting cannot consume the
// Go call stack, and it is linear in the forest plus the query count.
//
// parents holds NoNode for roots and otherwise a strictly smaller index;
// roots carries each node's own root so queries spanning separate roots stay
// NoNode rather than collapsing onto a false common ancestor. The result is
// nil when the forest or query vectors violate that shape.
//
// Recurrence owns this because it issues the hierarchy region tree every
// consumer resolves against; Causal joins its published regions to the same
// forest rather than restating the traversal.
func OfflineLCAs(parents, roots, left, right []uint32) []uint32 {
	if len(parents) == 0 || len(roots) != len(parents) || len(left) != len(right) {
		return nil
	}
	children := make([][]uint32, len(parents))
	for child, parent := range parents {
		if parent == NoNode {
			continue
		}
		if int(parent) >= len(parents) || parent >= uint32(child) {
			return nil
		}
		children[parent] = append(children[parent], uint32(child))
	}
	queries := make([][]uint32, len(parents))
	answers := make([]uint32, len(left))
	for index := range answers {
		answers[index] = NoNode
		if int(left[index]) >= len(parents) || int(right[index]) >= len(parents) {
			return nil
		}
		if roots[left[index]] != roots[right[index]] {
			continue
		}
		if left[index] == right[index] {
			answers[index] = left[index]
			continue
		}
		queries[left[index]] = append(queries[left[index]], uint32(index))
		queries[right[index]] = append(queries[right[index]], uint32(index))
	}
	disjoint := make([]uint32, len(parents))
	ancestor := make([]uint32, len(parents))
	black := make([]bool, len(parents))
	for index := range disjoint {
		disjoint[index], ancestor[index] = uint32(index), uint32(index)
	}
	find := func(node uint32) uint32 {
		root := node
		for disjoint[root] != root {
			root = disjoint[root]
		}
		for node != root {
			next := disjoint[node]
			disjoint[node] = root
			node = next
		}
		return root
	}
	type frame struct {
		node uint32
		next int
	}
	for root, parent := range parents {
		if parent != NoNode {
			continue
		}
		stack := []frame{{node: uint32(root)}}
		for len(stack) != 0 {
			current := &stack[len(stack)-1]
			if current.next < len(children[current.node]) {
				child := children[current.node][current.next]
				current.next++
				stack = append(stack, frame{node: child})
				continue
			}
			node := current.node
			black[node] = true
			for _, query := range queries[node] {
				other := left[query]
				if other == node {
					other = right[query]
				}
				if black[other] && answers[query] == NoNode {
					answers[query] = ancestor[find(other)]
				}
			}
			stack = stack[:len(stack)-1]
			if len(stack) != 0 {
				parent := stack[len(stack)-1].node
				disjoint[find(node)] = find(parent)
				ancestor[find(parent)] = parent
			}
		}
	}
	return answers
}
