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

// EveryPathTakesEdge reports that control cannot reach to from from without
// traversing the edge cutFrom -> cutTo. It is the topology question a guarded
// conclusion asks: a fact proven on one branch edge holds at a later point
// exactly when no path reaches that point around the edge.
//
// It is a single walk over the graph with the one edge removed, so a caller
// never rebuilds a dominator relation of its own. A point that is not in the
// graph, or that equals from, is answered false: nothing is proven about a
// coordinate the walk cannot place.
func EveryPathTakesEdge(graph Graph, from, to, cutFrom, cutTo Point) bool {
	if graph == nil || from == to {
		return false
	}
	if !pointInGraph(graph, from) || !pointInGraph(graph, to) ||
		!pointInGraph(graph, cutFrom) || !pointInGraph(graph, cutTo) {
		return false
	}
	// A target the source also reaches on its other condition is reached
	// whichever edge is taken, so cutting one of the two parallel edges would
	// remove a path the graph still has.
	if parallelEdgeTargets(graph, cutFrom, cutTo) {
		return false
	}
	words := (graph.Size() + 63) / 64
	visited := make([]uint64, words)
	bitsetAdd(visited, from)
	stack := []Point{from}
	for len(stack) != 0 {
		point := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if point == to {
			return false
		}
		for _, next := range SuccessorsReadOnly(graph, point) {
			if point == cutFrom && next == cutTo {
				continue
			}
			if !pointInGraph(graph, next) || bitsetHas(visited, next) {
				continue
			}
			bitsetAdd(visited, next)
			stack = append(stack, next)
		}
	}
	return true
}

// parallelEdgeTargets reports that from reaches to on more than one of its
// outgoing edges.
func parallelEdgeTargets(graph Graph, from, to Point) bool {
	count := 0
	for _, successor := range SuccessorsReadOnly(graph, from) {
		if successor == to {
			count++
		}
	}
	return count > 1
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
