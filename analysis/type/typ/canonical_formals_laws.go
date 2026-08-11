package typ

import (
	"context"
	"fmt"
)

// canonicalFormalLawPreflight reserves the complete linear working shape of a
// graph law before it allocates any graph-sized temporary.  The law-specific
// constants are deliberately overestimates: this is a hostile artifact
// boundary, and a false rejection of an oversized publication is preferable
// to allocating a proportional graph and discovering the limit afterwards.
func canonicalFormalLawPreflight(ctx context.Context, admission *canonicalFormalsAdmission, steps *uint64, nodes []canonicalTypeNode, bytesPerNode, bytesPerEdge int) error {
	edges := 0
	for index := range nodes {
		if err := canonicalFormalValidationCheckpoint(ctx, admission, steps); err != nil {
			return err
		}
		if len(nodes[index].edges) > maxInt()-edges {
			return invalidCanonicalFormals("edge admission")
		}
		edges += len(nodes[index].edges)
	}
	if err := canonicalFormalsPreflight(ctx, admission, steps, len(nodes), bytesPerNode); err != nil {
		return err
	}
	return canonicalFormalsPreflight(ctx, admission, steps, edges, bytesPerEdge)
}

func canonicalFormalForwardPreflight(ctx context.Context, admission *canonicalFormalsAdmission, steps *uint64, forward [][]int, bytesPerNode, bytesPerEdge int) error {
	edges := 0
	for index := range forward {
		if err := canonicalFormalValidationCheckpoint(ctx, admission, steps); err != nil {
			return err
		}
		if len(forward[index]) > maxInt()-edges {
			return invalidCanonicalFormals("edge admission")
		}
		edges += len(forward[index])
	}
	if err := canonicalFormalsPreflight(ctx, admission, steps, len(forward), bytesPerNode); err != nil {
		return err
	}
	return canonicalFormalsPreflight(ctx, admission, steps, edges, bytesPerEdge)
}

func validateCanonicalFormalConstraintCycles(ctx context.Context, admission *canonicalFormalsAdmission, nodes []canonicalTypeNode, shapes []canonicalFormalNodeShape) error {
	if len(nodes) == 0 || len(nodes) != len(shapes) {
		return invalidCanonicalFormals("graph shape")
	}
	var steps uint64
	if err := canonicalFormalLawPreflight(ctx, admission, &steps, nodes, 192, 32); err != nil {
		return err
	}
	forward, reverse := make([][]int, len(nodes)), make([][]int, len(nodes))
	var appendErr error
	for source := range nodes {
		if err := canonicalFormalValidationCheckpoint(ctx, admission, &steps); err != nil {
			return err
		}
		for _, target := range canonicalFormalConstraintEdges(source, nodes, shapes) {
			if err := canonicalFormalValidationCheckpoint(ctx, admission, &steps); err != nil {
				return err
			}
			if target < 0 || target >= len(nodes) {
				return invalidCanonicalFormals("edge outside graph")
			}
			forward[source], appendErr = canonicalFormalsAppend(ctx, admission, &steps, forward[source], target, canonicalFormalsIntBytes)
			if appendErr != nil {
				return appendErr
			}
			reverse[target], appendErr = canonicalFormalsAppend(ctx, admission, &steps, reverse[target], source, canonicalFormalsIntBytes)
			if appendErr != nil {
				return appendErr
			}
		}
	}
	order, err := canonicalFormalFinishOrder(ctx, admission, forward, &steps)
	if err != nil {
		return err
	}
	component := make([]int, len(nodes))
	for index := range component {
		component[index] = -1
	}
	componentSize := make([]int, 0, len(nodes))
	for orderIndex := len(order) - 1; orderIndex >= 0; orderIndex-- {
		if err := canonicalFormalValidationCheckpoint(ctx, admission, &steps); err != nil {
			return err
		}
		root := order[orderIndex]
		if component[root] >= 0 {
			continue
		}
		identifier := len(componentSize)
		var stack []int
		stack, appendErr = canonicalFormalsAppend(ctx, admission, &steps, stack, root, canonicalFormalsIntBytes)
		if appendErr != nil {
			return appendErr
		}
		component[root] = identifier
		size := 0
		for len(stack) != 0 {
			if err := canonicalFormalValidationCheckpoint(ctx, admission, &steps); err != nil {
				return err
			}
			current := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			size++
			for _, parent := range reverse[current] {
				if err := canonicalFormalValidationCheckpoint(ctx, admission, &steps); err != nil {
					return err
				}
				if component[parent] >= 0 {
					continue
				}
				component[parent] = identifier
				stack, appendErr = canonicalFormalsAppend(ctx, admission, &steps, stack, parent, canonicalFormalsIntBytes)
				if appendErr != nil {
					return appendErr
				}
			}
		}
		componentSize, appendErr = canonicalFormalsAppend(ctx, admission, &steps, componentSize, size, canonicalFormalsIntBytes)
		if appendErr != nil {
			return appendErr
		}
	}
	for formal, shape := range shapes {
		if err := canonicalFormalValidationCheckpoint(ctx, admission, &steps); err != nil {
			return err
		}
		if shape.tag != canonicalTypeParam || !canonicalFormalHasConstraint(formal, nodes, shape) {
			continue
		}
		id := component[formal]
		cyclic := componentSize[id] > 1
		if !cyclic {
			for _, child := range forward[formal] {
				if child == formal {
					cyclic = true
					break
				}
			}
		}
		if cyclic {
			return invalidCanonicalFormals("formal constraint recurrence without Mu")
		}
	}
	return nil
}

func canonicalFormalConstraintEdges(index int, nodes []canonicalTypeNode, shapes []canonicalFormalNodeShape) []int {
	shape := shapes[index]
	if shape.tag == canonicalRecursive {
		return nil
	}
	if shape.tag == canonicalTypeParam && shape.formalMode == canonicalScopedLocalFormal {
		if len(nodes[index].edges) == 2 {
			return nodes[index].edges[1:]
		}
		return nil
	}
	return nodes[index].edges
}

func canonicalFormalHasConstraint(index int, nodes []canonicalTypeNode, shape canonicalFormalNodeShape) bool {
	if shape.formalMode == canonicalScopedLocalFormal {
		return len(nodes[index].edges) == 2
	}
	return shape.formalMode == canonicalScopedExternalFormal && len(nodes[index].edges) == 1
}

func canonicalFormalFinishOrder(ctx context.Context, admission *canonicalFormalsAdmission, forward [][]int, steps *uint64) ([]int, error) {
	if err := canonicalFormalForwardPreflight(ctx, admission, steps, forward, 40, 16); err != nil {
		return nil, err
	}
	seen := make([]bool, len(forward))
	order := make([]int, 0, len(forward))
	type frame struct{ node, next int }
	var appendErr error
	for root := range forward {
		if err := canonicalFormalValidationCheckpoint(ctx, admission, steps); err != nil {
			return nil, err
		}
		if seen[root] {
			continue
		}
		seen[root] = true
		var stack []frame
		stack, appendErr = canonicalFormalsAppend(ctx, admission, steps, stack, frame{node: root}, canonicalFormalsFrameBytes)
		if appendErr != nil {
			return nil, appendErr
		}
		for len(stack) != 0 {
			if err := canonicalFormalValidationCheckpoint(ctx, admission, steps); err != nil {
				return nil, err
			}
			top := &stack[len(stack)-1]
			if top.next == len(forward[top.node]) {
				order, appendErr = canonicalFormalsAppend(ctx, admission, steps, order, top.node, canonicalFormalsIntBytes)
				if appendErr != nil {
					return nil, appendErr
				}
				stack = stack[:len(stack)-1]
				continue
			}
			child := forward[top.node][top.next]
			top.next++
			if seen[child] {
				continue
			}
			seen[child] = true
			stack, appendErr = canonicalFormalsAppend(ctx, admission, steps, stack, frame{node: child}, canonicalFormalsFrameBytes)
			if appendErr != nil {
				return nil, appendErr
			}
		}
	}
	return order, nil
}

func validateCanonicalFormalGenericRecurrence(ctx context.Context, admission *canonicalFormalsAdmission, nodes []canonicalTypeNode, shapes []canonicalFormalNodeShape) error {
	component, sizes, err := canonicalFormalGenericComponents(ctx, admission, nodes, shapes)
	if err != nil {
		return err
	}
	if len(component) != len(nodes) {
		return invalidCanonicalFormals("edge outside graph")
	}
	var steps uint64
	if err := canonicalFormalsPreflight(ctx, admission, &steps, len(sizes), canonicalFormalsIntBytes); err != nil {
		return err
	}
	generics := make([]int, len(sizes))
	for index, shape := range shapes {
		if err := canonicalFormalValidationCheckpoint(ctx, admission, &steps); err != nil {
			return err
		}
		if shape.tag == canonicalGeneric {
			generics[component[index]]++
		}
	}
	for identifier, count := range generics {
		if err := canonicalFormalValidationCheckpoint(ctx, admission, &steps); err != nil {
			return err
		}
		if count > 1 && sizes[identifier] > 1 {
			return invalidCanonicalFormals("mutual generic recurrence without Mu")
		}
	}
	return nil
}

func canonicalFormalGenericComponents(ctx context.Context, admission *canonicalFormalsAdmission, nodes []canonicalTypeNode, shapes []canonicalFormalNodeShape) ([]int, []int, error) {
	var steps uint64
	if err := canonicalFormalLawPreflight(ctx, admission, &steps, nodes, 32, 16); err != nil {
		return nil, nil, err
	}
	forward := make([][]int, len(nodes))
	var appendErr error
	for index := range nodes {
		if err := canonicalFormalValidationCheckpoint(ctx, admission, &steps); err != nil {
			return nil, nil, err
		}
		for _, child := range canonicalFormalGenericRecurrenceEdges(index, nodes, shapes) {
			if err := canonicalFormalValidationCheckpoint(ctx, admission, &steps); err != nil {
				return nil, nil, err
			}
			if child < 0 || child >= len(nodes) {
				return nil, nil, nil
			}
			forward[index], appendErr = canonicalFormalsAppend(ctx, admission, &steps, forward[index], child, canonicalFormalsIntBytes)
			if appendErr != nil {
				return nil, nil, appendErr
			}
		}
	}
	return canonicalFormalStrongComponents(ctx, admission, forward, &steps)
}

func canonicalFormalGenericRecurrenceEdges(index int, nodes []canonicalTypeNode, shapes []canonicalFormalNodeShape) []int {
	if shapes[index].tag == canonicalRecursive {
		return nil
	}
	return nodes[index].edges
}

func canonicalFormalStrongComponents(ctx context.Context, admission *canonicalFormalsAdmission, forward [][]int, steps *uint64) ([]int, []int, error) {
	if err := canonicalFormalForwardPreflight(ctx, admission, steps, forward, 96, 24); err != nil {
		return nil, nil, err
	}
	reverse := make([][]int, len(forward))
	var appendErr error
	for source, children := range forward {
		if err := canonicalFormalValidationCheckpoint(ctx, admission, steps); err != nil {
			return nil, nil, err
		}
		for _, target := range children {
			if err := canonicalFormalValidationCheckpoint(ctx, admission, steps); err != nil {
				return nil, nil, err
			}
			reverse[target], appendErr = canonicalFormalsAppend(ctx, admission, steps, reverse[target], source, canonicalFormalsIntBytes)
			if appendErr != nil {
				return nil, nil, appendErr
			}
		}
	}
	order, err := canonicalFormalFinishOrder(ctx, admission, forward, steps)
	if err != nil {
		return nil, nil, err
	}
	component := make([]int, len(forward))
	for index := range component {
		component[index] = -1
	}
	sizes := make([]int, 0, len(forward))
	for orderIndex := len(order) - 1; orderIndex >= 0; orderIndex-- {
		if err := canonicalFormalValidationCheckpoint(ctx, admission, steps); err != nil {
			return nil, nil, err
		}
		root := order[orderIndex]
		if component[root] >= 0 {
			continue
		}
		identifier := len(sizes)
		var stack []int
		stack, appendErr = canonicalFormalsAppend(ctx, admission, steps, stack, root, canonicalFormalsIntBytes)
		if appendErr != nil {
			return nil, nil, appendErr
		}
		component[root] = identifier
		size := 0
		for len(stack) != 0 {
			if err := canonicalFormalValidationCheckpoint(ctx, admission, steps); err != nil {
				return nil, nil, err
			}
			current := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			size++
			for _, parent := range reverse[current] {
				if err := canonicalFormalValidationCheckpoint(ctx, admission, steps); err != nil {
					return nil, nil, err
				}
				if component[parent] >= 0 {
					continue
				}
				component[parent] = identifier
				stack, appendErr = canonicalFormalsAppend(ctx, admission, steps, stack, parent, canonicalFormalsIntBytes)
				if appendErr != nil {
					return nil, nil, appendErr
				}
			}
		}
		sizes, appendErr = canonicalFormalsAppend(ctx, admission, steps, sizes, size, canonicalFormalsIntBytes)
		if appendErr != nil {
			return nil, nil, appendErr
		}
	}
	return component, sizes, nil
}

func validateCanonicalFormalMaterializability(ctx context.Context, admission *canonicalFormalsAdmission, nodes []canonicalTypeNode, shapes []canonicalFormalNodeShape) error {
	if len(nodes) == 0 || len(nodes) != len(shapes) {
		return invalidCanonicalFormals("graph shape")
	}
	var steps uint64
	if err := canonicalFormalLawPreflight(ctx, admission, &steps, nodes, 112, 16); err != nil {
		return err
	}
	preallocated := make([]bool, len(nodes))
	var appendErr error
	for index, shape := range shapes {
		if err := canonicalFormalValidationCheckpoint(ctx, admission, &steps); err != nil {
			return err
		}
		preallocated[index] = shape.tag == canonicalRecursive || shape.tag == canonicalTypeParam && shape.formalMode == canonicalScopedExternalFormal
	}
	waiting := make([]int, len(nodes))
	dependents := make([][]int, len(nodes))
	remaining := 0
	for index := range nodes {
		if err := canonicalFormalValidationCheckpoint(ctx, admission, &steps); err != nil {
			return err
		}
		if preallocated[index] {
			continue
		}
		remaining++
		for _, child := range canonicalFormalMaterializationDependencies(index, nodes, shapes) {
			if err := canonicalFormalValidationCheckpoint(ctx, admission, &steps); err != nil {
				return err
			}
			if child < 0 || child >= len(nodes) {
				return invalidCanonicalFormals("edge outside graph")
			}
			if preallocated[child] {
				continue
			}
			waiting[index]++
			dependents[child], appendErr = canonicalFormalsAppend(ctx, admission, &steps, dependents[child], index, canonicalFormalsIntBytes)
			if appendErr != nil {
				return appendErr
			}
		}
	}
	queue := make([]int, 0, remaining)
	for index := range nodes {
		if err := canonicalFormalValidationCheckpoint(ctx, admission, &steps); err != nil {
			return err
		}
		if !preallocated[index] && waiting[index] == 0 {
			queue, appendErr = canonicalFormalsAppend(ctx, admission, &steps, queue, index, canonicalFormalsIntBytes)
			if appendErr != nil {
				return appendErr
			}
		}
	}
	processed := 0
	for head := 0; head < len(queue); head++ {
		if err := canonicalFormalValidationCheckpoint(ctx, admission, &steps); err != nil {
			return err
		}
		current := queue[head]
		processed++
		for _, parent := range dependents[current] {
			if err := canonicalFormalValidationCheckpoint(ctx, admission, &steps); err != nil {
				return err
			}
			waiting[parent]--
			if waiting[parent] == 0 {
				queue, appendErr = canonicalFormalsAppend(ctx, admission, &steps, queue, parent, canonicalFormalsIntBytes)
				if appendErr != nil {
					return appendErr
				}
			}
		}
	}
	if processed != remaining {
		return invalidCanonicalFormals("cycle lacks materializable recursive head")
	}
	return nil
}

func canonicalFormalMaterializationDependencies(index int, nodes []canonicalTypeNode, shapes []canonicalFormalNodeShape) []int {
	shape := shapes[index]
	if shape.tag == canonicalRecursive || shape.tag == canonicalTypeParam && shape.formalMode == canonicalScopedExternalFormal {
		return nil
	}
	if shape.tag == canonicalTypeParam && shape.formalMode == canonicalScopedLocalFormal {
		if len(nodes[index].edges) == 2 {
			return nodes[index].edges[1:]
		}
		return nil
	}
	if shape.tag == canonicalGeneric {
		return nodes[index].edges[:shape.binderParams]
	}
	return nodes[index].edges
}

func validateCanonicalFormalLexicalScope(ctx context.Context, admission *canonicalFormalsAdmission, nodes []canonicalTypeNode, shapes []canonicalFormalNodeShape) error {
	if len(nodes) == 0 || len(nodes) != len(shapes) {
		return invalidCanonicalFormals("graph shape")
	}
	var steps uint64
	if err := canonicalFormalLawPreflight(ctx, admission, &steps, nodes, 320, 24); err != nil {
		return err
	}
	predecessors := make([][]int, len(nodes))
	var appendErr error
	for source, node := range nodes {
		if err := canonicalFormalValidationCheckpoint(ctx, admission, &steps); err != nil {
			return err
		}
		for _, target := range node.edges {
			if err := canonicalFormalValidationCheckpoint(ctx, admission, &steps); err != nil {
				return err
			}
			if target < 0 || target >= len(nodes) {
				return invalidCanonicalFormals("edge outside graph")
			}
			predecessors[target], appendErr = canonicalFormalsAppend(ctx, admission, &steps, predecessors[target], source, canonicalFormalsIntBytes)
			if appendErr != nil {
				return appendErr
			}
		}
	}
	vertex, number, parent, err := canonicalFormalDepthFirstOrder(ctx, admission, nodes, &steps)
	if err != nil {
		return err
	}
	semi := make([]int, len(nodes)+1)
	idom := make([]int, len(nodes)+1)
	ancestor := make([]int, len(nodes)+1)
	label := make([]int, len(nodes)+1)
	bucketHead := make([]int, len(nodes)+1)
	bucketNext := make([]int, len(nodes)+1)
	for index := 1; index <= len(nodes); index++ {
		if err := canonicalFormalValidationCheckpoint(ctx, admission, &steps); err != nil {
			return err
		}
		semi[index], label[index], bucketHead[index] = index, index, -1
	}
	if err := canonicalFormalsPreflight(ctx, admission, &steps, len(nodes), canonicalFormalsIntBytes); err != nil {
		return err
	}
	compressionPath := make([]int, 0, len(nodes))
	eval := func(value int) (int, error) { return value, nil }
	eval = func(value int) (int, error) {
		if ancestor[value] == 0 {
			return label[value], nil
		}
		path := compressionPath[:0]
		for cursor := value; ancestor[cursor] != 0 && ancestor[ancestor[cursor]] != 0; cursor = ancestor[cursor] {
			if err := canonicalFormalValidationCheckpoint(ctx, admission, &steps); err != nil {
				return 0, err
			}
			path = append(path, cursor)
		}
		for index := len(path) - 1; index >= 0; index-- {
			if err := canonicalFormalValidationCheckpoint(ctx, admission, &steps); err != nil {
				return 0, err
			}
			current := path[index]
			up := ancestor[current]
			if semi[label[up]] < semi[label[current]] {
				label[current] = label[up]
			}
			ancestor[current] = ancestor[up]
		}
		return label[value], nil
	}
	for current := len(nodes); current >= 2; current-- {
		if err := canonicalFormalValidationCheckpoint(ctx, admission, &steps); err != nil {
			return err
		}
		original := vertex[current]
		for _, predecessor := range predecessors[original] {
			if err := canonicalFormalValidationCheckpoint(ctx, admission, &steps); err != nil {
				return err
			}
			candidate, err := eval(number[predecessor])
			if err != nil {
				return err
			}
			if semi[candidate] < semi[current] {
				semi[current] = semi[candidate]
			}
		}
		bucketNext[current] = bucketHead[semi[current]]
		bucketHead[semi[current]] = current
		ancestor[current] = parent[current]
		for member := bucketHead[parent[current]]; member >= 0; member = bucketNext[member] {
			if err := canonicalFormalValidationCheckpoint(ctx, admission, &steps); err != nil {
				return err
			}
			candidate, err := eval(member)
			if err != nil {
				return err
			}
			if semi[candidate] < semi[member] {
				idom[member] = candidate
			} else {
				idom[member] = parent[current]
			}
		}
		bucketHead[parent[current]] = -1
	}
	for current := 2; current <= len(nodes); current++ {
		if err := canonicalFormalValidationCheckpoint(ctx, admission, &steps); err != nil {
			return err
		}
		if idom[current] != semi[current] {
			idom[current] = idom[idom[current]]
		}
	}
	entry, exit, err := canonicalFormalDominatorIntervals(ctx, admission, vertex, idom, &steps)
	if err != nil {
		return err
	}
	for formal, shape := range shapes {
		if err := canonicalFormalValidationCheckpoint(ctx, admission, &steps); err != nil {
			return err
		}
		if shape.tag != canonicalTypeParam || shape.formalMode != canonicalScopedLocalFormal {
			continue
		}
		owner := nodes[formal].edges[0]
		if !canonicalFormalDominates(entry, exit, owner, formal) {
			return invalidCanonicalFormals("local formal escapes lexical owner")
		}
		for _, source := range predecessors[formal] {
			if err := canonicalFormalValidationCheckpoint(ctx, admission, &steps); err != nil {
				return err
			}
			if !canonicalFormalDominates(entry, exit, owner, source) {
				return invalidCanonicalFormals(fmt.Sprintf("local formal escapes lexical owner (formal=%d owner=%d source=%d)", formal, owner, source))
			}
		}
	}
	return nil
}

func canonicalFormalDepthFirstOrder(ctx context.Context, admission *canonicalFormalsAdmission, nodes []canonicalTypeNode, steps *uint64) (vertex, number, parent []int, err error) {
	if err = canonicalFormalLawPreflight(ctx, admission, steps, nodes, 48, 0); err != nil {
		return nil, nil, nil, err
	}
	vertex = make([]int, len(nodes)+1)
	number = make([]int, len(nodes))
	parent = make([]int, len(nodes)+1)
	type frame struct{ node, next int }
	var stack []frame
	var appendErr error
	vertex[1], number[0] = 0, 1
	stack, appendErr = canonicalFormalsAppend(ctx, admission, steps, stack, frame{node: 0}, canonicalFormalsFrameBytes)
	if appendErr != nil {
		return nil, nil, nil, appendErr
	}
	count := 1
	for len(stack) != 0 {
		if err := canonicalFormalValidationCheckpoint(ctx, admission, steps); err != nil {
			return nil, nil, nil, err
		}
		top := &stack[len(stack)-1]
		if top.next == len(nodes[top.node].edges) {
			stack = stack[:len(stack)-1]
			continue
		}
		child := nodes[top.node].edges[top.next]
		top.next++
		if child < 0 || child >= len(nodes) {
			return nil, nil, nil, invalidCanonicalFormals("edge outside graph")
		}
		if number[child] != 0 {
			continue
		}
		count++
		number[child], vertex[count], parent[count] = count, child, number[top.node]
		stack, appendErr = canonicalFormalsAppend(ctx, admission, steps, stack, frame{node: child}, canonicalFormalsFrameBytes)
		if appendErr != nil {
			return nil, nil, nil, appendErr
		}
	}
	if count != len(nodes) {
		return nil, nil, nil, invalidCanonicalFormals("unowned graph node")
	}
	return vertex, number, parent, nil
}

func canonicalFormalDominatorIntervals(ctx context.Context, admission *canonicalFormalsAdmission, vertex, idom []int, steps *uint64) ([]int, []int, error) {
	if err := canonicalFormalsPreflight(ctx, admission, steps, len(vertex), 64); err != nil {
		return nil, nil, err
	}
	children := make([][]int, len(vertex)-1)
	var appendErr error
	for child := 2; child < len(vertex); child++ {
		if err := canonicalFormalValidationCheckpoint(ctx, admission, steps); err != nil {
			return nil, nil, err
		}
		owner := vertex[idom[child]]
		children[owner], appendErr = canonicalFormalsAppend(ctx, admission, steps, children[owner], vertex[child], canonicalFormalsIntBytes)
		if appendErr != nil {
			return nil, nil, appendErr
		}
	}
	entry, exit := make([]int, len(children)), make([]int, len(children))
	type frame struct{ node, next int }
	var stack []frame
	stack, appendErr = canonicalFormalsAppend(ctx, admission, steps, stack, frame{node: vertex[1]}, canonicalFormalsFrameBytes)
	if appendErr != nil {
		return nil, nil, appendErr
	}
	clock := 1
	entry[vertex[1]] = clock
	clock++
	for len(stack) != 0 {
		if err := canonicalFormalValidationCheckpoint(ctx, admission, steps); err != nil {
			return nil, nil, err
		}
		top := &stack[len(stack)-1]
		if top.next == len(children[top.node]) {
			exit[top.node] = clock
			clock++
			stack = stack[:len(stack)-1]
			continue
		}
		child := children[top.node][top.next]
		top.next++
		entry[child] = clock
		clock++
		stack, appendErr = canonicalFormalsAppend(ctx, admission, steps, stack, frame{node: child}, canonicalFormalsFrameBytes)
		if appendErr != nil {
			return nil, nil, appendErr
		}
	}
	return entry, exit, nil
}

func canonicalFormalDominates(entry, exit []int, owner, use int) bool {
	return owner >= 0 && owner < len(entry) && use >= 0 && use < len(entry) &&
		entry[owner] <= entry[use] && exit[use] <= exit[owner]
}
