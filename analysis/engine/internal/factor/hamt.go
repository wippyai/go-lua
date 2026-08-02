package factor

import (
	"math/bits"

	"github.com/wippyai/go-lua/analysis/lattice"
)

type nodeReader[V any] interface {
	node(ref) (*node, bool)
	entryPage(ref) (*entryPage[V], bool)
	childPage(ref) (*childPage, bool)
}

type operation uint8

const (
	joinOperation operation = iota + 1
	widenOperation
	narrowOperation
)

func slot(hash uint64, depth uint8) uint8 {
	return uint8((hash >> (depth * radixBits)) & radixMask)
}

func empty[V any](reader nodeReader[V], ref ref) bool {
	if ref.zero() {
		return true
	}
	node, valid := reader.node(ref)
	return valid && node.kind == emptyNode
}

func lookup[K ~uint64, V any](reader nodeReader[V], root ref, key K) (V, bool) {
	item, present := lookupEntry(reader, root, key)
	if !present {
		var zero V
		return zero, false
	}
	return item.value, true
}

// mixKey is the version-pinned bijection used by every direct-key HAMT. It
// is computed once per raw key operation and threaded through descent. Each
// xor-shift and odd multiplication is bijective on uint64, so distinct direct
// keys cannot share a complete hash or require a collision leaf.
func mixKey[K ~uint64](key K) uint64 {
	value := uint64(key)
	value ^= value >> 30
	value *= 0xbf58476d1ce4e5b9
	value ^= value >> 27
	value *= 0x94d049bb133111eb
	value ^= value >> 31
	return value
}

func compareKey[K ~uint64](left, right K) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

func lookupEntry[K ~uint64, V any](reader nodeReader[V], root ref, key K) (entry[V], bool) {
	return lookupEntryHash(reader, root, key, mixKey(key))
}

func lookupEntryHash[K ~uint64, V any](reader nodeReader[V], root ref, key K, hash uint64) (entry[V], bool) {
	ref := root
	for depth := uint8(0); ; depth++ {
		if ref.zero() {
			return entry[V]{}, false
		}
		node, valid := reader.node(ref)
		if !valid || node.kind == emptyNode {
			return entry[V]{}, false
		}
		switch node.kind {
		case leafNode:
			if node.hash != hash {
				return entry[V]{}, false
			}
			item, valid := entryOf(reader, node.entry)
			return item, valid && K(item.key) == key
		case branchNode:
			if depth >= hashLevels {
				return entry[V]{}, false
			}
			child, present := branchChild(reader, node, uint32(1)<<slot(hash, depth))
			if !present {
				return entry[V]{}, false
			}
			ref = child
		default:
			return entry[V]{}, false
		}
	}
}

// walk is iterative: traversal size never consumes the Go call stack.
func walk[K ~uint64, V any](reader nodeReader[V], root ref, visit func(K, V) bool) bool {
	if empty(reader, root) {
		return true
	}
	stack := []ref{root}
	for len(stack) != 0 {
		last := len(stack) - 1
		ref := stack[last]
		stack = stack[:last]
		node, valid := reader.node(ref)
		if !valid {
			return false
		}
		switch node.kind {
		case leafNode:
			item, valid := entryOf(reader, node.entry)
			if !valid || !visit(K(item.key), item.value) {
				return false
			}
		case branchNode:
			for index := node.children.count - 1; index >= 0; index-- {
				child, valid := childAt(reader, node.children, index)
				if !valid {
					return false
				}
				stack = append(stack, child)
			}
		default:
			return false
		}
	}
	return true
}

// replaceChanges computes the sparse symmetric value difference between two
// exact roots. It deliberately returns right unchanged to the caller; this
// traversal is only the reverse-read delta. It compares the two HAMTs
// directly, skipping identical subtrees and walking each unmatched branch
// exactly once. No map, sort, or root reconstruction participates.
//
// Every stored key is non-default by Arena construction, so a key present on
// only the left is precisely stored→default and one present only on the right
// is precisely default→stored. Independently-built semantic equals produce
// no changes because values remain the authority.
func (work *Work[K, V]) replaceChanges(left, right ref, changes *Changes[K]) bool {
	if work == nil || !work.Valid(Root[K, V]{ref: left}) || !work.Valid(Root[K, V]{ref: right}) {
		return false
	}
	stack := work.replaceStack[:0]
	defer func() {
		clear(stack[:cap(stack)])
		work.replaceStack = stack[:0]
	}()
	stack = append(stack, replaceFrame{left: left, right: right})
	for len(stack) != 0 {
		last := len(stack) - 1
		frame := stack[last]
		stack = stack[:last]
		if frame.left == frame.right {
			continue
		}
		leftEmpty := empty(work, frame.left)
		rightEmpty := empty(work, frame.right)
		if leftEmpty && rightEmpty {
			continue
		}
		if leftEmpty {
			if !work.replacePushRight(&stack, frame.right, frame.depth, changes) {
				return false
			}
			continue
		}
		if rightEmpty {
			if !work.replacePushLeft(&stack, frame.left, frame.depth, changes) {
				return false
			}
			continue
		}
		leftNode, leftOK := work.node(frame.left)
		rightNode, rightOK := work.node(frame.right)
		if !leftOK || !rightOK {
			return false
		}
		switch {
		case leftNode.kind == leafNode && rightNode.kind == leafNode:
			leftEntry, leftValid := entryOf(work, leftNode.entry)
			rightEntry, rightValid := entryOf(work, rightNode.entry)
			if !leftValid || !rightValid {
				return false
			}
			if leftNode.hash == rightNode.hash {
				if leftEntry.key != rightEntry.key {
					return false
				}
				if !work.arena.sameValue(leftEntry.value, rightEntry.value) && !work.changed(K(leftEntry.key), changes) {
					return false
				}
				continue
			}
			if !work.changed(K(leftEntry.key), changes) || !work.changed(K(rightEntry.key), changes) {
				return false
			}
		case leftNode.kind == branchNode && rightNode.kind == branchNode:
			if frame.depth >= hashLevels || !work.replacePushBranches(&stack, leftNode, rightNode, frame.depth) {
				return false
			}
		case leftNode.kind == leafNode && rightNode.kind == branchNode:
			if frame.depth >= hashLevels || !work.replacePushLeafBranch(&stack, frame.left, leftNode, rightNode, frame.depth, true) {
				return false
			}
		case leftNode.kind == branchNode && rightNode.kind == leafNode:
			if frame.depth >= hashLevels || !work.replacePushLeafBranch(&stack, frame.right, rightNode, leftNode, frame.depth, false) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// replacePushLeft emits every leaf reachable only from left.  Its result is
// the destination default, so every stored key is one exact removal.
func (work *Work[K, V]) replacePushLeft(stack *[]replaceFrame, root ref, depth uint8, changes *Changes[K]) bool {
	node, valid := work.node(root)
	if !valid {
		return false
	}
	switch node.kind {
	case leafNode:
		entry, valid := entryOf(work, node.entry)
		return valid && work.changed(K(entry.key), changes)
	case branchNode:
		if depth >= hashLevels {
			return false
		}
		for index := node.children.count - 1; index >= 0; index-- {
			child, valid := childAt(work, node.children, index)
			if !valid {
				return false
			}
			*stack = append(*stack, replaceFrame{left: child, depth: depth + 1})
		}
		return true
	default:
		return false
	}
}

// replacePushRight emits every leaf reachable only from right. Its left value
// is the destination default, so every stored key is one exact addition.
func (work *Work[K, V]) replacePushRight(stack *[]replaceFrame, root ref, depth uint8, changes *Changes[K]) bool {
	node, valid := work.node(root)
	if !valid {
		return false
	}
	switch node.kind {
	case leafNode:
		entry, valid := entryOf(work, node.entry)
		return valid && work.changed(K(entry.key), changes)
	case branchNode:
		if depth >= hashLevels {
			return false
		}
		for index := node.children.count - 1; index >= 0; index-- {
			child, valid := childAt(work, node.children, index)
			if !valid {
				return false
			}
			*stack = append(*stack, replaceFrame{right: child, depth: depth + 1})
		}
		return true
	default:
		return false
	}
}

func (work *Work[K, V]) replacePushBranches(stack *[]replaceFrame, left, right *node, depth uint8) bool {
	for index := int(radixMask); index >= 0; index-- {
		bit := uint32(1) << uint8(index)
		leftChild, leftPresent := branchChild(work, left, bit)
		rightChild, rightPresent := branchChild(work, right, bit)
		if left.bitmap&bit != 0 && !leftPresent || right.bitmap&bit != 0 && !rightPresent {
			return false
		}
		if !leftPresent && !rightPresent {
			continue
		}
		*stack = append(*stack, replaceFrame{left: leftChild, right: rightChild, depth: depth + 1})
	}
	return true
}

// replacePushLeafBranch pairs the leaf with the one branch path selected by
// its immutable hash. All other branch children are one-sided differences.
// leafIsLeft preserves the destination-left orientation in each emitted
// frame; it is not an operator order.
func (work *Work[K, V]) replacePushLeafBranch(stack *[]replaceFrame, leaf ref, leafNode, branchNode *node, depth uint8, leafIsLeft bool) bool {
	target := slot(leafNode.hash, depth)
	for index := int(radixMask); index >= 0; index-- {
		bit := uint32(1) << uint8(index)
		child, present := branchChild(work, branchNode, bit)
		if branchNode.bitmap&bit != 0 && !present {
			return false
		}
		if uint8(index) == target {
			if leafIsLeft {
				*stack = append(*stack, replaceFrame{left: leaf, right: child, depth: depth + 1})
			} else {
				*stack = append(*stack, replaceFrame{left: child, right: leaf, depth: depth + 1})
			}
			continue
		}
		if !present {
			continue
		}
		if leafIsLeft {
			*stack = append(*stack, replaceFrame{right: child, depth: depth + 1})
		} else {
			*stack = append(*stack, replaceFrame{left: child, depth: depth + 1})
		}
	}
	return true
}

// equalRoots compares two complete sparse Factors pointwise. It is shared by
// Arena and its one active Work so semantic equality never depends on whether
// a root has crossed the publication boundary. Both callers establish their
// own capability authority before entering this traversal.
//
// The traversal is deliberately iterative. Equal roots may have different
// physical histories, but their explicit entry counts and every direct-key
// lattice value must agree.
func equalRoots[K ~uint64, V any](reader nodeReader[V], values lattice.Lattice[V], left, right ref) (bool, bool) {
	leftNode, leftValid := reader.node(left)
	rightNode, rightValid := reader.node(right)
	if !leftValid || !rightValid {
		return false, false
	}
	if left == right {
		return true, true
	}
	if leftNode.count != rightNode.count {
		return false, true
	}
	equal := true
	completed := walk(reader, left, func(key K, value V) bool {
		other, present := lookup(reader, right, key)
		if !present || !sameWith(values, value, other) {
			equal = false
			return false
		}
		return true
	})
	if !completed && equal {
		return false, false
	}
	return equal, true
}

// lessOrEqualRoots compares total sparse maps under the Factor-declared
// default. Walking both explicit supports is necessary because that default is
// not required to be lattice bottom: a key missing on one side still carries a
// semantic value. Both traversals are iterative through walk.
func lessOrEqualRoots[K ~uint64, V any](reader nodeReader[V], values lattice.Lattice[V], defaultValue V, left, right ref) (bool, bool) {
	if _, valid := reader.node(left); !valid {
		return false, false
	}
	if _, valid := reader.node(right); !valid {
		return false, false
	}
	if left == right {
		return true, true
	}
	ordered := true
	checkLeft := walk(reader, left, func(key K, value V) bool {
		other, present := lookup(reader, right, key)
		if !present {
			other = defaultValue
		}
		if !values.LessOrEq(value, other) {
			ordered = false
			return false
		}
		return true
	})
	if !checkLeft && ordered {
		return false, false
	}
	if !ordered {
		return false, true
	}
	checkRight := walk(reader, right, func(key K, value V) bool {
		if _, present := lookup(reader, left, key); present {
			return true
		}
		if !values.LessOrEq(defaultValue, value) {
			ordered = false
			return false
		}
		return true
	})
	if !checkRight && ordered {
		return false, false
	}
	return ordered, true
}

// rootFingerprint is constant-time for an empty or branch root. Branch
// summaries reuse node.hash, which is otherwise meaningful only to leaf
// routing; leaf summaries are computed directly from their one entry. XOR is
// associative and order-independent, so persistent insertion histories do
// not affect the result. Collisions are expressly allowed.
func rootFingerprint[K ~uint64, V any](reader nodeReader[V], fingerprint func(V) uint64, root ref) (uint64, bool) {
	if root.zero() {
		return 0, true
	}
	node, valid := reader.node(root)
	if !valid {
		return 0, false
	}
	switch node.kind {
	case emptyNode:
		return 0, true
	case leafNode:
		item, valid := entryOf(reader, node.entry)
		if !valid {
			return 0, false
		}
		return entryFingerprint(node.hash, fingerprint(item.value)), true
	case branchNode:
		return node.hash, true
	default:
		return 0, false
	}
}

func entryFingerprint(keyHash, value uint64) uint64 {
	return keyHash ^ bits.RotateLeft64(mixKey(value), 1)
}

func (work *Work[K, V]) apply(key K, op operation, left, right V) (V, bool) {
	var result V
	switch op {
	case joinOperation:
		return work.arena.values.Join(left, right), true
	case widenOperation:
		// Widen is a Mu-only operation. An unranked Factor may have been
		// declared for an acyclic equation, but it has no authority to widen.
		// The compiler rejects such a cyclic tuple before execution; retain this
		// check as the typed storage boundary so a future caller cannot bypass
		// that proof obligation.
		if !work.arena.widenRank.valid() {
			work.fail()
			return result, false
		}
		result = work.arena.values.Widen(left, right)
		if !work.arena.values.LessOrEq(left, result) || !work.arena.values.LessOrEq(right, result) ||
			(!work.arena.sameValue(left, result) && !work.arena.widenRank.descends(key, left, result)) {
			work.fail()
			return result, false
		}
		return result, true
	case narrowOperation:
		if work.arena.values.Narrow == nil {
			work.fail()
			return result, false
		}
		result = work.arena.values.Narrow(left, right)
		if !work.arena.values.LessOrEq(right, result) || !work.arena.values.LessOrEq(result, left) ||
			(!work.arena.sameValue(left, result) && !work.arena.narrowRank.descends(key, left, result)) {
			work.fail()
			return result, false
		}
		return result, true
	default:
		work.fail()
		return result, false
	}
}

func (work *Work[K, V]) set(root ref, key K, hash uint64, value V, depth uint8) (ref, bool) {
	if empty(work, root) {
		return work.newLeaf(hash, entry[V]{key: uint64(key), value: value})
	}
	node, valid := work.node(root)
	if !valid {
		return ref{}, false
	}
	if node.kind == leafNode {
		if node.hash == hash {
			item, valid := entryOf(work, node.entry)
			if !valid || K(item.key) != key {
				work.fail()
				return ref{}, false
			}
			if work.arena.sameValue(item.value, value) {
				return root, true
			}
			return work.newLeaf(hash, entry[V]{key: uint64(key), value: value})
		}
		other, valid := work.newLeaf(hash, entry[V]{key: uint64(key), value: value})
		if !valid {
			return ref{}, false
		}
		return work.mergeLeaves(root, other, depth)
	}
	if node.kind != branchNode || depth >= hashLevels {
		return ref{}, false
	}
	bit := uint32(1) << slot(hash, depth)
	offset := bits.OnesCount32(node.bitmap & (bit - 1))
	if node.bitmap&bit == 0 {
		child, valid := work.newLeaf(hash, entry[V]{key: uint64(key), value: value})
		if !valid {
			return ref{}, false
		}
		return work.branchInsert(node, bit, offset, child)
	}
	child, valid := childAt(work, node.children, offset)
	if !valid {
		return ref{}, false
	}
	updated, valid := work.set(child, key, hash, value, depth+1)
	if !valid || updated == child {
		return updated, valid
	}
	return work.branchReplace(node, offset, updated)
}

func (work *Work[K, V]) remove(root ref, key K, hash uint64, depth uint8) (ref, bool) {
	if empty(work, root) {
		return ref{}, true
	}
	node, valid := work.node(root)
	if !valid {
		return ref{}, false
	}
	if node.kind == leafNode {
		if node.hash != hash {
			return root, true
		}
		item, valid := entryOf(work, node.entry)
		if !valid {
			return ref{}, false
		}
		if K(item.key) == key {
			return ref{}, true
		}
		return root, true
	}
	if node.kind != branchNode || depth >= hashLevels {
		return ref{}, false
	}
	bit := uint32(1) << slot(hash, depth)
	if node.bitmap&bit == 0 {
		return root, true
	}
	offset := bits.OnesCount32(node.bitmap & (bit - 1))
	child, valid := childAt(work, node.children, offset)
	if !valid {
		return ref{}, false
	}
	updated, valid := work.remove(child, key, hash, depth+1)
	if !valid || updated == child {
		return updated, valid
	}
	if updated.zero() {
		return work.branchRemove(node, bit, offset)
	}
	return work.branchReplace(node, offset, updated)
}

func (work *Work[K, V]) newLeaf(hash uint64, item entry[V]) (ref, bool) {
	stored, valid := work.allocateEntry(item)
	if !valid {
		return ref{}, false
	}
	return work.allocateNode(node{kind: leafNode, hash: hash, entry: stored, count: 1})
}

func (work *Work[K, V]) newBranch(bitmap uint32, children *[1 << radixBits]ref, childCount int) (ref, bool) {
	if childCount == 0 {
		return ref{}, true
	}
	if childCount == 1 {
		only, valid := work.node(children[0])
		if !valid {
			return ref{}, false
		}
		if only.kind == leafNode {
			return children[0], true
		}
	}
	count := 0
	fingerprint := uint64(0)
	for index := 0; index < childCount; index++ {
		childNode, valid := work.node(children[index])
		if !valid {
			return ref{}, false
		}
		count += childNode.count
		childFingerprint, valid := rootFingerprint[K, V](work, work.arena.fingerprint, children[index])
		if !valid {
			return ref{}, false
		}
		fingerprint ^= childFingerprint
	}
	packed, valid := work.reserveChildren(childCount)
	if !valid {
		return ref{}, false
	}
	for index := 0; index < childCount; index++ {
		if !work.writeChild(packed, index, children[index]) {
			return ref{}, false
		}
	}
	return work.allocateNode(node{kind: branchNode, hash: fingerprint, bitmap: bitmap, children: packed, count: count})
}

func (work *Work[K, V]) branchInsert(branch *node, bit uint32, offset int, child ref) (ref, bool) {
	var children [1 << radixBits]ref
	for index := 0; index < branch.children.count+1; index++ {
		if index == offset {
			children[index] = child
			continue
		}
		source := index
		if index > offset {
			source--
		}
		current, valid := childAt(work, branch.children, source)
		if !valid {
			return ref{}, false
		}
		children[index] = current
	}
	return work.newBranch(branch.bitmap|bit, &children, branch.children.count+1)
}

func (work *Work[K, V]) branchReplace(branch *node, offset int, child ref) (ref, bool) {
	var children [1 << radixBits]ref
	for index := 0; index < branch.children.count; index++ {
		if index == offset {
			children[index] = child
			continue
		}
		current, valid := childAt(work, branch.children, index)
		if !valid {
			return ref{}, false
		}
		children[index] = current
	}
	return work.newBranch(branch.bitmap, &children, branch.children.count)
}

func (work *Work[K, V]) branchRemove(branch *node, bit uint32, offset int) (ref, bool) {
	var children [1 << radixBits]ref
	for index := 0; index < branch.children.count-1; index++ {
		source := index
		if index >= offset {
			source++
		}
		current, valid := childAt(work, branch.children, source)
		if !valid {
			return ref{}, false
		}
		children[index] = current
	}
	return work.newBranch(branch.bitmap&^bit, &children, branch.children.count-1)
}

func (work *Work[K, V]) mergeLeaves(left, right ref, depth uint8) (ref, bool) {
	leftNode, leftValid := work.node(left)
	rightNode, rightValid := work.node(right)
	if !leftValid || !rightValid || leftNode.kind != leafNode || rightNode.kind != leafNode || depth >= hashLevels {
		return ref{}, false
	}
	leftSlot, rightSlot := slot(leftNode.hash, depth), slot(rightNode.hash, depth)
	if leftSlot == rightSlot {
		child, valid := work.mergeLeaves(left, right, depth+1)
		if !valid {
			return ref{}, false
		}
		var children [1 << radixBits]ref
		children[0] = child
		return work.newBranch(uint32(1)<<leftSlot, &children, 1)
	}
	var children [1 << radixBits]ref
	if leftSlot < rightSlot {
		children[0], children[1] = left, right
	} else {
		children[0], children[1] = right, left
	}
	return work.newBranch(uint32(1)<<leftSlot|uint32(1)<<rightSlot, &children, 2)
}

func (work *Work[K, V]) combine(left, right ref, depth uint8, op operation, changes *Changes[K]) (ref, bool) {
	if empty(work, left) {
		return work.transform(right, depth, op, false, changes)
	}
	if empty(work, right) {
		return work.transform(left, depth, op, true, changes)
	}
	leftNode, leftValid := work.node(left)
	rightNode, rightValid := work.node(right)
	if !leftValid || !rightValid {
		return ref{}, false
	}
	switch {
	case leftNode.kind == leafNode && rightNode.kind == leafNode:
		if leftNode.hash == rightNode.hash {
			return work.combineLeaf(left, right, leftNode, rightNode, op, changes)
		}
		leftNext, valid := work.transform(left, depth, op, true, changes)
		if !valid {
			return ref{}, false
		}
		rightNext, valid := work.transform(right, depth, op, false, changes)
		if !valid {
			return ref{}, false
		}
		if leftNext.zero() {
			return rightNext, true
		}
		if rightNext.zero() {
			return leftNext, true
		}
		return work.mergeLeaves(leftNext, rightNext, depth)
	case leftNode.kind == branchNode && rightNode.kind == branchNode:
		return work.combineBranches(left, right, leftNode, rightNode, depth, op, changes)
	case leftNode.kind == leafNode && rightNode.kind == branchNode:
		return work.combineLeafBranch(left, right, leftNode, rightNode, depth, op, true, changes)
	case leftNode.kind == branchNode && rightNode.kind == leafNode:
		return work.combineLeafBranch(right, left, rightNode, leftNode, depth, op, false, changes)
	default:
		return ref{}, false
	}
}

func (work *Work[K, V]) transform(root ref, depth uint8, op operation, leftSide bool, changes *Changes[K]) (ref, bool) {
	if empty(work, root) {
		return ref{}, true
	}
	node, valid := work.node(root)
	if !valid {
		return ref{}, false
	}
	if node.kind == leafNode {
		item, valid := entryOf(work, node.entry)
		if !valid {
			return ref{}, false
		}
		key := K(item.key)
		result, valid := work.applyDefault(key, op, item.value, leftSide)
		if !valid {
			return ref{}, false
		}
		if work.arena.sameValue(result, item.value) {
			// Reusing a right-only subtree is a storage optimization, never a
			// statement that the destination-left map is unchanged.  In
			// particular, Join(empty, right) often returns right verbatim; every
			// non-default right key still changed relative to the left default and
			// must be delivered to exact reverse readers.  The left-side case is
			// unchanged by definition, because item is already the old value.
			if !leftSide && !work.arena.sameValue(result, work.arena.defaultValue) && !work.changed(key, changes) {
				return ref{}, false
			}
			return root, true
		}
		if !work.changed(key, changes) {
			return ref{}, false
		}
		if work.arena.isDefault(result) {
			return ref{}, true
		}
		return work.newLeaf(node.hash, entry[V]{key: item.key, value: result})
	}
	if node.kind != branchNode || depth >= hashLevels {
		return ref{}, false
	}
	var output [1 << radixBits]ref
	bitmap := uint32(0)
	unchanged := true
	for slotIndex := uint8(0); slotIndex < 1<<radixBits; slotIndex++ {
		bit := uint32(1) << slotIndex
		if node.bitmap&bit == 0 {
			continue
		}
		child, valid := branchChild(work, node, bit)
		if !valid {
			return ref{}, false
		}
		updated, valid := work.transform(child, depth+1, op, leftSide, changes)
		if !valid {
			return ref{}, false
		}
		output[slotIndex] = updated
		if updated != child {
			unchanged = false
		}
		if !updated.zero() {
			bitmap |= bit
		}
	}
	if unchanged {
		return root, true
	}
	return work.branchFromSlots(bitmap, &output)
}

func (work *Work[K, V]) applyDefault(key K, op operation, value V, leftSide bool) (V, bool) {
	if leftSide {
		return work.apply(key, op, value, work.arena.defaultValue)
	}
	return work.apply(key, op, work.arena.defaultValue, value)
}

func (work *Work[K, V]) combineLeaf(left, right ref, leftNode, rightNode *node, op operation, changes *Changes[K]) (ref, bool) {
	leftEntry, leftValid := entryOf(work, leftNode.entry)
	rightEntry, rightValid := entryOf(work, rightNode.entry)
	if !leftValid || !rightValid || leftEntry.key != rightEntry.key {
		work.fail()
		return ref{}, false
	}
	key := K(leftEntry.key)
	result, valid := work.apply(key, op, leftEntry.value, rightEntry.value)
	if !valid {
		return ref{}, false
	}
	if work.arena.sameValue(result, leftEntry.value) {
		return left, true
	}
	if !work.changed(key, changes) {
		return ref{}, false
	}
	if work.arena.isDefault(result) {
		return ref{}, true
	}
	if work.arena.sameValue(result, rightEntry.value) {
		return right, true
	}
	return work.newLeaf(leftNode.hash, entry[V]{key: leftEntry.key, value: result})
}

func (work *Work[K, V]) combineBranches(left, right ref, leftNode, rightNode *node, depth uint8, op operation, changes *Changes[K]) (ref, bool) {
	var output [1 << radixBits]ref
	bitmap := uint32(0)
	unchanged := true
	for slotIndex := uint8(0); slotIndex < 1<<radixBits; slotIndex++ {
		bit := uint32(1) << slotIndex
		leftChild, leftPresent := branchChild(work, leftNode, bit)
		rightChild, rightPresent := branchChild(work, rightNode, bit)
		var result ref
		var valid bool
		switch {
		case leftPresent && rightPresent:
			result, valid = work.combine(leftChild, rightChild, depth+1, op, changes)
		case leftPresent:
			result, valid = work.transform(leftChild, depth+1, op, true, changes)
		case rightPresent:
			result, valid = work.transform(rightChild, depth+1, op, false, changes)
		default:
			continue
		}
		if !valid {
			return ref{}, false
		}
		output[slotIndex] = result
		if result != leftChild {
			unchanged = false
		}
		if !result.zero() {
			bitmap |= bit
		}
	}
	if unchanged && bitmap == leftNode.bitmap {
		return left, true
	}
	return work.branchFromSlots(bitmap, &output)
}

func (work *Work[K, V]) combineLeafBranch(leaf, branch ref, leafNode, branchNode *node, depth uint8, op operation, leafIsLeft bool, changes *Changes[K]) (ref, bool) {
	if depth >= hashLevels {
		return ref{}, false
	}
	target := slot(leafNode.hash, depth)
	var output [1 << radixBits]ref
	bitmap := uint32(0)
	branchUnchanged := true
	for slotIndex := uint8(0); slotIndex < 1<<radixBits; slotIndex++ {
		bit := uint32(1) << slotIndex
		branchChildRef, branchPresent := branchChild(work, branchNode, bit)
		var result ref
		var valid bool
		if slotIndex == target {
			switch {
			case branchPresent && leafIsLeft:
				result, valid = work.combine(leaf, branchChildRef, depth+1, op, changes)
			case branchPresent:
				result, valid = work.combine(branchChildRef, leaf, depth+1, op, changes)
			case leafIsLeft:
				result, valid = work.transform(leaf, depth+1, op, true, changes)
			default:
				result, valid = work.transform(leaf, depth+1, op, false, changes)
			}
		} else if branchPresent {
			result, valid = work.transform(branchChildRef, depth+1, op, !leafIsLeft, changes)
		} else {
			continue
		}
		if !valid {
			return ref{}, false
		}
		output[slotIndex] = result
		if result != branchChildRef {
			branchUnchanged = false
		}
		if !result.zero() {
			bitmap |= bit
		}
	}
	if !leafIsLeft && branchUnchanged && bitmap == branchNode.bitmap {
		return branch, true
	}
	if leafIsLeft && bitmap == uint32(1)<<target && output[target] == leaf {
		return leaf, true
	}
	return work.branchFromSlots(bitmap, &output)
}

func branchChild[V any](reader nodeReader[V], branch *node, bit uint32) (ref, bool) {
	if branch.bitmap&bit == 0 {
		return ref{}, false
	}
	return childAt(reader, branch.children, bits.OnesCount32(branch.bitmap&(bit-1)))
}

func (work *Work[K, V]) branchFromSlots(bitmap uint32, slots *[1 << radixBits]ref) (ref, bool) {
	var children [1 << radixBits]ref
	count := 0
	for slotIndex := uint8(0); slotIndex < 1<<radixBits; slotIndex++ {
		if bitmap&(uint32(1)<<slotIndex) != 0 {
			children[count] = slots[slotIndex]
			count++
		}
	}
	return work.newBranch(bitmap, &children, count)
}

func (work *Work[K, V]) changed(key K, changes *Changes[K]) bool {
	if changes == nil {
		return true
	}
	if changes.append(Location[K]{owner: work.arena.owner, key: key}) {
		return true
	}
	work.fail()
	return false
}
