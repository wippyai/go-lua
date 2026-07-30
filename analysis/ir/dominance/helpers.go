package dominance

import (
	"slices"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func predecessorsOf(g cfg.Graph, point cfg.Point) []cfg.Point {
	return cfg.PredecessorsReadOnly(g, point)
}

func successorsOf(g cfg.Graph, point cfg.Point) []cfg.Point {
	return cfg.SuccessorsReadOnly(g, point)
}

func rpoOf(g cfg.Graph) []cfg.Point {
	return cfg.RPOReadOnly(g)
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
