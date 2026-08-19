package diagram

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/scalar"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/terminal"
)

// RelateSoleFactorUnder proves visit for every terminal pair selected by
// within across the union of two sparse roots from a single-Factor Diagram.
// A missing column is the zero (undefined) terminal.  The caller gives that
// terminal its typed meaning (normally the Factor Default); Diagram remains
// structural and owns neither lattice semantics nor a second fact carrier.
//
// The walk is read-only and streams keys in ascending order. It constructs no
// Product rows, key slices, maps, valuations, support work, or restricted
// roots. Returning false from visit stops both the current FDD walk and the
// outer key merge immediately.
func (diagram *Diagram[F, K, V]) RelateSoleFactorUnder(left, right Root[F, K, V], within support.Mask, visit func(K, terminal.ID[V], terminal.ID[V]) bool) bool {
	if diagram == nil || !diagram.Valid(left) || !diagram.Valid(right) || !within.Valid() || within.Manager() != diagram.guards || visit == nil {
		return false
	}
	factor, ok := diagram.SoleFactor()
	if !ok {
		return false
	}
	rank, ok := diagram.ranks[factor]
	if !ok {
		return false
	}
	leftKeys := factorKeys(findFactor(left.root, rank))
	rightKeys := factorKeys(findFactor(right.root, rank))
	return diagram.relateKeyTrees(leftKeys, rightKeys, within, visit)
}

// relationInlineDepth keeps ordinary AVL/FDD walks entirely on the stack.
// The cursor spills without a semantic cap when an unusually deep, valid
// diagram needs it; a spill changes only storage, never the compared set.
const relationInlineDepth = 32

// avlCursor is an in-order immutable AVL cursor. spill is deliberately
// uncapped: facts must never become less precise because a representation is
// deeper than an implementation convenience.
type avlCursor[K scalar.Key, V any] struct {
	inline [relationInlineDepth]*keyNode[K, V]
	spill  []*keyNode[K, V]
	depth  int
}

func (cursor *avlCursor[K, V]) begin(root *keyNode[K, V]) {
	cursor.clear()
	cursor.pushLeft(root)
}

func (cursor *avlCursor[K, V]) clear() {
	for index := 0; index < cursor.depth; index++ {
		if index < len(cursor.inline) {
			cursor.inline[index] = nil
			continue
		}
		cursor.spill[index-len(cursor.inline)] = nil
	}
	cursor.depth = 0
}

func (cursor *avlCursor[K, V]) pushLeft(root *keyNode[K, V]) {
	for root != nil {
		cursor.push(root)
		root = root.left
	}
}

func (cursor *avlCursor[K, V]) push(value *keyNode[K, V]) {
	index := cursor.depth
	if index < len(cursor.inline) {
		cursor.inline[index] = value
	} else {
		index -= len(cursor.inline)
		if index == len(cursor.spill) {
			cursor.spill = append(cursor.spill, value)
		} else {
			cursor.spill[index] = value
		}
	}
	cursor.depth++
}

func (cursor *avlCursor[K, V]) next() (*keyNode[K, V], bool) {
	if cursor.depth == 0 {
		return nil, false
	}
	cursor.depth--
	index := cursor.depth
	var current *keyNode[K, V]
	if index < len(cursor.inline) {
		current = cursor.inline[index]
		cursor.inline[index] = nil
	} else {
		index -= len(cursor.inline)
		current = cursor.spill[index]
		cursor.spill[index] = nil
	}
	cursor.pushLeft(current.right)
	return current, true
}

func (diagram *Diagram[F, K, V]) relateKeyTrees(left, right *keyNode[K, V], within support.Mask, visit func(K, terminal.ID[V], terminal.ID[V]) bool) bool {
	var leftCursor, rightCursor avlCursor[K, V]
	leftCursor.begin(left)
	rightCursor.begin(right)
	leftKey, hasLeft := leftCursor.next()
	rightKey, hasRight := rightCursor.next()
	for hasLeft || hasRight {
		switch {
		case !hasRight || hasLeft && leftKey.key < rightKey.key:
			if !diagram.relateValuesAt(leftKey.value, nil, within, leftKey.key, visit) {
				return false
			}
			leftKey, hasLeft = leftCursor.next()
		case !hasLeft || rightKey.key < leftKey.key:
			if !diagram.relateValuesAt(nil, rightKey.value, within, rightKey.key, visit) {
				return false
			}
			rightKey, hasRight = rightCursor.next()
		default:
			if !diagram.relateValuesAt(leftKey.value, rightKey.value, within, leftKey.key, visit) {
				return false
			}
			leftKey, hasLeft = leftCursor.next()
			rightKey, hasRight = rightCursor.next()
		}
	}
	return true
}

func factorKeys[F ~uint64, K scalar.Key, V any](factor *factorNode[F, K, V]) *keyNode[K, V] {
	if factor == nil {
		return nil
	}
	return factor.keys
}

type relationFrame[V any] struct {
	left, right *node[V]
	region      support.Mask
}

// relationStack is the FDD counterpart to avlCursor. FDD paths are ordered
// and acyclic; the stack is only traversal storage, not a fixpoint/worklist.
// Like the AVL cursor it spills exactly when needed instead of imposing a
// depth budget on valid input.
type relationStack[V any] struct {
	inline [relationInlineDepth]relationFrame[V]
	spill  []relationFrame[V]
	depth  int
}

func (stack *relationStack[V]) push(value relationFrame[V]) {
	index := stack.depth
	if index < len(stack.inline) {
		stack.inline[index] = value
	} else {
		index -= len(stack.inline)
		if index == len(stack.spill) {
			stack.spill = append(stack.spill, value)
		} else {
			stack.spill[index] = value
		}
	}
	stack.depth++
}

func (stack *relationStack[V]) pop() (relationFrame[V], bool) {
	if stack.depth == 0 {
		return relationFrame[V]{}, false
	}
	stack.depth--
	index := stack.depth
	var result relationFrame[V]
	if index < len(stack.inline) {
		result = stack.inline[index]
		stack.inline[index] = relationFrame[V]{}
	} else {
		index -= len(stack.inline)
		result = stack.spill[index]
		stack.spill[index] = relationFrame[V]{}
	}
	return result, true
}

// relateValuesAt synchronizes an exact support BDD with two exact FDDs. At
// every step it splits at the least next guard rank, so every pushed path has
// a strictly later decision than its parent. The low frame is popped first to
// make failure deterministic and promptly observable.
func (diagram *Diagram[F, K, V]) relateValuesAt(left, right *node[V], region support.Mask, key K, visit func(K, terminal.ID[V], terminal.ID[V]) bool) bool {
	var pending relationStack[V]
	pending.push(relationFrame[V]{left: left, right: right, region: region})
	for {
		frame, present := pending.pop()
		if !present {
			return true
		}
		view, ok := frame.region.Decompose()
		if !ok {
			return false
		}
		if view.Terminal && !view.Value {
			continue
		}
		leftRank, leftOK := diagram.nodeRank(frame.left)
		rightRank, rightOK := diagram.nodeRank(frame.right)
		regionRank, regionOK := diagram.regionRank(view)
		if !leftOK || !rightOK || !regionOK {
			return false
		}
		rank := minimumRank(leftRank, rightRank, regionRank)
		if rank == noRelationRank {
			leftTerminal, leftValid := diagram.terminalAt(frame.left)
			rightTerminal, rightValid := diagram.terminalAt(frame.right)
			if !leftValid || !rightValid || !visit(key, leftTerminal, rightTerminal) {
				return false
			}
			continue
		}
		lowRegion, highRegion := frame.region, frame.region
		if regionRank == rank {
			lowRegion, highRegion = view.Low, view.High
		}
		// LIFO: push high first so the lower Boolean cofactor is compared
		// before the higher cofactor and can stop the relation immediately.
		pending.push(relationFrame[V]{
			left:   branchNode(frame.left, leftRank, rank, true),
			right:  branchNode(frame.right, rightRank, rank, true),
			region: highRegion,
		})
		pending.push(relationFrame[V]{
			left:   branchNode(frame.left, leftRank, rank, false),
			right:  branchNode(frame.right, rightRank, rank, false),
			region: lowRegion,
		})
	}
}

const noRelationRank = ^uint64(0)

func (diagram *Diagram[F, K, V]) nodeRank(value *node[V]) (uint64, bool) {
	if value == nil || value.terminal {
		return noRelationRank, true
	}
	return diagram.guards.Rank(value.atom)
}

func (diagram *Diagram[F, K, V]) regionRank(view support.Decomposition) (uint64, bool) {
	if view.Terminal {
		return noRelationRank, true
	}
	return diagram.guards.Rank(view.Atom)
}

func minimumRank(first, second, third uint64) uint64 {
	if first <= second && first <= third {
		return first
	}
	if second <= third {
		return second
	}
	return third
}

func branchNode[V any](value *node[V], valueRank, target uint64, high bool) *node[V] {
	if value == nil || valueRank != target {
		return value
	}
	if high {
		return value.high
	}
	return value.low
}

func (diagram *Diagram[F, K, V]) terminalAt(value *node[V]) (terminal.ID[V], bool) {
	if value == nil {
		return terminal.ID[V]{}, true
	}
	if !value.terminal {
		return terminal.ID[V]{}, false
	}
	if value.value == (terminal.ID[V]{}) {
		return value.value, true
	}
	return value.value, diagram.terminals.Valid(value.value)
}
