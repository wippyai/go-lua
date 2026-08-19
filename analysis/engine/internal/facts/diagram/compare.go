package diagram

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/terminal"
)

// CompareSoleFactorUnder proves a pure terminal relation across the sparse
// union of two roots from one Factor, restricted to within. It is the direct
// hot path for pointwise semantic laws: no Product rows, maps of columns,
// valuations, restricted roots, or carrier projection is constructed.
//
// relation must be pure. Unlike RelateSoleFactorUnder, this method may prove
// one reconvergent (left FDD, right FDD, support) suffix once, because its
// truth is independent of the route that reached that suffix. Use the older
// visitor when each structural occurrence must be observed.
func (diagram *Diagram[F, K, V]) CompareSoleFactorUnder(left, right Root[F, K, V], within support.Mask, scratch *SoleScratch[K, V], relation func(terminal.ID[V], terminal.ID[V]) bool) bool {
	if diagram == nil || !diagram.Valid(left) || !diagram.Valid(right) || !within.Valid() || within.Manager() != diagram.guards || scratch == nil || relation == nil {
		return false
	}
	factor, ok := diagram.SoleFactor()
	if !ok {
		return false
	}
	rank, ok := diagram.ranks[factor]
	if !ok || !scratch.prepare(factorKeys(findFactor(left.root, rank)), factorKeys(findFactor(right.root, rank))) {
		return false
	}
	return diagram.compareKeyTrees(within, scratch, relation)
}

// CompareSoleFactorRegions proves one pure terminal relation under a
// key-local region. A missing region means that the key is outside the left
// operand's lifted presence surface and therefore has no order obligation.
//
// Unlike repeatedly partitioning each FDD into support cells, this zipper
// synchronizes both immutable FDDs with the supplied BDD directly. It creates
// no guard candidate transaction or materialized intersections; all mutable
// traversal storage belongs to the caller's reusable SoleScratch.
func (diagram *Diagram[F, K, V]) CompareSoleFactorRegions(left, right Root[F, K, V], scratch *SoleScratch[K, V], regions func(K) (support.Mask, bool), relation func(terminal.ID[V], terminal.ID[V]) bool) bool {
	if diagram == nil || !diagram.Valid(left) || !diagram.Valid(right) || scratch == nil || regions == nil || relation == nil {
		return false
	}
	factor, ok := diagram.SoleFactor()
	if !ok {
		return false
	}
	rank, ok := diagram.ranks[factor]
	if !ok || !scratch.prepare(factorKeys(findFactor(left.root, rank)), factorKeys(findFactor(right.root, rank))) {
		return false
	}
	for {
		if !scratch.live() {
			return false
		}
		pair, present := scratch.nextPair()
		if !present {
			return true
		}
		region, covered := regions(pair.key)
		if !covered {
			continue
		}
		if !region.Valid() || region.Manager() != diagram.guards || !diagram.compareValuesAt(pair.left, pair.right, region, scratch, relation) {
			return false
		}
	}
}

// FoldValueUnder streams the terminal identities reachable from one immutable
// FDD value under within. It synchronizes the value's ordered decision chain
// with the supplied support BDD, just like compareValuesAt, but never builds
// their intersections or opens a support transaction. The caller owns
// SoleScratch, and the callback owns the terminal algebra. A zero terminal is
// reported so a semantic owner can distinguish sparse absence from a stored
// Default; a constant terminal is visited once because a coordinate-wise
// Join is idempotent.
//
// Returning false from visit or from the scratch liveness probe abandons the
// read immediately. A Value from another Diagram generation, a foreign
// support manager, or an invalid callback fails closed.
func (diagram *Diagram[F, K, V]) FoldValueUnder(value Value[V], within support.Mask, scratch *SoleScratch[K, V], visit func(terminal.ID[V]) bool) bool {
	if diagram == nil || value.owner != diagram.owner || value.node == nil || scratch == nil || visit == nil || !within.Valid() || within.Manager() != diagram.guards {
		return false
	}
	scratch.Clear()
	scratch.push(soleFrame[V]{left: value.node, region: within})
	for {
		if !scratch.live() {
			return false
		}
		frame, present := scratch.pop()
		if !present {
			return true
		}
		if scratch.seenBefore(soleTriple[V]{left: frame.left, region: frame.region}) {
			continue
		}
		view, ok := frame.region.Decompose()
		if !ok {
			return false
		}
		if view.Terminal && !view.Value {
			continue
		}
		valueRank, valueOK := diagram.nodeRank(frame.left)
		if !valueOK {
			return false
		}

		// A terminal FDD node has one value throughout the remaining support;
		// do not walk the support BDD merely to rediscover that same terminal.
		if valueRank == noRelationRank {
			id, valid := diagram.terminalAt(frame.left)
			if !valid || !visit(id) {
				return false
			}
			continue
		}
		regionRank, regionOK := diagram.regionRank(view)
		if !regionOK {
			return false
		}
		rank := minimumRank(valueRank, noRelationRank, regionRank)
		if rank == noRelationRank {
			id, valid := diagram.terminalAt(frame.left)
			if !valid || !visit(id) {
				return false
			}
			continue
		}
		lowRegion, highRegion := frame.region, frame.region
		if regionRank == rank {
			lowRegion, highRegion = view.Low, view.High
		}
		// LIFO keeps the exact low/high discovery order deterministic while
		// every descent advances to a strictly later ordered rank.
		scratch.push(soleFrame[V]{left: branchNode(frame.left, valueRank, rank, true), region: highRegion})
		scratch.push(soleFrame[V]{left: branchNode(frame.left, valueRank, rank, false), region: lowRegion})
	}
}

// compareKeyTrees is the ordered sparse-coordinate zipper shared by direct
// one-Factor operations. Its cursors preserve ascending keys without a
// materialized union, and its storage lives in SoleScratch for reuse by later
// fused merge/output work.
func (diagram *Diagram[F, K, V]) compareKeyTrees(within support.Mask, scratch *SoleScratch[K, V], relation func(terminal.ID[V], terminal.ID[V]) bool) bool {
	for {
		if !scratch.live() {
			return false
		}
		pair, present := scratch.nextPair()
		if !present {
			return true
		}
		if !diagram.compareValuesAt(pair.left, pair.right, within, scratch, relation) {
			return false
		}
	}
}

// compareValuesAt synchronizes one support BDD with two FDDs iteratively.
// The rank strictly increases on every pushed edge. High is pushed before low
// so a failed proof is deterministic: lowest key, then low cofactor first.
func (diagram *Diagram[F, K, V]) compareValuesAt(left, right *node[V], region support.Mask, scratch *SoleScratch[K, V], relation func(terminal.ID[V], terminal.ID[V]) bool) bool {
	scratch.push(soleFrame[V]{left: left, right: right, region: region})
	for {
		if !scratch.live() {
			return false
		}
		frame, present := scratch.pop()
		if !present {
			return true
		}
		triple := soleTriple[V]{left: frame.left, right: frame.right, region: frame.region}
		if scratch.seenBefore(triple) {
			continue
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
			if !leftValid || !rightValid || !relation(leftTerminal, rightTerminal) {
				return false
			}
			continue
		}
		lowRegion, highRegion := frame.region, frame.region
		if regionRank == rank {
			lowRegion, highRegion = view.Low, view.High
		}
		scratch.push(soleFrame[V]{
			left:   branchNode(frame.left, leftRank, rank, true),
			right:  branchNode(frame.right, rightRank, rank, true),
			region: highRegion,
		})
		scratch.push(soleFrame[V]{
			left:   branchNode(frame.left, leftRank, rank, false),
			right:  branchNode(frame.right, rightRank, rank, false),
			region: lowRegion,
		})
	}
}
