package index

import (
	"bytes"
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// rowPostingDirectory is a persistent AVL index over owner-directory
// coordinates. Successor updates copy only the search paths and affected
// posting slices; unchanged row groups remain shared by immutable roots.
// This keeps an index transition bounded by changed postings rather than the
// relation population width.
type rowPostingDirectory struct {
	root *rowPostingNode
}

type rowPostingNode struct {
	group       rowPostingGroup
	left, right *rowPostingNode
	height      int8
}

// buildRowPostingDirectory materializes the exact RowID inverse owned by one
// immutable trie root.  The trie walk is a construction/ successor operation;
// readers use the resulting directory directly and never enumerate the trie
// to discover a requested row.
//
// The directory coordinate is the mounted owner directory position.  It is
// deliberately retained as an int here rather than converted to geometry.Key:
// a RowID lookup has no authority to manufacture a physical geometry address
// from a logical identity. Each group's postings retain canonical posting
// order, including repeated support fibers for one RowID.
func buildRowPostingDirectory(root *trieNode, width int, mounted witness.Mounted, relation model.RelationID) (*rowPostingDirectory, bool) {
	if root == nil || width < 0 || !mounted.Available() || !relation.Available() {
		return nil, false
	}
	groups := make([]rowPostingGroup, 0)
	positions := make(map[model.RowID]int)
	var visit func(*trieNode, int) bool
	visit = func(node *trieNode, depth int) bool {
		if node == nil || depth < 0 || depth > width {
			return false
		}
		if len(node.postings) != 0 {
			if depth != width || len(node.children) != 0 {
				return false
			}
			for position := range node.postings {
				posting := node.postings[position]
				if posting.relation != relation || !posting.row.Available() || posting.row.Relation() != relation || !posting.region.Valid() || support.Empty(posting.region) {
					return false
				}
				coordinate, coordinateOK := mounted.RowIndex(relation, posting.row)
				if !coordinateOK || coordinate < 0 {
					return false
				}
				redeemed, redeemedOK := mounted.RowAt(relation, coordinate)
				if !redeemedOK || redeemed != posting.row {
					return false
				}
				group, found := positions[posting.row]
				if !found {
					group = len(groups)
					positions[posting.row] = group
					groups = append(groups, rowPostingGroup{coordinate: coordinate, row: posting.row})
				}
				if groups[group].coordinate != coordinate || groups[group].row != posting.row {
					return false
				}
				groups[group].postings = append(groups[group].postings, posting)
			}
			return true
		}
		if depth == width {
			// An empty leaf is a valid empty trie branch.
			return true
		}
		for _, edge := range node.children {
			if !visit(edge.child, depth+1) {
				return false
			}
		}
		return true
	}
	if !visit(root, 0) {
		return nil, false
	}
	// Mounted row coordinates are the canonical order for the inverse.  This
	// sort affects only group addressing; posting order within each group is
	// canonicalized independently so updates can redeem/remove one posting
	// with a bounded search.
	sort.SliceStable(groups, func(left, right int) bool {
		return groups[left].coordinate < groups[right].coordinate
	})
	for position := range groups {
		sort.SliceStable(groups[position].postings, func(left, right int) bool {
			return postingLess(groups[position].postings[left], groups[position].postings[right])
		})
	}
	return &rowPostingDirectory{root: buildRowPostingTree(groups, 0, len(groups))}, true
}

func buildRowPostingTree(groups []rowPostingGroup, left, right int) *rowPostingNode {
	if left >= right {
		return nil
	}
	middle := left + (right-left)/2
	node := &rowPostingNode{group: groups[middle], height: 1}
	node.left = buildRowPostingTree(groups, left, middle)
	node.right = buildRowPostingTree(groups, middle+1, right)
	return refreshRowPostingNode(node)
}

func rowPostingHeight(node *rowPostingNode) int8 {
	if node == nil {
		return 0
	}
	return node.height
}

func refreshRowPostingNode(node *rowPostingNode) *rowPostingNode {
	if node == nil {
		return nil
	}
	leftHeight, rightHeight := rowPostingHeight(node.left), rowPostingHeight(node.right)
	if leftHeight > rightHeight {
		node.height = leftHeight + 1
	} else {
		node.height = rightHeight + 1
	}
	return node
}

func cloneRowPostingNode(node *rowPostingNode) *rowPostingNode {
	if node == nil {
		return nil
	}
	copyOf := *node
	// Posting slices are immutable unless this exact coordinate is the
	// affected group. Path copies therefore retain unchanged groups by
	// identity; insertRowPosting/removeRowPosting clone the target slice before
	// editing it.
	return &copyOf
}

func balanceRowPostingNode(node *rowPostingNode) int8 {
	if node == nil {
		return 0
	}
	return rowPostingHeight(node.left) - rowPostingHeight(node.right)
}

func rotateRowPostingLeft(node *rowPostingNode) *rowPostingNode {
	copyOf := cloneRowPostingNode(node)
	right := cloneRowPostingNode(copyOf.right)
	if right == nil {
		return copyOf
	}
	copyOf.right = right.left
	right.left = refreshRowPostingNode(copyOf)
	return refreshRowPostingNode(right)
}

func rotateRowPostingRight(node *rowPostingNode) *rowPostingNode {
	copyOf := cloneRowPostingNode(node)
	left := cloneRowPostingNode(copyOf.left)
	if left == nil {
		return copyOf
	}
	copyOf.left = left.right
	left.right = refreshRowPostingNode(copyOf)
	return refreshRowPostingNode(left)
}

func rebalanceRowPostingNode(node *rowPostingNode) *rowPostingNode {
	node = refreshRowPostingNode(node)
	if balanceRowPostingNode(node) > 1 {
		if balanceRowPostingNode(node.left) < 0 {
			copyOf := cloneRowPostingNode(node)
			copyOf.left = rotateRowPostingLeft(copyOf.left)
			node = copyOf
		}
		return rotateRowPostingRight(node)
	}
	if balanceRowPostingNode(node) < -1 {
		if balanceRowPostingNode(node.right) > 0 {
			copyOf := cloneRowPostingNode(node)
			copyOf.right = rotateRowPostingRight(copyOf.right)
			node = copyOf
		}
		return rotateRowPostingLeft(node)
	}
	return node
}

func findRowPostingNode(node *rowPostingNode, coordinate int) *rowPostingNode {
	for node != nil {
		if coordinate < node.group.coordinate {
			node = node.left
			continue
		}
		if coordinate > node.group.coordinate {
			node = node.right
			continue
		}
		return node
	}
	return nil
}

func insertRowPostingNode(node *rowPostingNode, group rowPostingGroup) (*rowPostingNode, bool) {
	if node == nil {
		group.postings = append([]posting(nil), group.postings...)
		return &rowPostingNode{group: group, height: 1}, true
	}
	copyOf := cloneRowPostingNode(node)
	if group.coordinate < node.group.coordinate {
		var inserted bool
		copyOf.left, inserted = insertRowPostingNode(node.left, group)
		if !inserted {
			return node, false
		}
		return rebalanceRowPostingNode(copyOf), true
	}
	if group.coordinate > node.group.coordinate {
		var inserted bool
		copyOf.right, inserted = insertRowPostingNode(node.right, group)
		if !inserted {
			return node, false
		}
		return rebalanceRowPostingNode(copyOf), true
	}
	return node, false
}

func removeRowPostingNode(node *rowPostingNode, coordinate int) (*rowPostingNode, bool) {
	if node == nil {
		return nil, false
	}
	if coordinate < node.group.coordinate {
		copyOf := cloneRowPostingNode(node)
		var removed bool
		copyOf.left, removed = removeRowPostingNode(node.left, coordinate)
		if !removed {
			return node, false
		}
		return rebalanceRowPostingNode(copyOf), true
	}
	if coordinate > node.group.coordinate {
		copyOf := cloneRowPostingNode(node)
		var removed bool
		copyOf.right, removed = removeRowPostingNode(node.right, coordinate)
		if !removed {
			return node, false
		}
		return rebalanceRowPostingNode(copyOf), true
	}
	if node.left == nil {
		return node.right, true
	}
	if node.right == nil {
		return node.left, true
	}
	successor := node.right
	for successor.left != nil {
		successor = successor.left
	}
	copyOf := cloneRowPostingNode(successor)
	copyOf.left = node.left
	var removed bool
	copyOf.right, removed = removeRowPostingNode(node.right, successor.group.coordinate)
	if !removed {
		return node, false
	}
	return rebalanceRowPostingNode(copyOf), true
}

func replaceRowPostingNode(node *rowPostingNode, coordinate int, replacement *rowPostingNode) *rowPostingNode {
	if node == nil {
		return nil
	}
	if coordinate < node.group.coordinate {
		copyOf := cloneRowPostingNode(node)
		copyOf.left = replaceRowPostingNode(node.left, coordinate, replacement)
		return refreshRowPostingNode(copyOf)
	}
	if coordinate > node.group.coordinate {
		copyOf := cloneRowPostingNode(node)
		copyOf.right = replaceRowPostingNode(node.right, coordinate, replacement)
		return refreshRowPostingNode(copyOf)
	}
	return replacement
}

func insertRowPosting(directory *rowPostingDirectory, value posting, mounted witness.Mounted, relation model.RelationID) (*rowPostingDirectory, bool) {
	if directory == nil || !mounted.Available() || !relation.Available() || value.relation != relation || !value.row.Available() || value.row.Relation() != relation || !value.region.Valid() || support.Empty(value.region) {
		return nil, false
	}
	coordinate, ok := replayRowOrdinal(mounted, relation, value.row)
	if !ok {
		return nil, false
	}
	node := findRowPostingNode(directory.root, coordinate)
	if node != nil {
		if node.group.row != value.row {
			return nil, false
		}
		copyOf := cloneRowPostingNode(node)
		position := sort.Search(len(copyOf.group.postings), func(position int) bool {
			return !postingLess(copyOf.group.postings[position], value)
		})
		if position < len(copyOf.group.postings) && postingEqual(copyOf.group.postings[position], value) {
			return nil, false
		}
		copyOf.group.postings = append(copyOf.group.postings, posting{})
		copy(copyOf.group.postings[position+1:], copyOf.group.postings[position:])
		copyOf.group.postings[position] = value
		return &rowPostingDirectory{root: replaceRowPostingNode(directory.root, coordinate, copyOf)}, true
	}
	root, inserted := insertRowPostingNode(directory.root, rowPostingGroup{coordinate: coordinate, row: value.row, postings: []posting{value}})
	if !inserted {
		return nil, false
	}
	return &rowPostingDirectory{root: root}, true
}

func removeRowPosting(directory *rowPostingDirectory, value posting, mounted witness.Mounted, relation model.RelationID) (*rowPostingDirectory, bool) {
	if directory == nil || !mounted.Available() || !relation.Available() || value.relation != relation || !value.row.Available() || value.row.Relation() != relation {
		return nil, false
	}
	coordinate, ok := replayRowOrdinal(mounted, relation, value.row)
	if !ok {
		return nil, false
	}
	node := findRowPostingNode(directory.root, coordinate)
	if node == nil || node.group.row != value.row {
		return nil, false
	}
	position := sort.Search(len(node.group.postings), func(position int) bool {
		return !postingLess(node.group.postings[position], value)
	})
	if position >= len(node.group.postings) || !postingEqual(node.group.postings[position], value) {
		return nil, false
	}
	copyOf := cloneRowPostingNode(node)
	copyOf.group.postings = append(copyOf.group.postings[:position], copyOf.group.postings[position+1:]...)
	if len(copyOf.group.postings) != 0 {
		return &rowPostingDirectory{root: replaceRowPostingNode(directory.root, coordinate, copyOf)}, true
	}
	root, removed := removeRowPostingNode(directory.root, coordinate)
	if !removed {
		return nil, false
	}
	return &rowPostingDirectory{root: root}, true
}

// postingLess is the canonical order used by trie leaf posting vectors.  It
// is kept here as a strict comparator so a RowID inverse can preserve exact
// multiplicity/order without exposing or reconstructing a second row ABI.
func postingLess(left, right posting) bool {
	if left.key != right.key {
		return left.key < right.key
	}
	if left.relation != right.relation {
		leftOwner, rightOwner := left.relation.Owner().Content(), right.relation.Owner().Content()
		if compared := bytes.Compare(leftOwner[:], rightOwner[:]); compared != 0 {
			return compared < 0
		}
		leftRelation, rightRelation := left.relation.Content(), right.relation.Content()
		return bytes.Compare(leftRelation[:], rightRelation[:]) < 0
	}
	if left.row != right.row {
		leftOwner, rightOwner := left.row.Relation().Owner().Content(), right.row.Relation().Owner().Content()
		if compared := bytes.Compare(leftOwner[:], rightOwner[:]); compared != 0 {
			return compared < 0
		}
		leftRow, rightRow := left.row.Content(), right.row.Content()
		return bytes.Compare(leftRow[:], rightRow[:]) < 0
	}
	return bytes.Compare(left.regionID[:], right.regionID[:]) < 0
}
