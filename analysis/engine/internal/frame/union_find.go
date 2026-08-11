package frame

// disjointSet is compile-local. Its representative array is discarded after
// class expansion, so it cannot become a second runtime semantic authority.
type disjointSet struct {
	parent []int
	size   []uint32
}

func newDisjointSet(count int) *disjointSet {
	sets := &disjointSet{parent: make([]int, count), size: make([]uint32, count)}
	for index := range sets.parent {
		sets.parent[index] = index
		sets.size[index] = 1
	}
	return sets
}

func (sets *disjointSet) find(index int) int {
	root := index
	for sets.parent[root] != root {
		root = sets.parent[root]
	}
	for index != root {
		next := sets.parent[index]
		sets.parent[index] = root
		index = next
	}
	return root
}

func (sets *disjointSet) union(left, right int) {
	left, right = sets.find(left), sets.find(right)
	if left == right {
		return
	}
	if sets.size[left] < sets.size[right] {
		left, right = right, left
	}
	sets.parent[right] = left
	sets.size[left] += sets.size[right]
}
