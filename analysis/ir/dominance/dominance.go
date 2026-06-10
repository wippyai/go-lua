package dominance

import (
	"slices"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// DomInfo holds dominator tree and dominance frontier information.
type DomInfo struct {
	ImmediateDominators map[cfg.Point]cfg.Point
	DominatorTree       map[cfg.Point][]cfg.Point
	DominanceFrontier   map[cfg.Point][]cfg.Point
}

type predecessorsReader interface {
	PredecessorsReadOnly(cfg.Point) []cfg.Point
}

type successorsReader interface {
	SuccessorsReadOnly(cfg.Point) []cfg.Point
}

type rpoReader interface {
	RPOReadOnly() []cfg.Point
}

type immediateDominatorData struct {
	rpo         []cfg.Point
	rpoNum      []int
	idomByPoint []cfg.Point
	hasIDom     []bool
}

// ImmediateDominators is a point-indexed dominance view for one immutable CFG.
type ImmediateDominators struct {
	data immediateDominatorData
}

func predecessorsOf(g cfg.Graph, point cfg.Point) []cfg.Point {
	if direct, ok := g.(predecessorsReader); ok {
		return direct.PredecessorsReadOnly(point)
	}
	return g.Predecessors(point)
}

func successorsOf(g cfg.Graph, point cfg.Point) []cfg.Point {
	if direct, ok := g.(successorsReader); ok {
		return direct.SuccessorsReadOnly(point)
	}
	return g.Successors(point)
}

func rpoOf(g cfg.Graph) []cfg.Point {
	if direct, ok := g.(rpoReader); ok {
		return direct.RPOReadOnly()
	}
	return g.RPO()
}

func validPoint(point cfg.Point, graphSize int) bool {
	idx := int(point)
	return idx >= 0 && idx < graphSize
}

func buildRPONumbers(rpo []cfg.Point, graphSize int) []int {
	rpoNum := make([]int, graphSize)
	for i := range rpoNum {
		rpoNum[i] = -1
	}
	for i, point := range rpo {
		if validPoint(point, graphSize) {
			rpoNum[int(point)] = i
		}
	}
	return rpoNum
}

func computeImmediateDominatorData(g cfg.Graph) immediateDominatorData {
	if g == nil {
		return immediateDominatorData{}
	}

	rpo := rpoOf(g)
	graphSize := g.Size()
	if len(rpo) == 0 || graphSize == 0 {
		return immediateDominatorData{}
	}

	entry := g.Entry()
	if !validPoint(entry, graphSize) {
		return immediateDominatorData{}
	}

	rpoNum := buildRPONumbers(rpo, graphSize)
	idomByPoint := make([]cfg.Point, graphSize)
	hasIDom := make([]bool, graphSize)
	idomByPoint[int(entry)] = entry
	hasIDom[int(entry)] = true

	intersect := func(pointA, pointB cfg.Point) cfg.Point {
		fingerA, fingerB := pointA, pointB
		for fingerA != fingerB {
			for rpoNum[int(fingerA)] > rpoNum[int(fingerB)] {
				fingerA = idomByPoint[int(fingerA)]
			}
			for rpoNum[int(fingerB)] > rpoNum[int(fingerA)] {
				fingerB = idomByPoint[int(fingerB)]
			}
		}
		return fingerA
	}

	changed := true
	for changed {
		changed = false
		for _, block := range rpo {
			if block == entry || !validPoint(block, graphSize) {
				continue
			}

			preds := predecessorsOf(g, block)
			if len(preds) == 0 {
				continue
			}

			var newIDom cfg.Point
			found := false
			for _, pred := range preds {
				if validPoint(pred, graphSize) && hasIDom[int(pred)] {
					newIDom = pred
					found = true
					break
				}
			}
			if !found {
				continue
			}

			for _, pred := range preds {
				if pred == newIDom {
					continue
				}
				if validPoint(pred, graphSize) && hasIDom[int(pred)] {
					newIDom = intersect(pred, newIDom)
				}
			}

			blockIdx := int(block)
			if !hasIDom[blockIdx] || idomByPoint[blockIdx] != newIDom {
				idomByPoint[blockIdx] = newIDom
				hasIDom[blockIdx] = true
				changed = true
			}
		}
	}

	return immediateDominatorData{
		rpo:         rpo,
		rpoNum:      rpoNum,
		idomByPoint: idomByPoint,
		hasIDom:     hasIDom,
	}
}

func (d immediateDominatorData) asMap() map[cfg.Point]cfg.Point {
	idom := make(map[cfg.Point]cfg.Point, len(d.rpo))
	for _, point := range d.rpo {
		if validPoint(point, len(d.hasIDom)) && d.hasIDom[int(point)] {
			idom[point] = d.idomByPoint[int(point)]
		}
	}
	return idom
}

// ComputeImmediateDominatorInfo computes a dense dominance view.
func ComputeImmediateDominatorInfo(g cfg.Graph) *ImmediateDominators {
	return &ImmediateDominators{data: computeImmediateDominatorData(g)}
}

// Map materializes the immediate-dominator map.
func (d *ImmediateDominators) Map() map[cfg.Point]cfg.Point {
	if d == nil {
		return make(map[cfg.Point]cfg.Point)
	}
	return d.data.asMap()
}

// Dominates reports whether pointA dominates pointB.
func (d *ImmediateDominators) Dominates(pointA, pointB cfg.Point) bool {
	if pointA == pointB {
		return true
	}
	if d == nil {
		return false
	}

	runner := pointB
	for {
		if !validPoint(runner, len(d.data.hasIDom)) || !d.data.hasIDom[int(runner)] {
			return false
		}

		dom := d.data.idomByPoint[int(runner)]
		if dom == runner {
			return false
		}
		if dom == pointA {
			return true
		}
		runner = dom
	}
}

// StrictlyDominates reports whether pointA dominates pointB and the points differ.
func (d *ImmediateDominators) StrictlyDominates(pointA, pointB cfg.Point) bool {
	if pointA == pointB {
		return false
	}
	return d.Dominates(pointA, pointB)
}

// ComputeImmediateDominators computes only the immediate-dominator map.
func ComputeImmediateDominators(g cfg.Graph) map[cfg.Point]cfg.Point {
	return ComputeImmediateDominatorInfo(g).Map()
}

// ComputeDominators computes immediate dominators and the dominator tree.
func ComputeDominators(g cfg.Graph) (map[cfg.Point]cfg.Point, map[cfg.Point][]cfg.Point) {
	data := computeImmediateDominatorData(g)
	idom := data.asMap()
	domTree := make(map[cfg.Point][]cfg.Point, len(idom))

	for point, dom := range idom {
		if point != dom {
			domTree[dom] = append(domTree[dom], point)
		}
	}

	for point := range domTree {
		sortByRPO(domTree[point], data.rpoNum)
	}

	return idom, domTree
}

const minPredecessorsForDominanceFrontier = 2

func computeDominanceFrontierDense(
	g cfg.Graph,
	rpo []cfg.Point,
	rpoNum []int,
	idomByPoint []cfg.Point,
	hasIDom []bool,
) [][]cfg.Point {
	if g == nil {
		return nil
	}

	graphSize := g.Size()
	if graphSize == 0 || len(rpo) == 0 {
		return nil
	}

	dfByPoint := make([][]cfg.Point, graphSize)
	runnerMark := make([]uint32, graphSize)
	var markEpoch uint32 = 1

	for _, block := range rpo {
		if !validPoint(block, graphSize) {
			continue
		}

		preds := predecessorsOf(g, block)
		if len(preds) < minPredecessorsForDominanceFrontier {
			continue
		}

		blockIdx := int(block)
		if !hasIDom[blockIdx] {
			continue
		}
		domBlock := idomByPoint[blockIdx]

		markEpoch++
		if markEpoch == 0 {
			clear(runnerMark)
			markEpoch = 1
		}

		for _, pred := range preds {
			if !validPoint(pred, graphSize) {
				continue
			}

			runner := pred
			for runner != domBlock {
				if !validPoint(runner, graphSize) {
					break
				}

				runnerIdx := int(runner)
				if runnerMark[runnerIdx] == markEpoch {
					break
				}
				runnerMark[runnerIdx] = markEpoch
				dfByPoint[runnerIdx] = append(dfByPoint[runnerIdx], block)

				if !hasIDom[runnerIdx] {
					break
				}

				dom := idomByPoint[runnerIdx]
				if dom == runner {
					break
				}
				runner = dom
			}
		}
	}

	for _, frontier := range dfByPoint {
		sortByRPO(frontier, rpoNum)
	}

	return dfByPoint
}

// ComputeDominanceFrontier computes the dominance frontier for each node.
//
// DF[n] contains each node y where n dominates a predecessor of y but does not
// strictly dominate y.
func ComputeDominanceFrontier(g cfg.Graph, idom map[cfg.Point]cfg.Point) map[cfg.Point][]cfg.Point {
	if g == nil {
		return make(map[cfg.Point][]cfg.Point)
	}

	rpo := rpoOf(g)
	graphSize := g.Size()
	if len(rpo) == 0 || graphSize == 0 {
		return make(map[cfg.Point][]cfg.Point)
	}

	rpoNum := buildRPONumbers(rpo, graphSize)
	idomByPoint := make([]cfg.Point, graphSize)
	hasIDom := make([]bool, graphSize)
	for point, dom := range idom {
		if validPoint(point, graphSize) {
			idomByPoint[int(point)] = dom
			hasIDom[int(point)] = true
		}
	}

	dfByPoint := computeDominanceFrontierDense(g, rpo, rpoNum, idomByPoint, hasIDom)
	df := make(map[cfg.Point][]cfg.Point, len(rpo))
	for pointIdx, frontier := range dfByPoint {
		if len(frontier) > 0 {
			df[cfg.Point(pointIdx)] = frontier
		}
	}
	return df
}

// ComputeDomInfo computes immediate dominators, the dominator tree, and frontiers.
func ComputeDomInfo(g cfg.Graph) *DomInfo {
	idom, domTree := ComputeDominators(g)
	df := ComputeDominanceFrontier(g, idom)
	return &DomInfo{
		ImmediateDominators: idom,
		DominatorTree:       domTree,
		DominanceFrontier:   df,
	}
}

// Dominates returns true if pointA dominates pointB.
func Dominates(idom map[cfg.Point]cfg.Point, pointA, pointB cfg.Point) bool {
	if pointA == pointB {
		return true
	}

	runner := pointB
	for {
		dom, ok := idom[runner]
		if !ok || dom == runner {
			return false
		}
		if dom == pointA {
			return true
		}
		runner = dom
	}
}

// StrictlyDominates returns true if pointA dominates pointB and the points differ.
func StrictlyDominates(idom map[cfg.Point]cfg.Point, pointA, pointB cfg.Point) bool {
	if pointA == pointB {
		return false
	}
	return Dominates(idom, pointA, pointB)
}

type reversedGraph struct {
	g cfg.Graph
}

func (r *reversedGraph) ID() uint64 {
	if r == nil || r.g == nil {
		return 0
	}
	return r.g.ID()
}

func (r *reversedGraph) Entry() cfg.Point {
	if r == nil || r.g == nil {
		return 0
	}
	return r.g.Exit()
}

func (r *reversedGraph) Exit() cfg.Point {
	if r == nil || r.g == nil {
		return 0
	}
	return r.g.Entry()
}

func (r *reversedGraph) Node(point cfg.Point) *cfg.Node {
	if r == nil || r.g == nil {
		return nil
	}
	return r.g.Node(point)
}

func (r *reversedGraph) RPO() []cfg.Point {
	if r == nil || r.g == nil {
		return nil
	}

	graphSize := r.g.Size()
	visited := make([]bool, graphSize)
	order := make([]cfg.Point, 0, graphSize)

	var visit func(cfg.Point)
	visit = func(point cfg.Point) {
		if !validPoint(point, graphSize) || visited[int(point)] {
			return
		}
		visited[int(point)] = true

		for _, succ := range predecessorsOf(r.g, point) {
			visit(succ)
		}
		order = append(order, point)
	}

	visit(r.Entry())
	slices.Reverse(order)
	return order
}

func (r *reversedGraph) Predecessors(point cfg.Point) []cfg.Point {
	if r == nil || r.g == nil {
		return nil
	}
	return successorsOf(r.g, point)
}

func (r *reversedGraph) Successors(point cfg.Point) []cfg.Point {
	if r == nil || r.g == nil {
		return nil
	}
	return predecessorsOf(r.g, point)
}

func (r *reversedGraph) Edges() []cfg.Edge {
	if r == nil || r.g == nil {
		return nil
	}

	edges := r.g.Edges()
	reversed := make([]cfg.Edge, len(edges))
	for i, edge := range edges {
		reversed[i] = cfg.Edge{From: edge.To, To: edge.From, Cond: edge.Cond}
	}
	return reversed
}

func (r *reversedGraph) Size() int {
	if r == nil || r.g == nil {
		return 0
	}
	return r.g.Size()
}

func (r *reversedGraph) EdgeCond(from, to cfg.Point) (bool, bool) {
	if r == nil || r.g == nil {
		return false, false
	}
	return r.g.EdgeCond(to, from)
}

func (r *reversedGraph) IsJoin(point cfg.Point) bool {
	return r != nil && r.g != nil && r.g.IsBranch(point)
}

func (r *reversedGraph) IsBranch(point cfg.Point) bool {
	return r != nil && r.g != nil && r.g.IsJoin(point)
}

// ComputePostDominators computes post-dominators by running dominance on the
// reversed CFG.
func ComputePostDominators(g cfg.Graph) (map[cfg.Point]cfg.Point, map[cfg.Point][]cfg.Point) {
	if g == nil {
		return make(map[cfg.Point]cfg.Point), make(map[cfg.Point][]cfg.Point)
	}
	return ComputeDominators(&reversedGraph{g: g})
}

// ComputeImmediatePostDominators computes only the immediate post-dominator map.
func ComputeImmediatePostDominators(g cfg.Graph) map[cfg.Point]cfg.Point {
	if g == nil {
		return make(map[cfg.Point]cfg.Point)
	}
	return ComputeImmediateDominatorInfo(&reversedGraph{g: g}).Map()
}

// PostDominates returns true if pointA post-dominates pointB.
func PostDominates(postIDom map[cfg.Point]cfg.Point, pointA, pointB cfg.Point) bool {
	return Dominates(postIDom, pointA, pointB)
}

func sortByRPO(points []cfg.Point, rpoNum []int) {
	if len(points) <= 1 {
		return
	}
	slices.SortFunc(points, func(a, b cfg.Point) int {
		aNum, bNum := -1, -1
		if validPoint(a, len(rpoNum)) {
			aNum = rpoNum[int(a)]
		}
		if validPoint(b, len(rpoNum)) {
			bNum = rpoNum[int(b)]
		}
		if aNum != bNum {
			return aNum - bNum
		}
		return int(a) - int(b)
	})
}
