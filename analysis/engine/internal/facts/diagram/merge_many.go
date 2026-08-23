package diagram

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/scalar"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/terminal"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// SoleManyCombine resolves one completed guarded cell from the contributions
// present at it. The vector holds each distinct present contribution once and
// is never empty: a cell no operand covers is Absent and never reaches
// combine, while a covered sparse zero appears as the zero terminal ID and
// denotes Present(Default). The operation must therefore be an idempotent,
// commutative and associative join over that set - a set is all the diagram
// preserves. The slice is borrowed scratch and must not be retained.
type SoleManyCombine[K scalar.Key, V any] func(key K, values []terminal.ID[V]) (terminal.ID[V], bool)

// SoleManyRegions fills one exact key-local authored/physical region per
// operand. The output slice is pre-sized to the input root vector and is
// borrowed only for this call.
type SoleManyRegions[K scalar.Key] func(key K, output []support.Mask) bool

// MergeSoleFactorMany performs one synchronized fold over a vector of
// immutable sole-factor roots. It streams the sparse key union once,
// traverses every operand FDD and presence mask together, and constructs one
// final root. Typed meaning remains in combine; Diagram admits no terminal
// and owns no presence relation.
//
// The fused traversal is a join in the distributive lattice of guarded
// contributions, so its cost is a property of the operands and the result,
// never of the product of the operand region spaces. Two positions that hold
// the same contribution set and the same still-undecided operands are one
// state, whichever operands produced that set and in whatever order: an
// operand whose region is empty at a position leaves it, and an operand whose
// region covers it with an already-resolved value collapses into its
// contribution set.
//
// previous is a reconstruction predecessor: it contributes no terminal or
// authored presence to the per-column fold. Its sparse keys join the outer
// traversal so stale predecessor-only columns are deleted, and its immutable
// nodes are seeded into this Builder's unique tables so an unchanged cell
// republishes the predecessor node. Exact no-ops and untouched AVL subtrees
// remain pointer-shared. Cancellation is sampled independently of zipper
// exhaustion and once more before every successful return.
func (builder *Builder[F, K, V]) MergeSoleFactorMany(previous Root[F, K, V], inputs []Root[F, K, V], scratch *SoleScratch[K, V], regions *support.Work, combine SoleManyCombine[K, V], covers SoleManyRegions[K]) (Root[F, K, V], bool) {
	if builder == nil || !builder.open || builder.diagram == nil || len(inputs) == 0 || scratch == nil || regions == nil || !regions.Open() || combine == nil || covers == nil || !builder.diagram.Valid(previous) {
		return Root[F, K, V]{}, false
	}
	for _, input := range inputs {
		if !builder.diagram.Valid(input) {
			return Root[F, K, V]{}, false
		}
	}
	factor, sole := builder.diagram.SoleFactor()
	rank, ranked := builder.diagram.ranks[factor]
	if !sole || !ranked {
		return Root[F, K, V]{}, false
	}
	// The caller owns the operation scratch. Keep the key-root view there so
	// every fold reuses its backing storage and no diagram-local wrapper vector
	// survives the publication transaction.
	scratch.clearManyWork()
	roots := scratch.borrowManyRootNodes(len(inputs) + 1)
	roots[0] = factorKeys(findFactor(previous.root, rank))
	for index, input := range inputs {
		roots[index+1] = factorKeys(findFactor(input.root, rank))
	}
	defer scratch.Clear()
	if !scratch.prepareMany(roots) {
		return Root[F, K, V]{}, false
	}
	// Cursor width includes the reconstruction-only predecessor; semantic
	// support/terminal vectors contain only the real fold operands.
	scratch.manySupports = resizeClear(scratch.manySupports, len(inputs))

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
		if !builder.seedSoleManyPredecessor(columns[0], scratch) {
			return Root[F, K, V]{}, false
		}
		value, ok := builder.mergeSoleManyColumn(key, columns[1:], scratch.manySupports, scratch, regions, combine)
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
		return Root[F, K, V]{diagram: builder.diagram, root: previous.root, count: previous.count, lease: builder.token}, true
	}
	keys, ok := applySolePatches(baseKeys, scratch.patches, scratch.live)
	if !ok || !scratch.live() {
		return Root[F, K, V]{}, false
	}
	count := previous.count + delta
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
	return Root[F, K, V]{diagram: builder.diagram, root: root, count: count, lease: builder.token}, true
}

// seedSoleManyPredecessor imports one immutable predecessor column into the
// Builder's local terminal/decision tables, so a fold cell that resolves to a
// terminal the predecessor already carries republishes the predecessor node.
// The iterative postorder walk is intentionally separate from the fused tuple
// state: predecessor structure never affects state keys, ranks, cofactors, or
// presence.
func (builder *Builder[F, K, V]) seedSoleManyPredecessor(root *node[V], scratch *SoleScratch[K, V]) bool {
	if builder == nil || !builder.open || scratch == nil || !scratch.live() {
		return false
	}
	scratch.clearManySeed()
	if scratch.manySeedState == nil {
		scratch.manySeedState = make(map[*node[V]]uint8)
	}
	if root == nil {
		return true
	}
	scratch.manySeedStack = append(scratch.manySeedStack, soleManySeedFrame[V]{node: root})
	for len(scratch.manySeedStack) != 0 {
		if !scratch.live() {
			return false
		}
		index := len(scratch.manySeedStack) - 1
		frame := &scratch.manySeedStack[index]
		if frame.node == nil {
			return false
		}
		switch frame.phase {
		case 0:
			if state := scratch.manySeedState[frame.node]; state == 2 {
				scratch.manySeedStack = scratch.manySeedStack[:index]
				continue
			} else if state != 0 {
				return false
			}
			scratch.manySeedState[frame.node] = 1
			if frame.node.terminal {
				if !builder.validTerminal(frame.node.value) {
					return false
				}
				builder.terminals[frame.node.value] = frame.node
				builder.imports[frame.node] = frame.node
				scratch.manySeedState[frame.node] = 2
				scratch.manySeedStack = scratch.manySeedStack[:index]
				continue
			}
			frame.phase = 1
			// Process children sequentially. A legal reduced FDD is a DAG, so a
			// suffix reachable from both children must be black before the second
			// edge sees it; pushing both children would mistake that sharing for a
			// gray cycle.
			scratch.manySeedStack = append(scratch.manySeedStack, soleManySeedFrame[V]{node: frame.node.low})
		case 1:
			if _, lowOK := builder.imports[frame.node.low]; !lowOK {
				return false
			}
			frame.phase = 2
			scratch.manySeedStack = append(scratch.manySeedStack, soleManySeedFrame[V]{node: frame.node.high})
		case 2:
			low, lowOK := builder.imports[frame.node.low]
			high, highOK := builder.imports[frame.node.high]
			if !lowOK || !highOK {
				return false
			}
			if low == frame.node.low && high == frame.node.high {
				builder.decisions[decisionKey[V]{atom: frame.node.atom, low: low, high: high}] = frame.node
				builder.imports[frame.node] = frame.node
			} else {
				builder.imports[frame.node] = builder.decision(frame.node.atom, low, high)
			}
			scratch.manySeedState[frame.node] = 2
			scratch.manySeedStack = scratch.manySeedStack[:index]
		default:
			return false
		}
	}
	return true
}

func (builder *Builder[F, K, V]) mergeSoleManyColumn(key K, nodes []*node[V], supports []support.Mask, scratch *SoleScratch[K, V], regions *support.Work, combine SoleManyCombine[K, V]) (*node[V], bool) {
	if len(nodes) == 0 || len(nodes) != len(supports) {
		return nil, false
	}
	scratch.clearManyStates()
	root, ok := builder.soleManyState(scratch, regions, nodes, supports, noSoleManyState)
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
			if low == high {
				output = low
			} else {
				for index := 0; index < state.width; index++ {
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

// soleManyState classifies one raw operand tuple against the contribution set
// it inherits and interns the resulting canonical state. Classification is
// what keeps the fold's live state proportional to its result: an operand
// whose region is empty here can never contribute below this position and is
// dropped, and an operand whose region covers this position with a resolved
// value contributes exactly that value everywhere below, so it is absorbed
// into the contribution set instead of remaining a coordinate to expand.
func (builder *Builder[F, K, V]) soleManyState(scratch *SoleScratch[K, V], regions *support.Work, nodes []*node[V], supports []support.Mask, state int) (int, bool) {
	if builder == nil || builder.diagram == nil || scratch == nil || len(nodes) != len(supports) {
		return 0, false
	}
	if !scratch.inheritMany(state) {
		return 0, false
	}
	for index := range nodes {
		view, valid := regions.Decompose(supports[index])
		if !valid {
			return 0, false
		}
		switch {
		case view.Terminal && !view.Value:
		case view.Terminal && (nodes[index] == nil || nodes[index].terminal):
			id, resolved := builder.diagram.terminalAt(nodes[index])
			if !resolved {
				return 0, false
			}
			scratch.settleMany(id, nodes[index])
		default:
			scratch.manyLaneNodes = append(scratch.manyLaneNodes, nodes[index])
			scratch.manyLaneSupports = append(scratch.manyLaneSupports, supports[index])
			scratch.manyLaneViews = append(scratch.manyLaneViews, view)
		}
	}
	return scratch.internManyState()
}

func (builder *Builder[F, K, V]) analyzeSoleManyState(key K, stateIndex int, scratch *SoleScratch[K, V], regions *support.Work, combine SoleManyCombine[K, V]) (complete bool, output *node[V], atom guard.Atom, low, high int, ok bool) {
	state := scratch.manyStates[stateIndex]
	if state.offset < 0 || state.width < 0 || state.offset+state.width > len(scratch.manyTupleNodes) || state.offset+state.width > len(scratch.manyTupleSupport) ||
		state.settledStart < 0 || state.settledCount < 0 || state.settledStart+state.settledCount > len(scratch.manySettledIDs) || state.settledStart+state.settledCount > len(scratch.manySettledNodes) {
		return false, nil, 0, 0, 0, false
	}
	settledIDs := scratch.manySettledIDs[state.settledStart : state.settledStart+state.settledCount]
	settledLeaves := scratch.manySettledNodes[state.settledStart : state.settledStart+state.settledCount]
	if state.width == 0 {
		if state.settledCount == 0 {
			result, valid := builder.canonicalSoleManyTerminal(settledLeaves, terminal.ID[V]{})
			return true, result, 0, 0, 0, valid
		}
		// Even one contribution goes through the typed chooser. That is what
		// gives complementary sparse-default/equal-terminal leaves one physical
		// terminal pointer and lets ordinary FDD reduction collapse them without
		// a separate semantic node-equality pass.
		merged, accepted := combine(key, settledIDs)
		if !accepted || !builder.validTerminal(merged) {
			return false, nil, 0, 0, 0, false
		}
		result, valid := builder.canonicalSoleManyTerminal(settledLeaves, merged)
		return true, result, 0, 0, 0, valid
	}
	width := state.width
	if state.offset+width > len(scratch.manyTupleView) {
		return false, nil, 0, 0, 0, false
	}
	nodes := scratch.manyTupleNodes[state.offset : state.offset+width]
	supports := scratch.manyTupleSupport[state.offset : state.offset+width]
	views := scratch.manyTupleView[state.offset : state.offset+width]
	scratch.manyRegionRanks = resizeClear(scratch.manyRegionRanks, width)
	scratch.manyNodeRanks = resizeClear(scratch.manyNodeRanks, width)
	scratch.manyLowNodes = resizeClear(scratch.manyLowNodes, width)
	scratch.manyHighNodes = resizeClear(scratch.manyHighNodes, width)
	scratch.manyLowSupports = resizeClear(scratch.manyLowSupports, width)
	scratch.manyHighSupports = resizeClear(scratch.manyHighSupports, width)
	regionRanks := scratch.manyRegionRanks
	nodeRanks := scratch.manyNodeRanks
	minRank := noRelationRank
	for index := 0; index < width; index++ {
		regionRank := noRelationRank
		if !views[index].Terminal {
			rank, ranked := builder.diagram.regionRank(views[index])
			if !ranked {
				return false, nil, 0, 0, 0, false
			}
			regionRank = rank
			if rank < minRank {
				minRank = rank
			}
		}
		rank, ranked := builder.diagram.nodeRank(nodes[index])
		if !ranked {
			return false, nil, 0, 0, 0, false
		}
		regionRanks[index], nodeRanks[index] = regionRank, rank
		if rank < minRank {
			minRank = rank
		}
	}
	if minRank == noRelationRank {
		return false, nil, 0, 0, 0, false
	}
	var selected guard.Atom
	selectedSet := false
	for index := 0; index < width && !selectedSet; index++ {
		switch {
		case regionRanks[index] == minRank:
			selected, selectedSet = views[index].Atom, true
		case nodeRanks[index] == minRank:
			selected, selectedSet = nodes[index].atom, true
		}
	}
	if !selectedSet {
		return false, nil, 0, 0, 0, false
	}
	for index := 0; index < width; index++ {
		scratch.manyLowSupports[index], scratch.manyHighSupports[index] = supports[index], supports[index]
		if regionRanks[index] == minRank {
			scratch.manyLowSupports[index], scratch.manyHighSupports[index] = views[index].Low, views[index].High
		}
		scratch.manyLowNodes[index] = branchNode(nodes[index], nodeRanks[index], minRank, false)
		scratch.manyHighNodes[index] = branchNode(nodes[index], nodeRanks[index], minRank, true)
	}
	lowIndex, valid := builder.soleManyState(scratch, regions, scratch.manyLowNodes, scratch.manyLowSupports, stateIndex)
	if !valid {
		return false, nil, 0, 0, 0, false
	}
	highIndex, valid := builder.soleManyState(scratch, regions, scratch.manyHighNodes, scratch.manyHighSupports, stateIndex)
	if !valid {
		return false, nil, 0, 0, 0, false
	}
	return false, nil, selected, lowIndex, highIndex, true
}

// canonicalSoleManyTerminal registers the chosen final terminal exactly once
// in this Builder. An immutable operand leaf is adopted only if no chosen node
// is registered yet; every later branch therefore gets the same physical
// pointer for the same chosen ID. This is representational reuse only: the
// semantic layer selected `id` after its fixed-order fold.
func (builder *Builder[F, K, V]) canonicalSoleManyTerminal(nodes []*node[V], id terminal.ID[V]) (*node[V], bool) {
	if builder == nil || !builder.open || !builder.validTerminal(id) {
		return nil, false
	}
	if cached := builder.terminals[id]; cached != nil {
		return cached, true
	}
	for _, candidate := range nodes {
		if candidate != nil && candidate.terminal && candidate.value == id {
			builder.terminals[id] = candidate
			return candidate, true
		}
	}
	return builder.terminal(id), true
}
