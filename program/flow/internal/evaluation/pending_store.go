package evaluation

import (
	"errors"
	"math"
	"slices"

	"github.com/wippyai/go-lua/program/keyspace"
)

// pendingNode is one fixed-width (32-bit) persistent set node. Branches are
// intentionally uncompressed: every level is one Term bit, which gives
// insertion a bounded 32-node path copy and lets Count/At walk without a map,
// sort, recursion, or public node handle.
type pendingNode struct {
	left, right uint32
	count       uint32
	term        keyspace.Term
	bit         uint8
}

const (
	pendingLeafBit = uint8(32)
	pendingRootBit = pendingLeafBit - 1
)

type pendingTermStore struct {
	nodes []pendingNode
}

// reserve grows the append-only node slab for one complete insertion before
// any path-copy node is appended. A fixed-width trie insertion contributes at
// most one leaf and the 32 branch levels; reserving that batch keeps branch's
// individual appends inside the already-grown backing array instead of
// allowing a late level to copy the entire retained store.
func (store *pendingTermStore) reserve(additional int) error {
	if store == nil || additional < 0 {
		return errors.New("program/flow/evaluation: invalid pending Term reservation")
	}
	if uint64(additional) > uint64(math.MaxUint32)-uint64(len(store.nodes)) {
		return errors.New("program/flow/evaluation: pending Term store is too large")
	}
	store.nodes = slices.Grow(store.nodes, additional)
	return nil
}

func newPendingTermStore() *pendingTermStore {
	return &pendingTermStore{nodes: []pendingNode{{}}}
}

func (store *pendingTermStore) node(index uint32) (pendingNode, bool) {
	if store == nil || uint64(index) >= uint64(len(store.nodes)) {
		return pendingNode{}, false
	}
	return store.nodes[index], true
}

// pendingNodeTermValid is intentionally independent of authored Flow. The
// trie stores packed Terms from several owner families, so the only safe leaf
// authority at this layer is the canonical Term encoding itself.
func pendingNodeTermValid(term keyspace.Term) bool {
	return term != 0 && keyspace.TermFamily(term) != keyspace.FamilyInvalid && keyspace.TermOrdinal(term) != 0
}

func pendingLeafNodeValid(node pendingNode) bool {
	return node.bit == pendingLeafBit && node.left == 0 && node.right == 0 &&
		node.count == 1 && pendingNodeTermValid(node.term)
}

// pendingBranchNodeValid checks a branch's local shape and its two child
// headers. Child indexes must point backward into the append-only store. This
// makes malformed forward references/cycles fail closed without a recursive
// walk, while preserving persistent sharing between unrelated branches.
func pendingBranchNodeValid(nodes []pendingNode, index uint32, node pendingNode) bool {
	if node.bit >= pendingLeafBit || node.term != 0 || node.count == 0 ||
		uint64(node.count) > uint64(len(nodes)) ||
		(node.left == 0 && node.right == 0) ||
		(node.left != 0 && node.left == node.right) {
		return false
	}
	if node.left != 0 {
		if uint64(node.left) >= uint64(len(nodes)) || node.left >= index {
			return false
		}
		child := nodes[node.left]
		if !pendingNodeHeaderValid(child) || !pendingChildLevelValid(node.bit, child.bit) {
			return false
		}
	}
	if node.right != 0 {
		if uint64(node.right) >= uint64(len(nodes)) || node.right >= index {
			return false
		}
		child := nodes[node.right]
		if !pendingNodeHeaderValid(child) || !pendingChildLevelValid(node.bit, child.bit) {
			return false
		}
	}
	leftCount, rightCount := uint64(0), uint64(0)
	if node.left != 0 {
		leftCount = uint64(nodes[node.left].count)
	}
	if node.right != 0 {
		rightCount = uint64(nodes[node.right].count)
	}
	return leftCount+rightCount == uint64(node.count)
}

// pendingChildLevelValid preserves the uncompressed fixed-width shape. A
// branch at bit n may point only to bit n-1; bit zero is the sole branch level
// whose child may be the leaf sentinel. Merely decreasing levels would admit
// compressed 31-to-leaf shapes that bypass the 32-bit trie law.
func pendingChildLevelValid(parent, child uint8) bool {
	if child == pendingLeafBit {
		return parent == 0
	}
	return child < pendingLeafBit && parent > 0 && child+1 == parent
}

func pendingNodeHeaderValid(node pendingNode) bool {
	if node.bit == pendingLeafBit {
		return pendingLeafNodeValid(node)
	}
	return node.bit < pendingLeafBit && node.term == 0 && node.count != 0
}

// pendingCanonicalRootValid is stricter than the local branch validator:
// every nonempty published root is the complete fixed-width trie root at bit
// 31. A leaf or lower-bit branch can be locally well formed, but accepting it
// as a root would admit a compressed or partial root that was never produced
// by insert.
func pendingCanonicalRootValid(nodes []pendingNode, root uint32) bool {
	if uint64(root) >= uint64(len(nodes)) {
		return false
	}
	if root == 0 {
		return nodes[0] == (pendingNode{})
	}
	node := nodes[root]
	return node.bit == pendingRootBit && pendingBranchNodeValid(nodes, root, node)
}

func (store *pendingTermStore) leaf(term keyspace.Term) (uint32, error) {
	if store == nil || !pendingNodeTermValid(term) {
		return 0, errors.New("program/flow/evaluation: invalid pending Term leaf")
	}
	if uint64(len(store.nodes)) >= uint64(math.MaxUint32) {
		return 0, errors.New("program/flow/evaluation: pending Term store is too large")
	}
	index := uint32(len(store.nodes))
	store.nodes = append(store.nodes, pendingNode{count: 1, term: term, bit: pendingLeafBit})
	return index, nil
}

func (store *pendingTermStore) branch(bit uint8, left, right uint32) (uint32, error) {
	if store == nil || bit >= pendingLeafBit || (left == 0 && right == 0) || (left != 0 && left == right) {
		return 0, errors.New("program/flow/evaluation: invalid pending Term branch")
	}
	index := uint32(len(store.nodes))
	leftNode, leftOK := store.node(left)
	rightNode, rightOK := store.node(right)
	if left != 0 && (!leftOK || left >= index || uint64(leftNode.count) > uint64(len(store.nodes)) || !pendingNodeHeaderValid(leftNode) || !pendingChildLevelValid(bit, leftNode.bit)) {
		return 0, errors.New("program/flow/evaluation: invalid pending left branch")
	}
	if right != 0 && (!rightOK || right >= index || uint64(rightNode.count) > uint64(len(store.nodes)) || !pendingNodeHeaderValid(rightNode) || !pendingChildLevelValid(bit, rightNode.bit)) {
		return 0, errors.New("program/flow/evaluation: invalid pending right branch")
	}
	count := uint64(0)
	if left != 0 {
		count += uint64(leftNode.count)
	}
	if right != 0 {
		count += uint64(rightNode.count)
	}
	if count > uint64(math.MaxUint32) || uint64(len(store.nodes)) >= uint64(math.MaxUint32) {
		return 0, errors.New("program/flow/evaluation: pending Term store is too large")
	}
	store.nodes = append(store.nodes, pendingNode{left: left, right: right, count: uint32(count), bit: bit})
	return index, nil
}

func pendingTermBit(term keyspace.Term, bit uint8) uint32 {
	return (uint32(term) >> bit) & 1
}

// insert returns root ∪ {term}. It copies only the fixed 32-bit path to the
// new leaf; unaffected subtrees remain shared by every retained prefix root.
func (store *pendingTermStore) insert(root uint32, term keyspace.Term) (uint32, error) {
	if store == nil || !pendingNodeTermValid(term) {
		return 0, errors.New("program/flow/evaluation: invalid pending insertion")
	}
	if len(store.nodes) == 0 || store.nodes[0] != (pendingNode{}) {
		return 0, errors.New("program/flow/evaluation: malformed pending storage sentinel")
	}
	if root == 0 {
		if err := store.reserve(int(pendingLeafBit) + 1); err != nil {
			return 0, err
		}
		// The term and full append batch are now checked. Every generated
		// leaf/branch header below is valid by construction, so no
		// externally constructible state can fail after reserve and before
		// the first append; branch errors remain defensive guards.
		leaf, err := store.leaf(term)
		if err != nil {
			return 0, err
		}
		for level := uint8(0); level < pendingLeafBit; level++ {
			if pendingTermBit(term, level) == 0 {
				leaf, err = store.branch(level, leaf, 0)
			} else {
				leaf, err = store.branch(level, 0, leaf)
			}
			if err != nil {
				return 0, err
			}
		}
		return leaf, nil
	}

	var path [pendingLeafBit]uint32
	var direction [pendingLeafBit]bool
	depth := 0
	current := root
	if !pendingCanonicalRootValid(store.nodes, root) {
		return 0, errors.New("program/flow/evaluation: invalid pending root")
	}
	for {
		node, ok := store.node(current)
		if !ok {
			return 0, errors.New("program/flow/evaluation: invalid pending root")
		}
		if node.bit == pendingLeafBit {
			if !pendingLeafNodeValid(node) {
				return 0, errors.New("program/flow/evaluation: invalid pending trie leaf")
			}
			if node.term == term {
				return root, nil
			}
			return 0, errors.New("program/flow/evaluation: pending trie leaf collision")
		}
		if node.bit >= pendingLeafBit || depth >= len(path) || !pendingBranchNodeValid(store.nodes, current, node) {
			return 0, errors.New("program/flow/evaluation: invalid pending trie depth")
		}
		right := pendingTermBit(term, node.bit) != 0
		path[depth] = current
		direction[depth] = right
		depth++
		if right {
			current = node.right
		} else {
			current = node.left
		}
		if current == 0 {
			ancestor, ancestorOK := store.node(path[depth-1])
			if !ancestorOK {
				return 0, errors.New("program/flow/evaluation: invalid pending missing ancestor")
			}
			// Validate the complete existing path before growing the slab. The
			// append count is one leaf plus the missing suffix and copied
			// ancestors; for a canonical 32-bit root this is exactly 33 nodes.
			// Malformed roots therefore fail without a partial persistent update.
			appendCount := 1 + int(ancestor.bit) + depth
			if err := store.reserve(appendCount); err != nil {
				return 0, err
			}
			// Path validation and reservation are complete before the first
			// append. The generated suffix and copied ancestors are checked by
			// construction; branch errors below are defensive-only and cannot
			// describe a failure introduced by caller-visible store state.
			leaf, err := store.leaf(term)
			if err != nil {
				return 0, err
			}
			for bit := uint8(0); bit < ancestor.bit; bit++ {
				if pendingTermBit(term, bit) == 0 {
					leaf, err = store.branch(bit, leaf, 0)
				} else {
					leaf, err = store.branch(bit, 0, leaf)
				}
				if err != nil {
					return 0, err
				}
			}
			for index := depth - 1; index >= 0; index-- {
				ancestor, ancestorOK = store.node(path[index])
				if !ancestorOK {
					return 0, errors.New("program/flow/evaluation: invalid pending ancestor")
				}
				if direction[index] {
					leaf, err = store.branch(ancestor.bit, ancestor.left, leaf)
				} else {
					leaf, err = store.branch(ancestor.bit, leaf, ancestor.right)
				}
				if err != nil {
					return 0, err
				}
			}
			return leaf, nil
		}
	}
}

func (store *pendingTermStore) code(root uint32) (uint32, error) {
	if store == nil || uint64(root) >= uint64(len(store.nodes)) || root == math.MaxUint32 {
		return 0, errors.New("program/flow/evaluation: invalid pending root code")
	}
	if root == 0 {
		// Code 1 is the sole exact empty root. A nonzero/scrambled sentinel
		// would make an absent root indistinguishable from an empty subject.
		if store.nodes[0] != (pendingNode{}) {
			return 0, errors.New("program/flow/evaluation: invalid pending empty root sentinel")
		}
		return 1, nil
	}
	if _, ok := pendingRootCount(store.nodes, root); !ok {
		return 0, errors.New("program/flow/evaluation: invalid pending root")
	}
	return root + 1, nil
}

func pendingTermAt(nodes []pendingNode, root, index uint32) (keyspace.Term, bool) {
	if uint64(root) >= uint64(len(nodes)) {
		return 0, false
	}
	if root == 0 || !pendingCanonicalRootValid(nodes, root) {
		return 0, false
	}
	current := root
	for steps := uint8(0); steps <= pendingLeafBit; steps++ {
		node := nodes[current]
		if node.bit == pendingLeafBit {
			if !pendingLeafNodeValid(node) {
				return 0, false
			}
			return node.term, index == 0
		}
		if node.bit >= pendingLeafBit || !pendingBranchNodeValid(nodes, current, node) {
			return 0, false
		}
		leftCount := uint32(0)
		if node.left != 0 {
			if uint64(node.left) >= uint64(len(nodes)) {
				return 0, false
			}
			left := nodes[node.left]
			leftCount = left.count
		}
		if node.right != 0 && uint64(node.right) >= uint64(len(nodes)) {
			return 0, false
		}
		if index < leftCount {
			current = node.left
		} else {
			index -= leftCount
			current = node.right
		}
		if current == 0 || uint64(current) >= uint64(len(nodes)) {
			return 0, false
		}
	}
	return 0, false
}

// pendingRootCount validates one canonical bit-31 root and its leftmost path.
// Every transition is bounded by the 32 packed Term bits plus its leaf (<=33
// nodes); malformed cycles/forward references cannot hang a query. Branch
// counts are trusted only after their local child/count equation is checked.
// The append-only backward-reference invariant makes that local equation
// inductive for sealed stores without a recursive or allocation-heavy scan.
func pendingRootCount(nodes []pendingNode, root uint32) (uint32, bool) {
	if uint64(root) >= uint64(len(nodes)) {
		return 0, false
	}
	if root == 0 {
		if nodes[0] != (pendingNode{}) {
			return 0, false
		}
		return 0, true
	}
	if !pendingCanonicalRootValid(nodes, root) {
		return 0, false
	}
	rootCount := nodes[root].count
	current := root
	for steps := uint8(0); steps <= pendingLeafBit; steps++ {
		node := nodes[current]
		if node.bit == pendingLeafBit {
			if !pendingLeafNodeValid(node) {
				return 0, false
			}
			return rootCount, true
		}
		if node.bit >= pendingLeafBit || !pendingBranchNodeValid(nodes, current, node) {
			return 0, false
		}
		if node.left != 0 {
			current = node.left
		} else {
			current = node.right
		}
		if current == 0 || uint64(current) >= uint64(len(nodes)) {
			return 0, false
		}
	}
	return 0, false
}

// validatePendingStorage proves the complete immutable storage once, before a
// Pending value is published. Nodes are append-ordered, so a single forward
// pass validates every child before its parent; common-bit summaries prove
// that left/right partitions really contain the branch bit they claim. Roots
// are checked against the same pass. Queries can therefore trust counts and
// perform only scalar gates plus one fixed-width walk.
func validatePendingStorage(nodes []pendingNode, roots [keyspace.FamilyCount][]uint32) error {
	if len(nodes) == 0 || uint64(len(nodes)) > uint64(math.MaxUint32) || nodes[0] != (pendingNode{}) {
		return errors.New("program/flow/evaluation: malformed pending storage sentinel")
	}
	valid := make([]bool, len(nodes))
	commonAnd := make([]uint32, len(nodes))
	commonOr := make([]uint32, len(nodes))
	valid[0] = true
	for index := 1; index < len(nodes); index++ {
		node := nodes[index]
		if node.bit == pendingLeafBit {
			if !pendingLeafNodeValid(node) || !pendingPayloadTerm(node.term) {
				return errors.New("program/flow/evaluation: malformed pending retained leaf")
			}
			valid[index] = true
			commonAnd[index], commonOr[index] = uint32(node.term), uint32(node.term)
			continue
		}
		if node.bit >= pendingLeafBit || !pendingBranchNodeValid(nodes, uint32(index), node) {
			return errors.New("program/flow/evaluation: malformed pending retained branch")
		}
		if node.left != 0 {
			if !valid[node.left] {
				return errors.New("program/flow/evaluation: pending branch child is unresolved")
			}
		}
		if node.right != 0 {
			if !valid[node.right] {
				return errors.New("program/flow/evaluation: pending branch child is unresolved")
			}
		}
		bit := uint32(1) << node.bit
		if node.left != 0 && commonOr[node.left]&bit != 0 {
			return errors.New("program/flow/evaluation: pending left branch partition is malformed")
		}
		if node.right != 0 && commonAnd[node.right]&bit == 0 {
			return errors.New("program/flow/evaluation: pending right branch partition is malformed")
		}
		if node.left == 0 {
			commonAnd[index], commonOr[index] = commonAnd[node.right], commonOr[node.right]
		} else if node.right == 0 {
			commonAnd[index], commonOr[index] = commonAnd[node.left], commonOr[node.left]
		} else {
			commonAnd[index] = commonAnd[node.left] & commonAnd[node.right]
			commonOr[index] = commonOr[node.left] | commonOr[node.right]
		}
		valid[index] = true
	}
	if len(roots[keyspace.FamilyInvalid]) != 0 {
		return errors.New("program/flow/evaluation: malformed invalid-family root plane")
	}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		plane := roots[family]
		if len(plane) == 0 {
			continue
		}
		if !pendingSubjectFamily(family) || plane[0] != 0 {
			return errors.New("program/flow/evaluation: malformed pending root plane")
		}
		for ordinal := 1; ordinal < len(plane); ordinal++ {
			code := plane[ordinal]
			if code == 0 {
				continue
			}
			root := code - 1
			if uint64(root) >= uint64(len(nodes)) || !valid[root] ||
				(root != 0 && nodes[root].bit != pendingRootBit) {
				return errors.New("program/flow/evaluation: pending root leaves retained storage")
			}
		}
	}
	return nil
}
