package diagram

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/scalar"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/terminal"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// SoleCombine gives one typed plane its sole point at which two supported
// terminal values meet.  Diagram supplies the exact support/FDD zipper; the
// callback supplies all lattice meaning, including the meaning of zero as the
// plane's declared Default.
type SoleCombine[K scalar.Key, V any] func(key K, left, right terminal.ID[V]) (terminal.ID[V], bool)

// SoleEqual is the typed terminal equivalence used only for operation-produced
// change evidence.  It must treat an undefined terminal as the Factor's
// declared Default; structural terminal identity alone is not semantic
// equality at that sparse boundary.
type SoleEqual[V any] func(left, right terminal.ID[V]) bool

// SoleChange receives one key's exact difference between the merge reference
// and the constructed output.  It is called from the same sparse/FDD zipper
// that constructs the output, before either root is published.  The typed
// Binding consumes K locally; Diagram never stores a dependency vocabulary.
type SoleChange[K scalar.Key] func(key K, region support.Mask) bool

// SoleRegions supplies key-local operand and reference coverage. It is used
// by the carrier contribution fold, where opaque authored Targets resolve to
// concrete keys only beside their typed Binding.
type SoleRegions[K scalar.Key] func(key K) (left, right, reference support.Mask, ok bool)

type soleMergeTriple[V any] struct {
	left, right                                 *node[V]
	leftSupport, rightSupport, referenceSupport support.Mask
}

type soleMergeResult[V any] struct {
	value   *node[V]
	changed support.Mask
}

type soleMergeFrame[V any] struct {
	triple soleMergeTriple[V]
	atom   guard.Atom
	low    soleMergeTriple[V]
	high   soleMergeTriple[V]
	phase  uint8
}

type soleOutput[K scalar.Key, V any] struct {
	key   K
	value *node[V]
}

// soleTreeFrame builds the final immutable AVL without putting each column
// through a logarithmic persistent update or using host recursion.
type soleTreeFrame[K scalar.Key, V any] struct {
	low, high    int
	parent       *keyNode[K, V]
	node         *keyNode[K, V]
	right, phase uint8
}

// MergeSoleFactor is the only structural binary construction for a typed
// sole-factor plane. It streams the ascending sparse key union once, and for
// each key synchronizes the two FDDs with the two existing shared supports.
// At a terminal the four support cases are exact: 00 drops, 10 carries left,
// 01 carries right, and 11 invokes combine. No column support, coordinate
// table, valuation, or recursive builder route is created.
//
// referenceSupport bounds the published-state predecessor (the left operand)
// on which output changes are observable. Carrier uses left support for
// Join/Widen and right support for Narrow, whose output support is right. A
// non-nil report therefore receives only regions where old left and output
// are both observable and differ. regions is the one carrier-owned candidate
// Boolean transaction shared by every Factor.
//
// scratch is caller-owned and reusable. It is intentionally required: a
// worker that performs repeated merges owns its own work storage rather than
// smuggling mutable scratch into a Domain or global pool.
func (builder *Builder[F, K, V]) MergeSoleFactor(left, right Root[F, K, V], leftSupport, rightSupport, referenceSupport support.Mask, scratch *SoleScratch[K, V], regions *support.Work, combine SoleCombine[K, V], equal SoleEqual[V], report SoleChange[K]) (Root[F, K, V], bool) {
	if builder == nil || !builder.open || builder.diagram == nil || !builder.diagram.Valid(left) || !builder.diagram.Valid(right) ||
		!leftSupport.Valid() || !rightSupport.Valid() || !referenceSupport.Valid() ||
		leftSupport.Manager() != builder.diagram.guards || rightSupport.Manager() != builder.diagram.guards || referenceSupport.Manager() != builder.diagram.guards ||
		scratch == nil || combine == nil || report != nil && (regions == nil || !regions.Open() || equal == nil) {
		return Root[F, K, V]{}, false
	}
	return builder.mergeSoleFactorRegions(left, right, scratch, regions, combine, equal, report, func(_ K) (support.Mask, support.Mask, support.Mask, bool) {
		return leftSupport, rightSupport, referenceSupport, true
	})
}

// MergeSoleFactorRegions is the target-local counterpart of MergeSoleFactor.
// It streams the same sparse key union, but obtains operand coverage only
// after the typed Binding resolves an opaque Target to the current key. Keys
// absent from both roots need no physical output: their explicit Default
// authorship remains in carrier coverage, not in a second fact plane.
func (builder *Builder[F, K, V]) MergeSoleFactorRegions(left, right Root[F, K, V], scratch *SoleScratch[K, V], regions *support.Work, combine SoleCombine[K, V], equal SoleEqual[V], report SoleChange[K], covers SoleRegions[K]) (Root[F, K, V], bool) {
	if builder == nil || !builder.open || builder.diagram == nil || !builder.diagram.Valid(left) || !builder.diagram.Valid(right) || scratch == nil || combine == nil || covers == nil || report != nil && (regions == nil || !regions.Open() || equal == nil) {
		return Root[F, K, V]{}, false
	}
	return builder.mergeSoleFactorRegions(left, right, scratch, regions, combine, equal, report, covers)
}

// MergeSoleFactorKey applies the same closed contribution algebra to one
// known changed key and persistently patches the left root. It is the sparse
// incremental counterpart of MergeSoleFactorRegions: callers must derive the
// key and exact right region from an owner-issued change proof, while Diagram
// retains sole ownership of FDD traversal and immutable AVL publication.
func (builder *Builder[F, K, V]) MergeSoleFactorKey(left, right Root[F, K, V], key K, leftSupport, rightSupport, referenceSupport support.Mask, scratch *SoleScratch[K, V], regions *support.Work, combine SoleCombine[K, V], equal SoleEqual[V]) (Root[F, K, V], support.Mask, bool) {
	if builder == nil || !builder.open || builder.diagram == nil || !builder.Valid(left) || !builder.Valid(right) ||
		!leftSupport.Valid() || !rightSupport.Valid() || !referenceSupport.Valid() ||
		leftSupport.Manager() != builder.diagram.guards || rightSupport.Manager() != builder.diagram.guards || referenceSupport.Manager() != builder.diagram.guards ||
		scratch == nil || regions == nil || !regions.Open() || combine == nil || equal == nil {
		return Root[F, K, V]{}, support.Mask{}, false
	}
	factor, ok := builder.diagram.SoleFactor()
	if !ok {
		return Root[F, K, V]{}, support.Mask{}, false
	}
	rank, ok := builder.diagram.ranks[factor]
	if !ok {
		return Root[F, K, V]{}, support.Mask{}, false
	}
	leftFactor, rightFactor := findFactor(left.root, rank), findFactor(right.root, rank)
	var leftValue, rightValue *node[V]
	if leftFactor != nil {
		leftValue = columnValue(leftFactor.keys, key)
	}
	if rightFactor != nil {
		rightValue = columnValue(rightFactor.keys, key)
	}
	value, changed, ok := builder.mergeSoleColumn(key, leftValue, rightValue, leftSupport, rightSupport, referenceSupport, scratch, regions, combine, equal)
	if !ok {
		return Root[F, K, V]{}, support.Mask{}, false
	}
	root, ok := builder.Put(left, factor, key, Value[V]{owner: builder.diagram.owner, node: value})
	if !ok {
		return Root[F, K, V]{}, support.Mask{}, false
	}
	return root, changed, true
}

func (builder *Builder[F, K, V]) mergeSoleFactorRegions(left, right Root[F, K, V], scratch *SoleScratch[K, V], regions *support.Work, combine SoleCombine[K, V], equal SoleEqual[V], report SoleChange[K], covers SoleRegions[K]) (Root[F, K, V], bool) {
	factor, ok := builder.diagram.SoleFactor()
	if !ok {
		return Root[F, K, V]{}, false
	}
	rank, ok := builder.diagram.ranks[factor]
	if !ok || !scratch.prepare(factorKeys(findFactor(left.root, rank)), factorKeys(findFactor(right.root, rank))) {
		return Root[F, K, V]{}, false
	}

	sameLeft, sameRight := true, true
	for {
		if !scratch.live() {
			return Root[F, K, V]{}, false
		}
		pair, present := scratch.nextPair()
		if !present {
			break
		}
		leftSupport, rightSupport, referenceSupport, covered := covers(pair.key)
		if !covered || !leftSupport.Valid() || !rightSupport.Valid() || !referenceSupport.Valid() || leftSupport.Manager() != builder.diagram.guards || rightSupport.Manager() != builder.diagram.guards || referenceSupport.Manager() != builder.diagram.guards {
			return Root[F, K, V]{}, false
		}
		value, changed, ok := builder.mergeSoleColumn(pair.key, pair.left, pair.right, leftSupport, rightSupport, referenceSupport, scratch, regions, combine, equal)
		if !ok {
			return Root[F, K, V]{}, false
		}
		if report != nil {
			view, valid := regions.Decompose(changed)
			if !valid {
				return Root[F, K, V]{}, false
			}
			if !view.Terminal || view.Value {
				if !report(pair.key, changed) {
					return Root[F, K, V]{}, false
				}
			}
		}
		if !sameSparseNode(pair.left, value) {
			sameLeft = false
			scratch.patches = append(scratch.patches, soleOutput[K, V]{key: pair.key, value: value})
		}
		if !sameSparseNode(pair.right, value) {
			sameRight = false
		}
		if !undefinedNode(value) {
			scratch.output = append(scratch.output, soleOutput[K, V]{key: pair.key, value: value})
		}
	}
	if sameLeft {
		return Root[F, K, V]{diagram: builder.diagram, root: left.root, count: left.count, lease: builder.lease}, true
	}
	if sameRight {
		return Root[F, K, V]{diagram: builder.diagram, root: right.root, count: right.count, lease: builder.lease}, true
	}
	if len(scratch.output) == 0 {
		return Root[F, K, V]{diagram: builder.diagram, lease: builder.lease}, true
	}
	// A changed sparse point normally touches only a few persistent columns.
	// Preserve the immutable AVL subtrees of the left predecessor in that case
	// instead of rebuilding every key node. The bulk builder remains cheaper
	// when a substantial fraction changed. This changes representation only;
	// the synchronized fold above already constructed the exact final columns.
	if len(scratch.patches) != 0 && len(scratch.patches)*16 < len(scratch.output) {
		keys := factorKeys(findFactor(left.root, rank))
		for _, patch := range scratch.patches {
			if undefinedNode(patch.value) {
				var removed bool
				keys, removed = deleteKey(keys, patch.key)
				if !removed {
					return Root[F, K, V]{}, false
				}
				continue
			}
			keys, _ = setKey(keys, patch.key, patch.value)
		}
		return Root[F, K, V]{
			diagram: builder.diagram,
			root:    makeFactor(factor, rank, keys, nil, nil),
			count:   len(scratch.output),
			lease:   builder.lease,
		}, true
	}
	keys := buildSoleKeys(scratch)
	return Root[F, K, V]{
		diagram: builder.diagram,
		root:    makeFactor(factor, rank, keys, nil, nil),
		count:   len(scratch.output),
		lease:   builder.lease,
	}, true
}

func (builder *Builder[F, K, V]) mergeSoleColumn(key K, left, right *node[V], leftSupport, rightSupport, referenceSupport support.Mask, scratch *SoleScratch[K, V], regions *support.Work, combine SoleCombine[K, V], equal SoleEqual[V]) (*node[V], support.Mask, bool) {
	clear(scratch.merge)
	clear(scratch.mergeFrames)
	scratch.mergeFrames = scratch.mergeFrames[:0]
	root := soleMergeTriple[V]{left: left, right: right, leftSupport: leftSupport, rightSupport: rightSupport, referenceSupport: referenceSupport}
	scratch.mergeFrames = append(scratch.mergeFrames, soleMergeFrame[V]{triple: root})
	for len(scratch.mergeFrames) != 0 {
		if !scratch.live() {
			return nil, support.Mask{}, false
		}
		index := len(scratch.mergeFrames) - 1
		frame := &scratch.mergeFrames[index]
		if _, found := scratch.merge[frame.triple]; found {
			scratch.mergeFrames = scratch.mergeFrames[:index]
			continue
		}
		switch frame.phase {
		case 0:
			complete, value, changed, ok := builder.mergeSoleLeaf(key, frame.triple, regions, combine, equal)
			if !ok {
				return nil, support.Mask{}, false
			}
			if complete {
				if scratch.merge == nil {
					scratch.merge = make(map[soleMergeTriple[V]]soleMergeResult[V])
				}
				scratch.merge[frame.triple] = soleMergeResult[V]{value: value, changed: changed}
				scratch.mergeFrames = scratch.mergeFrames[:index]
				continue
			}
			atom, low, high, ok := builder.mergeSoleBranches(frame.triple)
			if !ok {
				return nil, support.Mask{}, false
			}
			frame.atom, frame.low, frame.high, frame.phase = atom, low, high, 1
			scratch.mergeFrames = append(scratch.mergeFrames, soleMergeFrame[V]{triple: low})
		case 1:
			frame.phase = 2
			scratch.mergeFrames = append(scratch.mergeFrames, soleMergeFrame[V]{triple: frame.high})
		default:
			low, lowOK := scratch.merge[frame.low]
			high, highOK := scratch.merge[frame.high]
			if !lowOK || !highOK {
				return nil, support.Mask{}, false
			}
			output := builder.decision(frame.atom, low.value, high.value)
			if sameDecision(frame.triple.left, frame.atom, low.value, high.value) {
				output = frame.triple.left
			} else if sameDecision(frame.triple.right, frame.atom, low.value, high.value) {
				output = frame.triple.right
			}
			var changed support.Mask
			if regions != nil {
				var changedOK bool
				changed, changedOK = regions.Decision(frame.atom, low.changed, high.changed)
				if !changedOK {
					return nil, support.Mask{}, false
				}
			}
			scratch.merge[frame.triple] = soleMergeResult[V]{value: output, changed: changed}
			scratch.mergeFrames = scratch.mergeFrames[:index]
		}
	}
	value, ok := scratch.merge[root]
	return value.value, value.changed, ok
}

// mergeSoleLeaf resolves a completed support cell. The direct carry cases
// retain immutable predecessor FDDs verbatim, including their terminal IDs.
// When regions is present it also derives the old-left-to-output delta for
// this cell.  The comparison is only meaningful under referenceSupport;
// right-only Join/Widen support and removed Narrow support are reported by
// carrier's Added/Removed masks rather than fake unit changes.
func (builder *Builder[F, K, V]) mergeSoleLeaf(key K, triple soleMergeTriple[V], regions *support.Work, combine SoleCombine[K, V], equal SoleEqual[V]) (complete bool, value *node[V], changed support.Mask, ok bool) {
	leftView, leftOK := triple.leftSupport.Decompose()
	rightView, rightOK := triple.rightSupport.Decompose()
	referenceView, referenceOK := triple.referenceSupport.Decompose()
	if !leftOK || !rightOK || !referenceOK {
		return false, nil, support.Mask{}, false
	}
	if leftView.Terminal && rightView.Terminal {
		var output *node[V]
		switch {
		case !leftView.Value && !rightView.Value:
			output = builder.terminal(terminal.ID[V]{})
		case leftView.Value && !rightView.Value:
			if triple.left == nil {
				output = builder.terminal(terminal.ID[V]{})
			} else {
				output = triple.left
			}
		case !leftView.Value && rightView.Value:
			if triple.right == nil {
				output = builder.terminal(terminal.ID[V]{})
			} else {
				output = triple.right
			}
		default:
			// Both inputs are supported. Their FDDs may still branch, so the
			// synchronized traversal below must reach a terminal pair before
			// invoking the typed operation.
			output = nil
		}
		if output != nil {
			if regions == nil {
				return true, output, support.Mask{}, true
			}
			return true, output, regions.False(), true
		}
	}
	leftRank, leftOK := builder.diagram.nodeRank(triple.left)
	rightRank, rightOK := builder.diagram.nodeRank(triple.right)
	leftSupportRank, leftSupportOK := builder.diagram.regionRank(leftView)
	rightSupportRank, rightSupportOK := builder.diagram.regionRank(rightView)
	referenceSupportRank, referenceSupportOK := builder.diagram.regionRank(referenceView)
	if !leftOK || !rightOK || !leftSupportOK || !rightSupportOK || !referenceSupportOK {
		return false, nil, support.Mask{}, false
	}
	if minimumMergeFive(leftRank, rightRank, leftSupportRank, rightSupportRank, referenceSupportRank) != noRelationRank {
		return false, nil, support.Mask{}, true
	}
	leftID, leftValid := builder.diagram.terminalAt(triple.left)
	rightID, rightValid := builder.diagram.terminalAt(triple.right)
	if !leftValid || !rightValid || !leftView.Terminal || !rightView.Terminal || !leftView.Value || !rightView.Value {
		return false, nil, support.Mask{}, false
	}
	merged, accepted := combine(key, leftID, rightID)
	if !accepted || !builder.validTerminal(merged) {
		return false, nil, support.Mask{}, false
	}
	output := builder.terminal(merged)
	if triple.left != nil && triple.left.terminal && triple.left.value == merged {
		output = triple.left
	} else if triple.right != nil && triple.right.terminal && triple.right.value == merged {
		output = triple.right
	}
	if regions == nil {
		return true, output, support.Mask{}, true
	}
	changed = regions.False()
	if referenceView.Value {
		referenceID, referenceValid := builder.diagram.terminalAt(triple.left)
		outputID, outputValid := builder.terminalAt(output)
		if !referenceValid || !outputValid {
			return false, nil, support.Mask{}, false
		}
		if !equal(referenceID, outputID) {
			changed = regions.True()
		}
	}
	return true, output, changed, true
}

// terminalAt reads an output made by this Builder. Unlike Diagram.terminalAt,
// it accepts a terminal admitted by the current candidate terminal Work; the
// merge delta must compare that output before Builder.Seal publishes it.
func (builder *Builder[F, K, V]) terminalAt(value *node[V]) (terminal.ID[V], bool) {
	if value == nil {
		return terminal.ID[V]{}, true
	}
	if !value.terminal {
		return terminal.ID[V]{}, false
	}
	return value.value, builder.validTerminal(value.value)
}

func (builder *Builder[F, K, V]) mergeSoleBranches(triple soleMergeTriple[V]) (guard.Atom, soleMergeTriple[V], soleMergeTriple[V], bool) {
	leftView, leftOK := triple.leftSupport.Decompose()
	rightView, rightOK := triple.rightSupport.Decompose()
	referenceView, referenceOK := triple.referenceSupport.Decompose()
	if !leftOK || !rightOK || !referenceOK {
		return 0, soleMergeTriple[V]{}, soleMergeTriple[V]{}, false
	}
	leftRank, leftOK := builder.diagram.nodeRank(triple.left)
	rightRank, rightOK := builder.diagram.nodeRank(triple.right)
	leftSupportRank, leftSupportOK := builder.diagram.regionRank(leftView)
	rightSupportRank, rightSupportOK := builder.diagram.regionRank(rightView)
	referenceSupportRank, referenceSupportOK := builder.diagram.regionRank(referenceView)
	if !leftOK || !rightOK || !leftSupportOK || !rightSupportOK || !referenceSupportOK {
		return 0, soleMergeTriple[V]{}, soleMergeTriple[V]{}, false
	}
	rank := minimumMergeFive(leftRank, rightRank, leftSupportRank, rightSupportRank, referenceSupportRank)
	if rank == noRelationRank {
		return 0, soleMergeTriple[V]{}, soleMergeTriple[V]{}, false
	}
	var atom guard.Atom
	switch {
	case leftSupportRank == rank:
		atom = leftView.Atom
	case rightSupportRank == rank:
		atom = rightView.Atom
	case referenceSupportRank == rank:
		atom = referenceView.Atom
	case leftRank == rank:
		atom = triple.left.atom
	default:
		atom = triple.right.atom
	}
	lowLeftSupport, highLeftSupport := triple.leftSupport, triple.leftSupport
	if leftSupportRank == rank {
		lowLeftSupport, highLeftSupport = leftView.Low, leftView.High
	}
	lowRightSupport, highRightSupport := triple.rightSupport, triple.rightSupport
	if rightSupportRank == rank {
		lowRightSupport, highRightSupport = rightView.Low, rightView.High
	}
	lowReferenceSupport, highReferenceSupport := triple.referenceSupport, triple.referenceSupport
	if referenceSupportRank == rank {
		lowReferenceSupport, highReferenceSupport = referenceView.Low, referenceView.High
	}
	return atom,
		soleMergeTriple[V]{left: branchNode(triple.left, leftRank, rank, false), right: branchNode(triple.right, rightRank, rank, false), leftSupport: lowLeftSupport, rightSupport: lowRightSupport, referenceSupport: lowReferenceSupport},
		soleMergeTriple[V]{left: branchNode(triple.left, leftRank, rank, true), right: branchNode(triple.right, rightRank, rank, true), leftSupport: highLeftSupport, rightSupport: highRightSupport, referenceSupport: highReferenceSupport},
		true
}

func minimumMergeRank(first, second, third, fourth uint64) uint64 {
	return minimumRank(minimumRank(first, second, third), fourth, noRelationRank)
}

func minimumMergeFive(first, second, third, fourth, fifth uint64) uint64 {
	return minimumRank(minimumMergeRank(first, second, third, fourth), fifth, noRelationRank)
}

func undefinedNode[V any](value *node[V]) bool {
	return value == nil || value.terminal && value.value == (terminal.ID[V]{})
}

func sameSparseNode[V any](stored, output *node[V]) bool {
	return stored == output || stored == nil && undefinedNode(output)
}

func sameDecision[V any](candidate *node[V], atom guard.Atom, low, high *node[V]) bool {
	return candidate != nil && !candidate.terminal && candidate.atom == atom && candidate.low == low && candidate.high == high
}

func buildSoleKeys[K scalar.Key, V any](scratch *SoleScratch[K, V]) *keyNode[K, V] {
	if scratch == nil || len(scratch.output) == 0 {
		return nil
	}
	clear(scratch.treeFrames)
	scratch.treeFrames = append(scratch.treeFrames[:0], soleTreeFrame[K, V]{low: 0, high: len(scratch.output)})
	var root *keyNode[K, V]
	for len(scratch.treeFrames) != 0 {
		index := len(scratch.treeFrames) - 1
		frame := &scratch.treeFrames[index]
		switch frame.phase {
		case 0:
			if frame.low >= frame.high {
				scratch.treeFrames = scratch.treeFrames[:index]
				continue
			}
			middle := frame.low + (frame.high-frame.low)/2
			entry := scratch.output[middle]
			frame.node = &keyNode[K, V]{key: entry.key, value: entry.value}
			if frame.parent == nil {
				root = frame.node
			} else if frame.right != 0 {
				frame.parent.right = frame.node
			} else {
				frame.parent.left = frame.node
			}
			frame.phase = 1
			scratch.treeFrames = append(scratch.treeFrames, soleTreeFrame[K, V]{low: frame.low, high: middle, parent: frame.node})
		case 1:
			middle := frame.low + (frame.high-frame.low)/2
			frame.phase = 2
			scratch.treeFrames = append(scratch.treeFrames, soleTreeFrame[K, V]{low: middle + 1, high: frame.high, parent: frame.node, right: 1})
		default:
			frame.node.height = keyHeight(frame.node.left)
			if rightHeight := keyHeight(frame.node.right); rightHeight > frame.node.height {
				frame.node.height = rightHeight
			}
			frame.node.height++
			scratch.treeFrames = scratch.treeFrames[:index]
		}
	}
	return root
}
