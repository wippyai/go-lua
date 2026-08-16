package static

import "math/bits"

// A coverage is one packed bitset over the dense positions of the sealed
// observation universe P. It is the canonical descriptor form: every hot
// ClassSet judgment is a bitwise operation on coverages and asks the sealed
// atom relation nothing.
//
// All coverages of one ClassSet share the owner's stride, so the operations
// below index words directly and never allocate.

func coverageBit(position int) (int, uint64) {
	return position >> 6, 1 << (uint(position) & 63)
}

func coverageContains(coverage []uint64, position int) bool {
	word, mask := coverageBit(position)
	return word < len(coverage) && coverage[word]&mask != 0
}

func coverageSet(coverage []uint64, position int) {
	word, mask := coverageBit(position)
	coverage[word] |= mask
}

func coverageEqual(left, right []uint64) bool {
	if len(left) != len(right) {
		return false
	}
	for index, word := range left {
		if word != right[index] {
			return false
		}
	}
	return true
}

// coverageSubset is the order of the descriptor lattice: left is below right
// exactly when it observes nothing right does not.
func coverageSubset(left, right []uint64) bool {
	if len(left) != len(right) {
		return false
	}
	for index, word := range left {
		if word&^right[index] != 0 {
			return false
		}
	}
	return true
}

func coverageOr(destination, left, right []uint64) bool {
	if len(destination) != len(left) || len(left) != len(right) {
		return false
	}
	for index := range destination {
		destination[index] = left[index] | right[index]
	}
	return true
}

func coveragePopcount(coverage []uint64) uint64 {
	total := uint64(0)
	for _, word := range coverage {
		total += uint64(bits.OnesCount64(word))
	}
	return total
}

// coverageCompare is the total order induced by the universe order: at the
// lowest position where two coverages differ, the one observing that atom
// sorts first. It agrees with coverage equality by construction.
func coverageCompare(left, right []uint64) int {
	if len(left) != len(right) {
		if len(left) < len(right) {
			return -1
		}
		return 1
	}
	for index, word := range left {
		difference := word ^ right[index]
		if difference == 0 {
			continue
		}
		if word&(difference&-difference) != 0 {
			return -1
		}
		return 1
	}
	return 0
}

// coverageHash and coverageJoinHash address the owner's coverage index. The
// join form lets a recurrent join probe the index without materializing its
// result, which keeps the hot lookup allocation-free.
func coverageHash(coverage []uint64) uint64 {
	const offset = 1469598103934665603
	const prime = 1099511628211
	hash := uint64(offset)
	for _, word := range coverage {
		hash = (hash ^ word) * prime
	}
	return hash
}

func coverageJoinHash(left, right []uint64) uint64 {
	const offset = 1469598103934665603
	const prime = 1099511628211
	hash := uint64(offset)
	for index, word := range left {
		hash = (hash ^ (word | right[index])) * prime
	}
	return hash
}

func coverageEqualsJoin(candidate, left, right []uint64) bool {
	if len(candidate) != len(left) || len(left) != len(right) {
		return false
	}
	for index, word := range candidate {
		if word != left[index]|right[index] {
			return false
		}
	}
	return true
}
