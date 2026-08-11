package continuation

import "github.com/wippyai/go-lua/program/keyspace"

// guardNode is one canonical ordered Guard-set prefix.  Every nonzero node
// appends term to prev.  Equal prefixes share one node during Seal; jump is a
// Fenwick ancestor used to stay within the fixed uint32 structural proof
// bound during GuardAt.
type guardNode struct {
	prev  uint32
	jump  uint32
	term  keyspace.Term
	count uint32
}

type guardChainBuilder struct {
	nodes      []guardNode
	childEpoch []uint32
	childID    []uint32
	generation uint32
}

func newGuardChainBuilder() guardChainBuilder {
	return guardChainBuilder{nodes: []guardNode{{}}, childEpoch: []uint32{0}, childID: []uint32{0}}
}

func (builder *guardChainBuilder) beginRank() {
	builder.generation++
	if builder.generation != 0 {
		return
	}
	for index := range builder.childEpoch {
		builder.childEpoch[index] = 0
	}
	builder.generation = 1
}

func (builder *guardChainBuilder) append(parent uint32, term keyspace.Term) (uint32, bool) {
	if builder == nil || builder.generation == 0 || uint64(parent) >= uint64(len(builder.nodes)) {
		return 0, false
	}
	if uint64(parent) >= uint64(len(builder.childEpoch)) {
		if uint64(parent)+1 > uint64(^uint(0)>>1) {
			return 0, false
		}
		growth := int(parent) + 1 - len(builder.childEpoch)
		builder.childEpoch = append(builder.childEpoch, make([]uint32, growth)...)
		builder.childID = append(builder.childID, make([]uint32, growth)...)
	}
	if builder.childEpoch[parent] == builder.generation {
		return builder.childID[parent], true
	}
	parentNode := builder.nodes[parent]
	if parentNode.count == ^uint32(0) || (parentNode.count != 0 && parentNode.term >= term) ||
		uint64(len(builder.nodes)) >= uint64(^uint32(0)) || uint64(len(builder.nodes)) >= uint64(^uint(0)>>1) {
		return 0, false
	}
	count := parentNode.count + 1
	target := count - guardLowbit(count)
	jump, ok := guardAncestor(builder.nodes, parent, target)
	if !ok {
		return 0, false
	}
	entryLength := uint64(len(builder.nodes))
	if entryLength >= uint64(^uint32(0)) || entryLength >= uint64(^uint(0)>>1) {
		return 0, false
	}
	entry := uint32(entryLength)
	builder.nodes = append(builder.nodes, guardNode{prev: parent, jump: jump, term: term, count: count})
	builder.childEpoch[parent] = builder.generation
	builder.childID[parent] = entry
	return entry, true
}

func guardRank(term keyspace.Term, counts [keyspace.FamilyCount]uint32) (uint32, bool) {
	ordinal := keyspace.TermOrdinal(term)
	if ordinal == 0 {
		return 0, false
	}
	priorOrdinal := ordinal - 1
	prior := uint64(minUint32(counts[keyspace.FamilySelect], priorOrdinal)) +
		uint64(minUint32(counts[keyspace.FamilyBranch], priorOrdinal)) +
		uint64(minUint32(counts[keyspace.FamilyLoop], priorOrdinal))
	var offset uint64
	switch keyspace.TermFamily(term) {
	case keyspace.FamilySelect:
		if ordinal > counts[keyspace.FamilySelect] {
			return 0, false
		}
	case keyspace.FamilyBranch:
		if ordinal > counts[keyspace.FamilyBranch] {
			return 0, false
		}
		if ordinal <= counts[keyspace.FamilySelect] {
			offset++
		}
	case keyspace.FamilyLoop:
		if ordinal > counts[keyspace.FamilyLoop] {
			return 0, false
		}
		if ordinal <= counts[keyspace.FamilySelect] {
			offset++
		}
		if ordinal <= counts[keyspace.FamilyBranch] {
			offset++
		}
	default:
		return 0, false
	}
	rank := prior + offset
	if rank > uint64(^uint32(0)) {
		return 0, false
	}
	return uint32(rank), true
}

func guardTermAtRank(rank uint32, counts [keyspace.FamilyCount]uint32) (keyspace.Term, bool) {
	total := uint64(counts[keyspace.FamilySelect]) + uint64(counts[keyspace.FamilyBranch]) + uint64(counts[keyspace.FamilyLoop])
	if uint64(rank) >= total {
		return 0, false
	}
	maximum := maxUint32(counts[keyspace.FamilySelect], maxUint32(counts[keyspace.FamilyBranch], counts[keyspace.FamilyLoop]))
	left, right := uint32(1), maximum
	for left < right {
		middle := left + (right-left)/2
		through := uint64(minUint32(counts[keyspace.FamilySelect], middle)) +
			uint64(minUint32(counts[keyspace.FamilyBranch], middle)) +
			uint64(minUint32(counts[keyspace.FamilyLoop], middle))
		if uint64(rank) < through {
			right = middle
		} else {
			left = middle + 1
		}
	}
	ordinal := left
	prior := uint64(minUint32(counts[keyspace.FamilySelect], ordinal-1)) +
		uint64(minUint32(counts[keyspace.FamilyBranch], ordinal-1)) +
		uint64(minUint32(counts[keyspace.FamilyLoop], ordinal-1))
	offset := uint64(rank) - prior
	for _, family := range [...]keyspace.Family{keyspace.FamilySelect, keyspace.FamilyBranch, keyspace.FamilyLoop} {
		if ordinal > counts[family] {
			continue
		}
		if offset == 0 {
			return keyspace.MakeTerm(family, ordinal), true
		}
		offset--
	}
	return 0, false
}

func minUint32(left, right uint32) uint32 {
	if left < right {
		return left
	}
	return right
}

func maxUint32(left, right uint32) uint32 {
	if left > right {
		return left
	}
	return right
}

func guardLowbit(value uint32) uint32 { return value & (^value + 1) }

// guardAncestorFenwickProofBound is a representation proof bound, not a
// semantic convergence or solver-work budget.  For a w-bit prefix count, the
// jump-or-prev Fenwick recurrence has T(1)=0 and T(w)=T(w-1)+w, hence
// worst-case length w(w+1)/2-1 links; the fixed uint32 representation
// therefore has bound 32*33/2-1 = 527.  The traversal uses this
// mathematically-derived bound only as a fail-closed corruption fence: a
// malformed store that would exceed it is unavailable instead of being
// allowed to loop.
const guardAncestorFenwickProofBound = 32*33/2 - 1

// guardAncestor returns the node with the requested prefix count.
func guardAncestor(nodes []guardNode, root, target uint32) (uint32, bool) {
	if uint64(root) >= uint64(len(nodes)) || target > nodes[root].count {
		return 0, false
	}
	current := root
	for steps := 0; steps < guardAncestorFenwickProofBound && nodes[current].count > target; steps++ {
		node := nodes[current]
		next := node.prev
		if uint64(node.jump) >= uint64(len(nodes)) || uint64(next) >= uint64(len(nodes)) {
			return 0, false
		}
		if nodes[node.jump].count >= target {
			next = node.jump
		}
		if next >= current || nodes[next].count >= node.count {
			return 0, false
		}
		current = next
	}
	return current, nodes[current].count == target
}

func (projection *guardProjection) at(root, count, index uint32) (keyspace.Term, bool) {
	if projection == nil || len(projection.nodes) == 0 || projection.nodes[0] != (guardNode{}) || index >= count ||
		uint64(root) >= uint64(len(projection.nodes)) || projection.nodes[root].count != count {
		return 0, false
	}
	nodeIndex, ok := guardAncestor(projection.nodes, root, index+1)
	if !ok || nodeIndex == 0 {
		return 0, false
	}
	term := projection.nodes[nodeIndex].term
	if _, valid := guardRank(term, projection.counts); !valid {
		return 0, false
	}
	return term, true
}
