package dominance

import (
	"slices"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

type predecessorsReader interface {
	PredecessorsReadOnly(cfg.Point) []cfg.Point
}

type successorsReader interface {
	SuccessorsReadOnly(cfg.Point) []cfg.Point
}

type rpoReader interface {
	RPOReadOnly() []cfg.Point
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
