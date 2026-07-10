package dominance

import "github.com/wippyai/go-lua/analysis/ir/cfg"

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
