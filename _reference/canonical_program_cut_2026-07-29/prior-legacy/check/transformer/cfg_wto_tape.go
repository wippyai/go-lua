package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/engine/solve"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// symbolicWTOEdgeKind describes how a reachable CFG edge crosses the nested
// components of a weak topological ordering. It is topology only: none of the
// kinds imply a row-domain operation, iteration policy, or widening policy.
type symbolicWTOEdgeKind uint8

const (
	symbolicWTOEdgeOutside symbolicWTOEdgeKind = iota
	symbolicWTOEdgeEntry
	symbolicWTOEdgeBody
	symbolicWTOEdgeBackedge
	symbolicWTOEdgeExit
	// A transition exits one component and enters a sibling component. Keeping
	// this distinct prevents a future executor from silently discarding either
	// half of the boundary crossing.
	symbolicWTOEdgeTransition
)

// symbolicWTOTape is an immutable, dense rendering of solve.WTOPlan for the
// reachable part of one CFG. Points are in WTO preorder. All execution-facing
// references are dense indexes; point lookup is an array indexed by cfg.Point.
// Maps used by solve.NewWTOPlan do not escape construction.
type symbolicWTOTape struct {
	points     []symbolicWTOTapePoint
	components []symbolicWTOTapeComponent
	edges      []symbolicWTOTapeEdge
	pointIndex []int32
}

type symbolicWTOTapePoint struct {
	point         cfg.Point
	component     int32 // innermost containing component, or -1
	headComponent int32 // component headed by this point, or -1
	edgeBegin     uint32
	edgeEnd       uint32
}

// symbolicWTOTapeComponent owns the contiguous WTO-preorder range [begin,end).
// Nested component ranges are contained by their parent's range.
type symbolicWTOTapeComponent struct {
	head   uint32
	parent int32
	begin  uint32
	end    uint32
	// depth is bounded by the number of components in this frozen tape, not by
	// an unrelated wire-sized integer. Using int also makes subtraction below
	// exact for every topology that can be represented by the owning slices.
	depth int
}

type symbolicWTOTapeEdge struct {
	from uint32
	to   uint32
	// cond is the polarity of this exact CFG edge occurrence. It cannot be
	// reconstructed from (from,to): valid CFGs may contain the truthy and
	// falsy branch edges between the same two points.
	cond      bool
	kind      symbolicWTOEdgeKind
	component int32 // component relevant to kind, or -1 for outside
	// exitCount and enterCount preserve both sides of a sibling transition.
	exitCount  int
	enterCount int
}

// compileSymbolicWTOTape compiles the canonical reachable RPO into a dense WTO
// tape. Unreachable CFG points and edges incident only to them are deliberately
// absent. A malformed RPO fails closed instead of creating an ambiguous index.
func compileSymbolicWTOTape(graph cfg.Graph) (*symbolicWTOTape, error) {
	if graph == nil {
		return nil, fmt.Errorf("transformer: symbolic WTO tape requires a graph")
	}
	if graph.Size() < 0 {
		return nil, fmt.Errorf("transformer: symbolic WTO graph has negative size")
	}
	rpo := cfg.RPOReadOnly(graph)
	if len(rpo) > 1<<31-1 {
		return nil, fmt.Errorf("transformer: symbolic WTO reachable point count overflows dense index")
	}
	reachable := make([]cfg.Point, 0, len(rpo))
	declared := make([]bool, graph.Size())
	for _, point := range rpo {
		if int(point) >= graph.Size() || graph.Node(point) == nil {
			return nil, fmt.Errorf("transformer: symbolic WTO RPO contains invalid point %d", point)
		}
		if declared[point] {
			return nil, fmt.Errorf("transformer: symbolic WTO RPO repeats point %d", point)
		}
		declared[point] = true
		reachable = append(reachable, point)
	}

	plan := solve.NewWTOPlan(reachable, func(point cfg.Point) []cfg.Point {
		return cfg.SuccessorsReadOnly(graph, point)
	})
	tape := &symbolicWTOTape{
		points:     make([]symbolicWTOTapePoint, 0, len(reachable)),
		pointIndex: make([]int32, graph.Size()),
	}
	for i := range tape.pointIndex {
		tape.pointIndex[i] = -1
	}

	var flatten func([]solve.WTOElement[cfg.Point], int32) error
	flatten = func(elements []solve.WTOElement[cfg.Point], parent int32) error {
		for _, element := range elements {
			if !element.IsComponent() {
				tape.appendPoint(element.Vertex, parent, -1)
				continue
			}
			if len(tape.components) >= 1<<31-1 {
				return fmt.Errorf("transformer: symbolic WTO component count overflows dense index")
			}
			component := int32(len(tape.components))
			depth := 1
			if parent >= 0 {
				parentDepth := tape.components[parent].depth
				depth = parentDepth + 1
			}
			begin := uint32(len(tape.points))
			tape.components = append(tape.components, symbolicWTOTapeComponent{
				parent: parent, begin: begin, depth: depth,
			})
			head := tape.appendPoint(element.Vertex, component, component)
			tape.components[component].head = head
			if err := flatten(element.Body, component); err != nil {
				return err
			}
			tape.components[component].end = uint32(len(tape.points))
		}
		return nil
	}
	if err := flatten(plan.Elements(), -1); err != nil {
		return nil, err
	}
	if len(tape.points) != len(reachable) {
		return nil, fmt.Errorf("transformer: symbolic WTO tape covers %d of %d reachable points", len(tape.points), len(reachable))
	}

	// Graph.Edges, rather than a later EdgeCond(from,to) lookup, is the
	// authoritative edge inventory. The latter is intentionally pair-shaped
	// and therefore cannot distinguish parallel truthy/falsy edges.
	edgesByFrom := make([][]cfg.Edge, len(tape.points))
	graphEdges := graph.Edges()
	seenEdges := make(map[cfg.Edge]struct{}, len(graphEdges))
	for _, edge := range graphEdges {
		from, to := tape.denseIndex(edge.From), tape.denseIndex(edge.To)
		if from < 0 || to < 0 {
			continue
		}
		if !graph.IsBranch(edge.From) {
			edge.Cond = false
		}
		// An exact duplicate carries no additional control-flow meaning. Keep
		// the equation inventory idempotent while retaining opposite branch
		// polarities as distinct cfg.Edge values.
		if _, duplicate := seenEdges[edge]; duplicate {
			continue
		}
		seenEdges[edge] = struct{}{}
		edgesByFrom[from] = append(edgesByFrom[from], edge)
	}
	for from := range tape.points {
		tape.points[from].edgeBegin = uint32(len(tape.edges))
		for _, edge := range edgesByFrom[from] {
			to := tape.denseIndex(edge.To)
			tape.edges = append(tape.edges, tape.classifyEdge(uint32(from), uint32(to), edge.Cond))
		}
		tape.points[from].edgeEnd = uint32(len(tape.edges))
	}
	return tape, nil
}

func (t *symbolicWTOTape) appendPoint(point cfg.Point, component, headComponent int32) uint32 {
	index := uint32(len(t.points))
	t.points = append(t.points, symbolicWTOTapePoint{
		point: point, component: component, headComponent: headComponent,
	})
	t.pointIndex[point] = int32(index)
	return index
}

func (t *symbolicWTOTape) denseIndex(point cfg.Point) int32 {
	if int(point) >= len(t.pointIndex) {
		return -1
	}
	return t.pointIndex[point]
}

func (t *symbolicWTOTape) classifyEdge(from, to uint32, cond bool) symbolicWTOTapeEdge {
	fromComponent := t.points[from].component
	toComponent := t.points[to].component
	fromDepth := t.componentDepth(fromComponent)
	toDepth := t.componentDepth(toComponent)
	common := t.commonComponent(fromComponent, toComponent)
	commonDepth := t.componentDepth(common)
	exits := fromDepth - commonDepth
	enters := toDepth - commonDepth

	edge := symbolicWTOTapeEdge{
		from: from, to: to, cond: cond, component: -1,
		exitCount: exits, enterCount: enters,
	}
	if headed := t.points[to].headComponent; headed >= 0 && t.componentContains(headed, from) {
		edge.kind = symbolicWTOEdgeBackedge
		edge.component = headed
		return edge
	}
	switch {
	case exits == 0 && enters == 0 && common < 0:
		edge.kind = symbolicWTOEdgeOutside
	case exits == 0 && enters == 0:
		edge.kind = symbolicWTOEdgeBody
		edge.component = common
	case exits == 0:
		edge.kind = symbolicWTOEdgeEntry
		edge.component = toComponent
	case enters == 0:
		edge.kind = symbolicWTOEdgeExit
		edge.component = fromComponent
	default:
		edge.kind = symbolicWTOEdgeTransition
		edge.component = common
	}
	return edge
}

func (t *symbolicWTOTape) componentDepth(component int32) int {
	if component < 0 {
		return 0
	}
	return t.components[component].depth
}

func (t *symbolicWTOTape) componentContains(component int32, point uint32) bool {
	item := t.components[component]
	return point >= item.begin && point < item.end
}

func (t *symbolicWTOTape) commonComponent(left, right int32) int32 {
	for t.componentDepth(left) > t.componentDepth(right) {
		left = t.components[left].parent
	}
	for t.componentDepth(right) > t.componentDepth(left) {
		right = t.components[right].parent
	}
	for left != right {
		if left < 0 || right < 0 {
			return -1
		}
		left = t.components[left].parent
		right = t.components[right].parent
	}
	return left
}
