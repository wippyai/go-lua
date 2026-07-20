package transformer

import (
	"context"
	"sort"
)

type decisionLeafTupleRegion struct {
	leaves []decisionLeaf
	care   decisionRef
}

type decisionTupleProductKey struct {
	care   decisionRef
	values decisionVectorRef
}

// partitionLeafTuplesUnderCare computes the exact joint terminal partition of
// every demanded root in one synchronous product traversal.  The former
// incremental schedule rebuilt a two-root product for every live prefix and
// every root; its work therefore grew with intermediate tuple multiplicity.
// Here each reachable (care, vector) state has one canonical arena identity,
// and only final terminal tuples are materialized.
func (k *decisionKernel) partitionLeafTuplesUnderCare(
	ctx context.Context,
	care decisionRef,
	roots []decisionRef,
) ([]decisionLeafTupleRegion, error) {
	if ctx == nil {
		return nil, errDecisionMalformed
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if care == decisionFalse || int(care) >= len(k.nodes) {
		return nil, errDecisionMalformed
	}
	for _, root := range roots {
		if int(root) >= len(k.nodes) {
			return nil, errDecisionMalformed
		}
	}
	type productNode struct {
		key            decisionTupleProductKey
		terminal       bool
		variable       uint32
		low, high      int
		lowLive        bool
		highLive       bool
		reachableGuard decisionRef
	}

	vectors := newDecisionVectorArena(k)
	rootVector, err := vectors.Build(roots)
	if err != nil {
		return nil, err
	}
	rootKey := decisionTupleProductKey{care: care, values: rootVector}
	indices := map[decisionTupleProductKey]int{rootKey: 0}
	nodes := []productNode{{key: rootKey}}
	steps := uint64(0)
	for index := 0; index < len(nodes); index++ {
		steps++
		if steps&255 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		key := nodes[index].key
		if int(key.care) >= len(k.nodes) {
			return nil, errDecisionMalformed
		}
		careNode := k.nodes[key.care]
		if careNode.terminal && careNode.leaf != 1 {
			return nil, errDecisionMalformed
		}
		width, widthErr := vectors.Width(key.values)
		if widthErr != nil || width != len(roots) {
			return nil, errDecisionMalformed
		}
		variable := ^uint32(0)
		if !careNode.terminal {
			variable = careNode.variable
		}
		vectorMinimum, minimumErr := vectors.MinVariable(key.values)
		if minimumErr != nil {
			return nil, minimumErr
		}
		if vectorMinimum < variable {
			variable = vectorMinimum
		}
		if variable == ^uint32(0) {
			nodes[index].terminal = true
			continue
		}
		nodes[index].variable = variable
		split := func(ref decisionRef) (decisionRef, decisionRef) {
			node := k.nodes[ref]
			if !node.terminal && node.variable == variable {
				return node.low, node.high
			}
			return ref, ref
		}
		careLow, careHigh := split(key.care)
		live := [2]bool{careLow != decisionFalse, careHigh != decisionFalse}
		childKeys := [2]decisionTupleProductKey{{care: careLow}, {care: careHigh}}
		valuesLow, valuesHigh, splitErr := vectors.SplitAt(key.values, variable)
		if splitErr != nil {
			return nil, splitErr
		}
		if live[0] {
			childKeys[0].values = valuesLow
		}
		if live[1] {
			childKeys[1].values = valuesHigh
		}
		childIndices := [2]int{-1, -1}
		for childIndex, child := range childKeys {
			if !live[childIndex] {
				continue
			}
			known, exists := indices[child]
			if !exists {
				known = len(nodes)
				indices[child] = known
				nodes = append(nodes, productNode{key: child})
			}
			childIndices[childIndex] = known
		}
		nodes[index].low, nodes[index].high = childIndices[0], childIndices[1]
		nodes[index].lowLive, nodes[index].highLive = live[0], live[1]
	}

	// Decision variables strictly increase along every product edge. Sorting
	// by the next variable therefore gives a deterministic topological order
	// for forward aggregation of exact path guards.
	order := make([]int, len(nodes))
	for index := range nodes {
		order[index] = index
	}
	sort.Slice(order, func(i, j int) bool {
		left, right := nodes[order[i]], nodes[order[j]]
		if left.terminal != right.terminal {
			return !left.terminal
		}
		if left.variable != right.variable {
			return left.variable < right.variable
		}
		return order[i] < order[j]
	})
	nodes[0].reachableGuard = decisionTrue
	mergeGuard := func(index int, contribution decisionRef) error {
		if contribution == decisionFalse {
			return nil
		}
		if nodes[index].reachableGuard == decisionFalse {
			nodes[index].reachableGuard = contribution
			return nil
		}
		merged, err := k.apply(ctx, uint8(decisionOr), true, nodes[index].reachableGuard, contribution, decisionLeafOr)
		if err != nil {
			return err
		}
		nodes[index].reachableGuard = merged
		return nil
	}
	for _, index := range order {
		current := nodes[index]
		if current.terminal {
			continue
		}
		if current.reachableGuard == decisionFalse {
			return nil, errDecisionMalformed
		}
		if current.lowLive {
			literal := k.branch(current.variable, decisionTrue, decisionFalse)
			contribution, err := k.apply(ctx, uint8(decisionAnd), true, current.reachableGuard, literal, decisionLeafAnd)
			if err != nil {
				return nil, err
			}
			if err := mergeGuard(current.low, contribution); err != nil {
				return nil, err
			}
		}
		if current.highLive {
			literal := k.branch(current.variable, decisionFalse, decisionTrue)
			contribution, err := k.apply(ctx, uint8(decisionAnd), true, current.reachableGuard, literal, decisionLeafAnd)
			if err != nil {
				return nil, err
			}
			if err := mergeGuard(current.high, contribution); err != nil {
				return nil, err
			}
		}
	}

	regions := make([]decisionLeafTupleRegion, 0)
	// Product nodes are interned in a deterministic low-then-high traversal:
	// child keys are considered in that fixed order and indices is lookup-only
	// (map iteration never influences creation).  Consequently node-index order
	// is a stable terminal emission order.  Consumers condition an ROBDD by each
	// region's exact care and are independent of the order among disjoint
	// terminals, so a second lexicographic sort of the materialized leaf rows is
	// both unnecessary and pure width*RlogR overhead.
	for index := range nodes {
		if !nodes[index].terminal {
			continue
		}
		if nodes[index].reachableGuard == decisionFalse {
			return nil, errDecisionMalformed
		}
		values, flattenErr := vectors.Flatten(nodes[index].key.values)
		if flattenErr != nil {
			return nil, flattenErr
		}
		leaves := make([]decisionLeaf, len(values))
		for valueIndex, value := range values {
			node := k.nodes[value]
			if !node.terminal {
				return nil, errDecisionMalformed
			}
			leaves[valueIndex] = node.leaf
		}
		regions = append(regions, decisionLeafTupleRegion{leaves: leaves, care: nodes[index].reachableGuard})
	}
	return regions, nil
}
