package sequence

import (
	"math/bits"

	"github.com/wippyai/go-lua/analysis/internal/hash"
)

// closedWord is an immutable exact word. Its root is a height-balanced rope:
// concatenation preserves both inputs and copies only a logarithmic spine.
// length is duplicated at the carrier boundary because many mode operations
// need it without first checking a nil root.
type closedWord struct {
	root   *closedNode
	length int
}

// closedNode is private representation only. A leaf has no children and is
// either a copied flat run or a repeated handle run; a branch has cached
// logical length and AVL height. Flat data is never exposed mutably.
type closedNode struct {
	left, right *closedNode
	flat        []Handle
	repeat      Handle
	count       int
	length      int
	height      uint8
}

func closedWordFromFlat(values []Handle) closedWord {
	flat := copyHandles(values)
	return closedWordFromLeaf(closedFlatLeaf(flat))
}

// closedWordFromSharedFlat is limited to internally owned immutable slices.
// It is used after public constructors have copied their input.
func closedWordFromSharedFlat(values []Handle) closedWord {
	return closedWordFromLeaf(closedFlatLeaf(values))
}

func closedWordFromLeaf(leaf *closedNode) closedWord {
	if leaf == nil {
		return closedWord{}
	}
	return closedWord{root: leaf, length: leaf.length}
}

func closedFlatLeaf(values []Handle) *closedNode {
	if len(values) == 0 {
		return nil
	}
	return &closedNode{flat: values, length: len(values), height: 1}
}

func closedRepeatLeaf(value Handle, count int) *closedNode {
	if count <= 0 {
		return nil
	}
	return &closedNode{repeat: value, count: count, length: count, height: 1}
}

func (word closedWord) copy() closedWord { return word }

func (word closedWord) At(index int) (Handle, bool) {
	if index < 0 || index >= word.length {
		return Handle{}, false
	}
	for node := word.root; node != nil; {
		if node.left == nil && node.right == nil {
			if len(node.flat) != 0 {
				return node.flat[index], true
			}
			return node.repeat, true
		}
		if index < node.left.length {
			node = node.left
		} else {
			index -= node.left.length
			node = node.right
		}
	}
	return Handle{}, false
}

func (word closedWord) Materialize() []Handle {
	if word.length == 0 {
		return nil
	}
	result := make([]Handle, 0, word.length)
	iterator := newClosedIterator(word)
	for {
		value, ok := iterator.Next()
		if !ok {
			return result
		}
		result = append(result, value)
	}
}

func (word closedWord) equal(labels Labels, other closedWord) bool {
	if word.length != other.length {
		return false
	}
	left, right := newClosedIterator(word), newClosedIterator(other)
	for {
		leftValue, leftOK := left.Next()
		rightValue, rightOK := right.Next()
		if leftOK != rightOK {
			return false
		}
		if !leftOK {
			return true
		}
		if !labels.Equal(leftValue, rightValue) {
			return false
		}
	}
}

func (word closedWord) hash(labels Labels, current uint64) uint64 {
	current = hash.MixHash(current, uint64(word.length))
	iterator := newClosedIterator(word)
	for {
		value, ok := iterator.Next()
		if !ok {
			return current
		}
		current = hash.MixHash(current, labels.Hash(value))
	}
}

func (word closedWord) lessEqual(labels Labels, other closedWord) bool {
	if word.length != other.length {
		return false
	}
	left, right := newClosedIterator(word), newClosedIterator(other)
	for {
		leftValue, leftOK := left.Next()
		rightValue, rightOK := right.Next()
		if leftOK != rightOK {
			return false
		}
		if !leftOK {
			return true
		}
		if !labelLessEqual(labels, leftValue, rightValue) {
			return false
		}
	}
}

func concatClosedWords(words ...closedWord) closedWord {
	var root *closedNode
	for _, word := range words {
		root = joinClosedNodes(root, word.root)
	}
	return closedWordFromLeaf(root)
}

func joinClosedNodes(left, right *closedNode) *closedNode {
	if left == nil {
		return right
	}
	if right == nil {
		return left
	}
	if closedHeight(left) > closedHeight(right)+1 {
		return balanceClosed(closedBranch(left.left, joinClosedNodes(left.right, right)))
	}
	if closedHeight(right) > closedHeight(left)+1 {
		return balanceClosed(closedBranch(joinClosedNodes(left, right.left), right.right))
	}
	return closedBranch(left, right)
}

func closedBranch(left, right *closedNode) *closedNode {
	if left == nil {
		return right
	}
	if right == nil {
		return left
	}
	height := closedHeight(left)
	if candidate := closedHeight(right); candidate > height {
		height = candidate
	}
	return &closedNode{left: left, right: right, length: left.length + right.length, height: height + 1}
}

func balanceClosed(node *closedNode) *closedNode {
	if node == nil || node.left == nil || node.right == nil {
		return node
	}
	if closedHeight(node.left) > closedHeight(node.right)+1 {
		left := node.left
		if closedHeight(left.left) >= closedHeight(left.right) {
			return closedBranch(left.left, closedBranch(left.right, node.right))
		}
		middle := left.right
		return closedBranch(closedBranch(left.left, middle.left), closedBranch(middle.right, node.right))
	}
	if closedHeight(node.right) > closedHeight(node.left)+1 {
		right := node.right
		if closedHeight(right.right) >= closedHeight(right.left) {
			return closedBranch(closedBranch(node.left, right.left), right.right)
		}
		middle := right.left
		return closedBranch(closedBranch(node.left, middle.left), closedBranch(middle.right, right.right))
	}
	return node
}

func closedHeight(node *closedNode) uint8 {
	if node == nil {
		return 0
	}
	return node.height
}

// ClosedIterator is a zero-allocation cursor over one immutable closed word.
// The fixed stack derives from the maximum height of an AVL tree whose node
// count fits in an addressable Go slice: fewer than two machine-word bits per
// level is sufficient. It is not a semantic capacity or widening bound.
const closedIteratorStackBound = 2 * bits.UintSize

type ClosedIterator struct {
	stack  [closedIteratorStackBound]*closedNode
	depth  int
	leaf   *closedNode
	offset int
}

func newClosedIterator(word closedWord) ClosedIterator {
	iterator := ClosedIterator{}
	iterator.descend(word.root)
	return iterator
}

func (iterator *ClosedIterator) descend(node *closedNode) {
	for node != nil && node.left != nil {
		if iterator.depth == len(iterator.stack) {
			panic("closed word AVL height exceeds addressable-node bound")
		}
		iterator.stack[iterator.depth] = node.right
		iterator.depth++
		node = node.left
	}
	iterator.leaf = node
	iterator.offset = 0
}

func (iterator *ClosedIterator) Next() (Handle, bool) {
	if iterator == nil {
		return Handle{}, false
	}
	for iterator.leaf != nil {
		if iterator.offset < iterator.leaf.length {
			var value Handle
			if len(iterator.leaf.flat) != 0 {
				value = iterator.leaf.flat[iterator.offset]
			} else {
				value = iterator.leaf.repeat
			}
			iterator.offset++
			return value, true
		}
		if iterator.depth == 0 {
			iterator.leaf = nil
			break
		}
		iterator.depth--
		next := iterator.stack[iterator.depth]
		iterator.stack[iterator.depth] = nil
		iterator.descend(next)
	}
	return Handle{}, false
}
