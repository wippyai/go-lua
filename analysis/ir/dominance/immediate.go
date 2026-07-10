package dominance

import "github.com/wippyai/go-lua/analysis/ir/cfg"

type immediateDominatorData struct {
	rpo         []cfg.Point
	rpoNum      []int
	idomByPoint []cfg.Point
	hasIDom     []bool
	preorder    []int
	postorder   []int
}

// ImmediateDominators is a point-indexed dominance view for one immutable CFG.
type ImmediateDominators struct {
	data immediateDominatorData
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

	preorder, postorder := dominanceTreeIntervals(rpo, idomByPoint, hasIDom)
	return immediateDominatorData{
		rpo:         rpo,
		rpoNum:      rpoNum,
		idomByPoint: idomByPoint,
		hasIDom:     hasIDom,
		preorder:    preorder,
		postorder:   postorder,
	}
}

// dominanceTreeIntervals assigns each reachable point an interval in the
// immediate-dominator tree. A dominates B exactly when B's interval is
// contained by A's. Keeping the intervals beside the dense idom data makes
// repeated dominance queries constant time without changing the computed tree.
func dominanceTreeIntervals(rpo []cfg.Point, idomByPoint []cfg.Point, hasIDom []bool) ([]int, []int) {
	preorder := make([]int, len(hasIDom))
	postorder := make([]int, len(hasIDom))
	children := make([][]cfg.Point, len(hasIDom))
	var root cfg.Point
	haveRoot := false
	for _, point := range rpo {
		if !validPoint(point, len(hasIDom)) || !hasIDom[int(point)] {
			continue
		}
		parent := idomByPoint[int(point)]
		if parent == point {
			if !haveRoot {
				root = point
				haveRoot = true
			}
			continue
		}
		if validPoint(parent, len(hasIDom)) && hasIDom[int(parent)] {
			children[int(parent)] = append(children[int(parent)], point)
		}
	}
	if !haveRoot {
		return preorder, postorder
	}
	type frame struct {
		point cfg.Point
		next  int
	}
	clock := 1
	preorder[int(root)] = clock
	clock++
	stack := []frame{{point: root}}
	for len(stack) != 0 {
		top := &stack[len(stack)-1]
		if top.next < len(children[int(top.point)]) {
			child := children[int(top.point)][top.next]
			top.next++
			preorder[int(child)] = clock
			clock++
			stack = append(stack, frame{point: child})
			continue
		}
		postorder[int(top.point)] = clock
		clock++
		stack = stack[:len(stack)-1]
	}
	return preorder, postorder
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
	if !validPoint(pointA, len(d.data.hasIDom)) || !validPoint(pointB, len(d.data.hasIDom)) ||
		!d.data.hasIDom[int(pointA)] || !d.data.hasIDom[int(pointB)] {
		return false
	}
	return d.data.preorder[int(pointA)] <= d.data.preorder[int(pointB)] &&
		d.data.postorder[int(pointB)] <= d.data.postorder[int(pointA)]
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
