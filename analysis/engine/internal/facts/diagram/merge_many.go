package diagram

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/scalar"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/terminal"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// SoleManyCombine resolves one completed guarded cell across a fixed ordered
// operand vector. present distinguishes Absent from Present(Default), whose
// sparse terminal ID is also zero. reference is reconstruction-only: it may
// be reused only after the already-computed aggregate is equal, and must never
// participate in the fold itself. The slices are borrowed scratch and must
// not be retained by the callback.
type SoleManyCombine[K scalar.Key, V any] func(key K, reference terminal.ID[V], values []terminal.ID[V], present []bool) (terminal.ID[V], bool)

// SoleManyRegions fills one exact key-local authored/physical region per
// operand. The output slice is pre-sized to the input root vector and is
// borrowed only for this call.
type SoleManyRegions[K scalar.Key] func(key K, output []support.Mask) bool

// MergeSoleFactorMany performs one synchronized fixed-order fold over a
// vector of immutable sole-factor roots. It streams the sparse key union once,
// traverses every operand FDD and presence mask together, and constructs one
// final root. Typed meaning remains in combine; Diagram admits no terminal
// and owns no presence relation.
//
// reference is a reconstruction predecessor only: it contributes no terminal
// or authored presence to combine. Its sparse keys join the traversal so stale
// predecessor-only columns are deleted, while exact no-ops and untouched AVL
// subtrees remain pointer-shared. Cancellation is sampled independently of
// zipper exhaustion and once more before every successful return.
func (builder *Builder[F, K, V]) MergeSoleFactorMany(reference Root[F, K, V], inputs []Root[F, K, V], scratch *SoleScratch[K, V], regions *support.Work, combine SoleManyCombine[K, V], covers SoleManyRegions[K]) (Root[F, K, V], bool) {
	if builder == nil || !builder.open || builder.diagram == nil || len(inputs) == 0 || scratch == nil || regions == nil || !regions.Open() || combine == nil || covers == nil || !builder.diagram.Valid(reference) {
		return Root[F, K, V]{}, false
	}
	factor, sole := builder.diagram.SoleFactor()
	rank, ranked := builder.diagram.ranks[factor]
	if !sole || !ranked {
		return Root[F, K, V]{}, false
	}
	roots := make([]*keyNode[K, V], len(inputs)+1)
	roots[0] = factorKeys(findFactor(reference.root, rank))
	for index, input := range inputs {
		if !builder.diagram.Valid(input) {
			return Root[F, K, V]{}, false
		}
		roots[index+1] = factorKeys(findFactor(input.root, rank))
	}
	if !scratch.prepareMany(roots) {
		return Root[F, K, V]{}, false
	}
	// Cursor width includes the reconstruction-only reference; semantic
	// support/terminal vectors contain only the real fold operands.
	scratch.manySupports = resizeClear(scratch.manySupports, len(inputs))
	defer scratch.Clear()

	baseKeys := roots[0]
	delta := 0
	for {
		if !scratch.live() {
			return Root[F, K, V]{}, false
		}
		key, columns, present := scratch.nextMany()
		if !present {
			break
		}
		clear(scratch.manySupports)
		if !covers(key, scratch.manySupports) {
			return Root[F, K, V]{}, false
		}
		for _, region := range scratch.manySupports {
			if !validSoleSupport(builder.diagram.guards, regions, region) {
				return Root[F, K, V]{}, false
			}
		}
		if len(columns) != len(inputs)+1 {
			return Root[F, K, V]{}, false
		}
		value, ok := builder.mergeSoleManyColumn(key, columns[0], columns[1:], scratch.manySupports, scratch, regions, combine)
		if !ok {
			return Root[F, K, V]{}, false
		}
		base := columns[0]
		if sameSparseNode(base, value) {
			continue
		}
		scratch.patches = append(scratch.patches, soleOutput[K, V]{key: key, value: value})
		switch {
		case undefinedNode(base) && !undefinedNode(value):
			delta++
		case !undefinedNode(base) && undefinedNode(value):
			delta--
		}
	}
	if !scratch.live() {
		return Root[F, K, V]{}, false
	}
	if len(scratch.patches) == 0 {
		return Root[F, K, V]{diagram: builder.diagram, root: reference.root, count: reference.count, lease: builder.lease}, true
	}
	keys, ok := applySolePatches(baseKeys, scratch.patches, scratch.live)
	if !ok || !scratch.live() {
		return Root[F, K, V]{}, false
	}
	count := reference.count + delta
	if count < 0 || (keys == nil) != (count == 0) {
		return Root[F, K, V]{}, false
	}
	var root *factorNode[F, K, V]
	if keys != nil {
		root = makeFactor(factor, rank, keys, nil, nil)
	}
	if !scratch.live() {
		return Root[F, K, V]{}, false
	}
	return Root[F, K, V]{diagram: builder.diagram, root: root, count: count, lease: builder.lease}, true
}

func (builder *Builder[F, K, V]) mergeSoleManyColumn(key K, reference *node[V], nodes []*node[V], supports []support.Mask, scratch *SoleScratch[K, V], regions *support.Work, combine SoleManyCombine[K, V]) (*node[V], bool) {
	if len(nodes) == 0 || len(nodes) != len(supports) {
		return nil, false
	}
	scratch.clearManyStates()
	scratch.manyWidth = len(nodes)
	scratch.manyPresent = resizeClear(scratch.manyPresent, scratch.manyWidth)
	scratch.manyIDs = resizeClear(scratch.manyIDs, scratch.manyWidth)
	scratch.manyLowNodes = resizeClear(scratch.manyLowNodes, scratch.manyWidth)
	scratch.manyHighNodes = resizeClear(scratch.manyHighNodes, scratch.manyWidth)
	scratch.manyLowSupports = resizeClear(scratch.manyLowSupports, scratch.manyWidth)
	scratch.manyHighSupports = resizeClear(scratch.manyHighSupports, scratch.manyWidth)
	root, ok := scratch.manyState(reference, nodes, supports)
	if !ok {
		return nil, false
	}
	scratch.manyStack = append(scratch.manyStack, root)
	for len(scratch.manyStack) != 0 {
		if !scratch.live() {
			return nil, false
		}
		stackIndex := len(scratch.manyStack) - 1
		stateIndex := scratch.manyStack[stackIndex]
		state := &scratch.manyStates[stateIndex]
		if state.result != nil {
			scratch.manyStack[stackIndex] = 0
			scratch.manyStack = scratch.manyStack[:stackIndex]
			continue
		}
		switch state.phase {
		case 0:
			complete, output, atom, low, high, valid := builder.analyzeSoleManyState(key, stateIndex, scratch, regions, combine)
			if !valid {
				return nil, false
			}
			// analyze may append child states and grow the backing slice; never
			// retain a pointer across that operation.
			state = &scratch.manyStates[stateIndex]
			if complete {
				state.result = output
				continue
			}
			state.atom, state.low, state.high, state.phase = atom, low, high, 1
			if scratch.manyStates[low].result == nil {
				scratch.manyStack = append(scratch.manyStack, low)
			}
		case 1:
			state.phase = 2
			if scratch.manyStates[state.high].result == nil {
				scratch.manyStack = append(scratch.manyStack, state.high)
			}
		default:
			low, high := scratch.manyStates[state.low].result, scratch.manyStates[state.high].result
			if low == nil || high == nil {
				return nil, false
			}
			var output *node[V]
			reference := state.reference
			if low == high {
				output = low
			} else if sameDecision(reference, state.atom, low, high) {
				// The reference is never an operand: this is solely a physical
				// reuse proof after the output's fixed-order fold is complete.
				output = reference
			} else {
				width := len(nodes)
				for index := 0; index < width; index++ {
					candidate := scratch.manyTupleNodes[state.offset+index]
					if sameDecision(candidate, state.atom, low, high) {
						output = candidate
						break
					}
				}
			}
			if output == nil {
				output = builder.decision(state.atom, low, high)
			}
			state.result = output
		}
	}
	result := scratch.manyStates[root].result
	return result, result != nil && scratch.live()
}

func (builder *Builder[F, K, V]) analyzeSoleManyState(key K, stateIndex int, scratch *SoleScratch[K, V], regions *support.Work, combine SoleManyCombine[K, V]) (complete bool, output *node[V], atom guard.Atom, low, high int, ok bool) {
	state := scratch.manyStates[stateIndex]
	width := scratch.manyWidth
	if width == 0 || state.offset < 0 || state.offset+width > len(scratch.manyTupleNodes) || state.offset+width > len(scratch.manyTupleSupport) {
		return false, nil, 0, 0, 0, false
	}
	nodes := scratch.manyTupleNodes[state.offset : state.offset+width]
	supports := scratch.manyTupleSupport[state.offset : state.offset+width]
	minRank := noRelationRank
	active := 0
	allSupportTerminal, allActiveTerminal := true, true
	for index := 0; index < width; index++ {
		view, valid := regions.Decompose(supports[index])
		if !valid {
			return false, nil, 0, 0, 0, false
		}
		if !view.Terminal {
			allSupportTerminal = false
			rank, ranked := builder.diagram.regionRank(view)
			if !ranked {
				return false, nil, 0, 0, 0, false
			}
			if rank < minRank {
				minRank = rank
			}
		} else if view.Value {
			active++
			if nodes[index] != nil && !nodes[index].terminal {
				allActiveTerminal = false
			}
		}
		if !view.Terminal || view.Value {
			rank, ranked := builder.diagram.nodeRank(nodes[index])
			if !ranked {
				return false, nil, 0, 0, 0, false
			}
			if rank < minRank {
				minRank = rank
			}
		}
	}
	referenceID, referenceTerminal := builder.terminalAt(state.reference)
	if allSupportTerminal && referenceTerminal {
		switch {
		case active == 0:
			output, valid := builder.canonicalSoleManyTerminal(state.reference, nodes, terminal.ID[V]{})
			return true, output, 0, 0, 0, valid
		case allActiveTerminal:
			// Even one active operand goes through the typed chooser. That is
			// what gives complementary sparse-default/equal-terminal leaves one
			// physical terminal pointer and lets ordinary FDD reduction collapse
			// them without a separate semantic node-equality pass.
			clear(scratch.manyPresent)
			for index := 0; index < width; index++ {
				view, valid := regions.Decompose(supports[index])
				if !valid || !view.Terminal {
					return false, nil, 0, 0, 0, false
				}
				scratch.manyPresent[index] = view.Value
			}
			clear(scratch.manyIDs)
			for index := 0; index < width; index++ {
				if !scratch.manyPresent[index] {
					continue
				}
				id, valid := builder.diagram.terminalAt(nodes[index])
				if !valid {
					return false, nil, 0, 0, 0, false
				}
				scratch.manyIDs[index] = id
			}
			merged, accepted := combine(key, referenceID, scratch.manyIDs, scratch.manyPresent)
			if !accepted || !builder.validTerminal(merged) {
				return false, nil, 0, 0, 0, false
			}
			output, valid := builder.canonicalSoleManyTerminal(state.reference, nodes, merged)
			return true, output, 0, 0, 0, valid
		}
	}
	referenceRank, referenceRanked := builder.diagram.nodeRank(state.reference)
	if !referenceRanked {
		return false, nil, 0, 0, 0, false
	}
	if referenceRank < minRank {
		minRank = referenceRank
	}
	if minRank == noRelationRank {
		return false, nil, 0, 0, 0, false
	}
	var selected guard.Atom
	selectedSet := false
	for index := 0; index < width && !selectedSet; index++ {
		view, valid := regions.Decompose(supports[index])
		if !valid {
			return false, nil, 0, 0, 0, false
		}
		regionRank, regionOK := builder.diagram.regionRank(view)
		nodeRank, nodeOK := builder.diagram.nodeRank(nodes[index])
		if !regionOK || !nodeOK {
			return false, nil, 0, 0, 0, false
		}
		switch {
		case regionRank == minRank:
			selected, selectedSet = view.Atom, true
		case nodeRank == minRank:
			selected, selectedSet = nodes[index].atom, true
		}
	}
	if !selectedSet && referenceRank == minRank && state.reference != nil && !state.reference.terminal {
		selected, selectedSet = state.reference.atom, true
	}
	if !selectedSet {
		return false, nil, 0, 0, 0, false
	}
	for index := 0; index < width; index++ {
		view, valid := regions.Decompose(supports[index])
		if !valid {
			return false, nil, 0, 0, 0, false
		}
		regionRank, regionOK := builder.diagram.regionRank(view)
		nodeRank, nodeOK := builder.diagram.nodeRank(nodes[index])
		if !regionOK || !nodeOK {
			return false, nil, 0, 0, 0, false
		}
		scratch.manyLowSupports[index], scratch.manyHighSupports[index] = supports[index], supports[index]
		if regionRank == minRank {
			scratch.manyLowSupports[index], scratch.manyHighSupports[index] = view.Low, view.High
		}
		scratch.manyLowNodes[index] = branchNode(nodes[index], nodeRank, minRank, false)
		scratch.manyHighNodes[index] = branchNode(nodes[index], nodeRank, minRank, true)
	}
	lowReference := branchNode(state.reference, referenceRank, minRank, false)
	highReference := branchNode(state.reference, referenceRank, minRank, true)
	lowIndex, valid := scratch.manyState(lowReference, scratch.manyLowNodes, scratch.manyLowSupports)
	if !valid {
		return false, nil, 0, 0, 0, false
	}
	highIndex, valid := scratch.manyState(highReference, scratch.manyHighNodes, scratch.manyHighSupports)
	if !valid {
		return false, nil, 0, 0, 0, false
	}
	return false, nil, selected, lowIndex, highIndex, true
}

// canonicalSoleManyTerminal registers the chosen final terminal exactly once
// in this Builder. An immutable reference or operand leaf is adopted only if
// no chosen node is registered yet; every later branch therefore gets the
// same physical pointer for the same chosen ID. This is representational
// reuse only: the semantic layer selected `id` after its fixed-order fold.
func (builder *Builder[F, K, V]) canonicalSoleManyTerminal(reference *node[V], nodes []*node[V], id terminal.ID[V]) (*node[V], bool) {
	if builder == nil || !builder.open || !builder.validTerminal(id) {
		return nil, false
	}
	if cached := builder.terminals[id]; cached != nil {
		return cached, true
	}
	if reference != nil && reference.terminal && reference.value == id {
		builder.terminals[id] = reference
		return reference, true
	}
	for _, candidate := range nodes {
		if candidate != nil && candidate.terminal && candidate.value == id {
			builder.terminals[id] = candidate
			return candidate, true
		}
	}
	return builder.terminal(id), true
}
