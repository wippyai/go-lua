package recurrence

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/sourcecontrol"
)

const unassignedComponent = ^uint32(0)

// components is Seal-local SCC state. It owns no Terms and is discarded
// before Result publication. The only graph consulted here is the already
// sealed sourcecontrol CSR exposed by Result's typed queries.
type components struct {
	of       []uint32
	sizes    []uint32
	selfLoop []bool
	cyclic   []bool
}

func deriveComponents(graph *sourcecontrol.Result) (components, error) {
	var empty components
	if graph == nil || graph.NodeCount() == 0 || !graph.VertexCatalogAvailable() {
		return empty, errors.New("program/flow/recurrence: sourcecontrol graph is unavailable")
	}
	nodeCount := graph.NodeCount()
	if uint64(nodeCount) >= uint64(maxSliceInt()) {
		return empty, errors.New("program/flow/recurrence: coordinate denominator is not indexable")
	}

	// Kosaraju's first pass is iterative. The explicit frame keeps recursion
	// depth out of the seal path even for a generated deep body.
	visited := make([]bool, int(nodeCount))
	finish := make([]uint32, 0)
	for ordinal := 0; ordinal < int(nodeCount); ordinal++ {
		start, canonical := graph.CanonicalNodeAt(ordinal)
		if !canonical {
			return empty, errors.New("program/flow/recurrence: canonical vertex permutation is unavailable")
		}
		if !graph.Reachable(start) || visited[start] {
			continue
		}
		visited[start] = true
		frames := []dfsFrame{{node: start}}
		for len(frames) != 0 {
			last := len(frames) - 1
			frame := &frames[last]
			count := graph.SuccessorCount(frame.node)
			if frame.next < count {
				next, ok := graph.SuccessorAt(frame.node, frame.next)
				frame.next++
				if !ok || !graph.Reachable(next) || visited[next] {
					continue
				}
				visited[next] = true
				frames = append(frames, dfsFrame{node: next})
				continue
			}
			finish = append(finish, frame.node)
			frames = frames[:last]
		}
	}

	of := make([]uint32, int(nodeCount))
	for index := range of {
		of[index] = unassignedComponent
	}
	sizes := make([]uint32, 0)
	for index := len(finish) - 1; index >= 0; index-- {
		start := finish[index]
		if of[start] != unassignedComponent {
			continue
		}
		component := uint32(len(sizes))
		size := uint32(0)
		stack := []uint32{start}
		of[start] = component
		for len(stack) != 0 {
			last := len(stack) - 1
			node := stack[last]
			stack = stack[:last]
			size++
			count := graph.PredecessorCount(node)
			for edge := 0; edge < count; edge++ {
				previous, ok := graph.PredecessorAt(node, edge)
				if !ok || !graph.Reachable(previous) || of[previous] != unassignedComponent {
					continue
				}
				of[previous] = component
				stack = append(stack, previous)
			}
		}
		sizes = append(sizes, size)
	}
	if len(finish) == 0 {
		return empty, errors.New("program/flow/recurrence: reachable graph has no node")
	}
	selfLoop := make([]bool, len(sizes))
	for node := uint32(0); node < nodeCount; node++ {
		if !graph.Reachable(node) {
			continue
		}
		component := of[node]
		if component == unassignedComponent || uint64(component) >= uint64(len(selfLoop)) {
			return empty, errors.New("program/flow/recurrence: reachable node lacks SCC")
		}
		for edge := 0; edge < graph.SuccessorCount(node); edge++ {
			to, ok := graph.SuccessorAt(node, edge)
			if ok && to == node {
				selfLoop[component] = true
			}
		}
	}
	cyclic := make([]bool, len(sizes))
	for index, size := range sizes {
		cyclic[index] = size > 1 || selfLoop[index]
	}
	return components{of: of, sizes: sizes, selfLoop: selfLoop, cyclic: cyclic}, nil
}

type dfsFrame struct {
	node uint32
	next int
}

func maxSliceInt() int {
	return int(^uint(0) >> 1)
}
