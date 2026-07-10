package cfg

// Reachability memoizes forward reachability for one immutable CFG.
//
// It is intentionally a CFG-topology object, not a diagnostic or summary
// helper: consumers should ask the graph layer whether one point can reach
// another instead of each layer building its own graph-walk cache.
type Reachability struct {
	graph Graph
	sets  map[Point][]uint64
	stack []Point
	words int
}

// NewReachability returns a reusable reachability query for graph.
func NewReachability(graph Graph) *Reachability {
	if graph == nil {
		return nil
	}
	size := graph.Size()
	if size < 0 {
		size = 0
	}
	return &Reachability{
		graph: graph,
		sets:  make(map[Point][]uint64),
		words: (size + 63) / 64,
	}
}

// CanReach reports whether control can flow from from to to. A point reaches
// itself, including entry and exit points.
func (r *Reachability) CanReach(from, to Point) bool {
	if r == nil || r.graph == nil {
		return false
	}
	if from == to {
		return true
	}
	if !pointInGraph(r.graph, from) || !pointInGraph(r.graph, to) {
		return false
	}
	return bitsetHas(r.reachableSet(from), to)
}

func (r *Reachability) reachableSet(from Point) []uint64 {
	if set, ok := r.sets[from]; ok {
		return set
	}
	set := make([]uint64, r.words)
	bitsetAdd(set, from)
	r.stack = append(r.stack[:0], SuccessorsReadOnly(r.graph, from)...)
	for len(r.stack) != 0 {
		point := r.stack[len(r.stack)-1]
		r.stack = r.stack[:len(r.stack)-1]
		if !pointInGraph(r.graph, point) || bitsetHas(set, point) {
			continue
		}
		bitsetAdd(set, point)
		r.stack = append(r.stack, SuccessorsReadOnly(r.graph, point)...)
	}
	r.sets[from] = set
	return set
}

func pointInGraph(graph Graph, point Point) bool {
	return graph != nil && int(point) >= 0 && int(point) < graph.Size()
}

func bitsetHas(set []uint64, point Point) bool {
	i := int(point)
	word := i / 64
	if word < 0 || word >= len(set) {
		return false
	}
	return set[word]&(uint64(1)<<uint(i%64)) != 0
}

func bitsetAdd(set []uint64, point Point) {
	i := int(point)
	word := i / 64
	if word < 0 || word >= len(set) {
		return
	}
	set[word] |= uint64(1) << uint(i%64)
}
