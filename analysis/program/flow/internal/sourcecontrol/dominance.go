package sourcecontrol

import "errors"

// dominanceProof is the compact, immutable result of sealDominance. A zero
// interval means that a node was not reachable from one of the virtual-root
// children. For every reachable node, pre/post are a forest interval over the
// immediate-dominator forest rooted at the virtual root.
//
// The construction scratch (DFS numbering, reverse CSR, and Lengauer-Tarjan
// state) is deliberately not retained. The two dense interval slices are the
// complete query proof.
type dominanceProof struct {
	pre  []uint32
	post []uint32
}

// dominates reports inclusive dominance in constant time. Invalid indices,
// zero intervals, malformed slice pairs, and malformed intervals fail closed.
func (p dominanceProof) dominates(ancestor, descendant uint32) bool {
	if len(p.pre) == 0 || len(p.pre) != len(p.post) ||
		uint64(ancestor) >= uint64(len(p.pre)) || uint64(descendant) >= uint64(len(p.pre)) {
		return false
	}
	ancestorPre, ancestorPost := p.pre[ancestor], p.post[ancestor]
	descendantPre, descendantPost := p.pre[descendant], p.post[descendant]
	if ancestorPre == 0 || ancestorPost == 0 || descendantPre == 0 || descendantPost == 0 ||
		ancestorPost < ancestorPre || descendantPost < descendantPre {
		return false
	}
	return ancestorPre <= descendantPre && descendantPost <= ancestorPost
}

var errMalformedDominance = errors.New("program/flow/sourcecontrol: malformed dominance graph")

// sealDominance proves dominance for a canonical directed graph represented
// as one already-sealed forward/reverse CSR pair. The graph nodes are
// [0,nodeCount), and roots are the exact children of an implicit virtual root
// (the Entry start followed by reachable Function Body starts). Nodes not
// reached from those children receive zero intervals.
//
// The proof uses iterative Lengauer-Tarjan over the virtual-rooted graph. All
// traversal and path-compression work is explicit, so graph depth cannot grow
// the Go call stack. Every temporary array is discarded before the compact
// interval proof is returned.
func sealDominance(
	nodeCount uint32,
	adjacency adjacencyProof,
	roots []uint32,
) (dominanceProof, error) {
	if !validDominanceCSR(nodeCount, adjacency, roots) {
		return dominanceProof{}, errMalformedDominance
	}
	offsets, targets := adjacency.forwardOffsets, adjacency.forwardTargets
	reverseOffsets, reverseTargets := adjacency.reverseOffsets, adjacency.reverseTargets

	nodes := int(nodeCount) + 1 // one additional node is the virtual root
	virtual := nodeCount
	isRoot := make([]bool, int(nodeCount))
	for _, root := range roots {
		isRoot[root] = true
	}

	// Number the virtual-rooted graph with an explicit DFS stack. DFS numbers
	// are the coordinates used by Lengauer-Tarjan; vertex[0] is a sentinel.
	dfs := make([]uint32, nodes)
	vertex := make([]uint32, 1, nodes)
	parent := make([]uint32, 1, nodes)
	dfs[virtual] = 1
	vertex = append(vertex, virtual)
	parent = append(parent, 0)
	type frame struct {
		node uint32
		next uint32
	}
	stack := make([]frame, 0, nodes)
	stack = append(stack, frame{node: virtual})
	for len(stack) != 0 {
		top := &stack[len(stack)-1]
		var child uint32
		haveChild := false
		if top.node == virtual {
			for top.next < uint32(len(roots)) {
				child = roots[top.next]
				top.next++
				if dfs[child] == 0 {
					haveChild = true
					break
				}
			}
		} else {
			for top.next < offsets[top.node+1] {
				child = targets[top.next]
				top.next++
				if dfs[child] == 0 {
					haveChild = true
					break
				}
			}
		}
		if !haveChild {
			stack = stack[:len(stack)-1]
			continue
		}
		number := uint32(len(vertex))
		dfs[child] = number
		vertex = append(vertex, child)
		parent = append(parent, dfs[top.node])
		stack = append(stack, frame{node: child, next: offsets[child]})
	}

	last := len(vertex) - 1
	if last < 1 || uint64(last) > uint64(^uint32(0)) {
		return dominanceProof{}, errMalformedDominance
	}

	// Iterative Lengauer-Tarjan. Every array is one-based in DFS-number space.
	semi := make([]uint32, last+1)
	idom := make([]uint32, last+1)
	ancestor := make([]uint32, last+1)
	label := make([]uint32, last+1)
	bucketHead := make([]uint32, last+1)
	bucketNext := make([]uint32, last+1)
	for number := 1; number <= last; number++ {
		semi[number] = uint32(number)
		label[number] = uint32(number)
	}

	// evalScratch is shared by all path-compression walks. It is bounded by
	// the DFS tree depth and avoids recursion in the union-find evaluator.
	evalScratch := make([]uint32, 0, last)
	eval := func(node uint32) uint32 {
		if ancestor[node] == 0 {
			return label[node]
		}
		evalScratch = evalScratch[:0]
		cursor := node
		for ancestor[cursor] != 0 && ancestor[ancestor[cursor]] != 0 {
			evalScratch = append(evalScratch, cursor)
			cursor = ancestor[cursor]
		}
		for index := len(evalScratch) - 1; index >= 0; index-- {
			at := evalScratch[index]
			ancestorAt := ancestor[at]
			if semi[label[ancestorAt]] < semi[label[at]] {
				label[at] = label[ancestorAt]
			}
			ancestor[at] = ancestor[ancestorAt]
		}
		return label[node]
	}
	consider := func(candidate, node uint32) {
		if candidate == 0 {
			return
		}
		candidate = eval(candidate)
		if semi[candidate] < semi[node] {
			semi[node] = semi[candidate]
		}
	}

	for number := last; number >= 2; number-- {
		original := vertex[number]
		// Every exact root has one incoming virtual-root edge. A root may also
		// have ordinary predecessors, including a predecessor in another
		// activation; all such paths participate in the proof.
		if isRoot[original] {
			consider(1, uint32(number))
		}
		for edge := reverseOffsets[original]; edge < reverseOffsets[original+1]; edge++ {
			predecessor := reverseTargets[edge]
			predecessorNumber := dfs[predecessor]
			if predecessorNumber != 0 {
				consider(predecessorNumber, uint32(number))
			}
		}
		semiNumber := semi[number]
		bucketNext[number] = bucketHead[semiNumber]
		bucketHead[semiNumber] = uint32(number)
		ancestor[number] = parent[number] // link(parent[number], number)
		for member := bucketHead[parent[number]]; member != 0; member = bucketNext[member] {
			candidate := eval(member)
			if semi[candidate] < semi[member] {
				idom[member] = candidate
			} else {
				idom[member] = parent[number]
			}
		}
		bucketHead[parent[number]] = 0
	}

	idom[1] = 1
	for number := 2; number <= last; number++ {
		if idom[number] == 0 || idom[number] > uint32(last) {
			return dominanceProof{}, errMalformedDominance
		}
		if idom[number] != semi[number] {
			idom[number] = idom[idom[number]]
			if idom[number] == 0 {
				return dominanceProof{}, errMalformedDominance
			}
		}
	}

	// Convert the DFS-coordinate dominator tree to graph-node coordinates.
	// The virtual root remains scratch-only; its children are the roots of the
	// compact dominance forest over real graph nodes.
	childCount := make([]uint32, nodes)
	for number := 2; number <= last; number++ {
		parentNumber := idom[number]
		if parentNumber == 0 || parentNumber > uint32(last) {
			return dominanceProof{}, errMalformedDominance
		}
		parentNode := vertex[parentNumber]
		childCount[parentNode]++
	}
	childStart := make([]uint32, nodes+1)
	for index, amount := range childCount {
		childStart[index+1] = childStart[index] + amount
	}
	childNext := make([]uint32, nodes)
	copy(childNext, childStart[:nodes])
	children := make([]uint32, last-1)
	for number := 2; number <= last; number++ {
		child := vertex[number]
		parentNode := vertex[idom[number]]
		children[childNext[parentNode]] = child
		childNext[parentNode]++
	}

	// Euler-style intervals over the immediate-dominator forest. A one-sided
	// clock is sufficient: entry timestamps are strictly increasing, and the
	// exit timestamp is the latest entry in the subtree. This keeps the compact
	// timestamps within uint32 even for the largest indexable graph.
	pre := make([]uint32, int(nodeCount))
	post := make([]uint32, int(nodeCount))
	clock := uint32(0)
	type walkFrame struct {
		node uint32
		next uint32
	}
	walk := make([]walkFrame, 0, nodes)
	walk = append(walk, walkFrame{node: virtual, next: childStart[virtual]})
	for len(walk) != 0 {
		top := &walk[len(walk)-1]
		if top.next < childStart[top.node+1] {
			child := children[top.next]
			top.next++
			clock++
			pre[child] = clock
			walk = append(walk, walkFrame{node: child, next: childStart[child]})
			continue
		}
		if top.node != virtual {
			post[top.node] = clock
		}
		walk = walk[:len(walk)-1]
	}
	if clock != uint32(last-1) {
		return dominanceProof{}, errMalformedDominance
	}
	return dominanceProof{pre: pre, post: post}, nil
}

func validDominanceCSR(nodeCount uint32, adjacency adjacencyProof, roots []uint32) bool {
	if nodeCount == 0 || uint64(nodeCount) > uint64(int(^uint(0)>>1)-2) || len(roots) == 0 ||
		uint64(len(roots)) > uint64(^uint32(0)) {
		return false
	}
	if !validCSR(nodeCount, adjacency.forwardOffsets, adjacency.forwardTargets) ||
		!validCSR(nodeCount, adjacency.reverseOffsets, adjacency.reverseTargets) ||
		len(adjacency.reverseTargets) != len(adjacency.forwardTargets) {
		return false
	}
	// The reverse rows are not merely another well-formed graph: they must be
	// the exact transpose of the forward rows. Check membership against their
	// canonical sorted rows without retaining a validation-side scratch table.
	for from := uint32(0); from < nodeCount; from++ {
		for edge := adjacency.forwardOffsets[from]; edge < adjacency.forwardOffsets[from+1]; edge++ {
			to := adjacency.forwardTargets[edge]
			start, end := adjacency.reverseOffsets[to], adjacency.reverseOffsets[to+1]
			lo, hi := start, end
			for lo < hi {
				middle := lo + (hi-lo)/2
				if adjacency.reverseTargets[middle] < from {
					lo = middle + 1
				} else {
					hi = middle
				}
			}
			if lo == end || adjacency.reverseTargets[lo] != from {
				return false
			}
		}
	}
	var previousRoot uint32
	for index, root := range roots {
		if root >= nodeCount || index != 0 && root <= previousRoot {
			return false
		}
		previousRoot = root
	}
	return true
}

func validCSR(nodeCount uint32, offsets, targets []uint32) bool {
	if uint64(len(offsets)) != uint64(nodeCount)+1 ||
		uint64(len(targets)) > uint64(^uint32(0)) {
		return false
	}
	if offsets[0] != 0 || uint64(offsets[len(offsets)-1]) != uint64(len(targets)) {
		return false
	}
	for row := uint32(0); row < nodeCount; row++ {
		start, end := offsets[row], offsets[row+1]
		if start > end || uint64(end) > uint64(len(targets)) {
			return false
		}
		var previous uint32
		for edge := start; edge < end; edge++ {
			target := targets[edge]
			if target >= nodeCount || edge != start && target <= previous {
				return false
			}
			previous = target
		}
	}
	return true
}
