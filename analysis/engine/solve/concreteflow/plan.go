// Package concreteflow executes certified CFG equation systems over dense
// point-indexed storage. It owns scheduling and scratch only; semantic work is
// still performed by the canonical transfer equation supplied by transfer.
package concreteflow

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/solve"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

type opcode uint8

const (
	opVertex opcode = iota + 1
	opLoopBegin
	opLoopEnd
)

type instruction struct {
	op    opcode
	point cfg.Point
	jump  uint32
}

// Plan is an immutable certification and instruction tape for one prepared
// CFG. A nil plan means the ordinary solver remains authoritative.
type Plan struct {
	graph      cfg.Graph
	wto        *solve.WTOPlan[cfg.Point]
	tape       []instruction
	identity   []bool
	nodeWork   []bool
	edgeWork   []bool
	maxNesting int
}

// Compile accepts only a complete, reducible CFG whose point rows follow the
// canonical operation barriers. Irreducible components fail closed.
func Compile(graph cfg.Graph, operations *operationplan.Plan, wto *solve.WTOPlan[cfg.Point]) (*Plan, error) {
	if graph == nil || operations == nil || wto == nil {
		return nil, fmt.Errorf("concreteflow: graph, operation plan, and WTO are required")
	}
	n := graph.Size()
	if operations.PointCount() != n {
		return nil, fmt.Errorf("concreteflow: operation rows=%d graph points=%d", operations.PointCount(), n)
	}
	cells := append([]cfg.Point(nil), graph.RPO()...)
	if len(cells) != n || !wto.Matches(cells) {
		return nil, fmt.Errorf("concreteflow: WTO does not cover the dense CFG")
	}
	seen := make([]bool, n)
	identity := make([]bool, n)
	nodeWork := make([]bool, n)
	edgeWork := make([]bool, n)
	indegree := make([]uint32, n)
	for p := cfg.Point(0); int(p) < n; p++ {
		for _, succ := range cfg.SuccessorsReadOnly(graph, p) {
			if uint64(succ) >= uint64(n) {
				return nil, fmt.Errorf("concreteflow: edge %d -> %d leaves CFG", p, succ)
			}
			indegree[succ]++
		}
	}
	for p := cfg.Point(0); int(p) < n; p++ {
		cursor := operations.Cursor(p)
		hasWork := false
		var last operationplan.Barrier
		for {
			cell, ok := cursor.Next()
			if !ok {
				break
			}
			meta, exists := operationplan.Describe(cell.Kind())
			if !exists || meta.Barrier < last {
				return nil, fmt.Errorf("concreteflow: non-canonical operation row at %d", p)
			}
			hasWork = true
			if meta.Phase == operationplan.Node {
				nodeWork[p] = true
			}
			if meta.Phase == operationplan.Edge || meta.Stages.Has(operationplan.E5CallEffects) {
				edgeWork[p] = true
			}
			last = meta.Barrier
		}
		succ := cfg.SuccessorsReadOnly(graph, p)
		identity[p] = !hasWork && !graph.IsBranch(p) && len(succ) == 1 && indegree[succ[0]] == 1
	}

	elements := wto.Elements()
	if !certifyReducible(graph, elements) {
		return nil, fmt.Errorf("concreteflow: irreducible WTO component")
	}
	plan := &Plan{graph: graph, wto: wto, identity: identity, nodeWork: nodeWork, edgeWork: edgeWork}
	var compilePartition func([]solve.WTOElement[cfg.Point], int) error
	compilePartition = func(items []solve.WTOElement[cfg.Point], depth int) error {
		if depth > plan.maxNesting {
			plan.maxNesting = depth
		}
		for _, item := range items {
			if uint64(item.Vertex) >= uint64(n) || seen[item.Vertex] {
				return fmt.Errorf("concreteflow: invalid or duplicate WTO point %d", item.Vertex)
			}
			seen[item.Vertex] = true
			if !item.IsComponent() {
				plan.tape = append(plan.tape, instruction{op: opVertex, point: item.Vertex})
				continue
			}
			begin := len(plan.tape)
			plan.tape = append(plan.tape, instruction{op: opLoopBegin, point: item.Vertex})
			if err := compilePartition(item.Body, depth+1); err != nil {
				return err
			}
			end := len(plan.tape)
			plan.tape = append(plan.tape, instruction{op: opLoopEnd, jump: uint32(begin)})
			plan.tape[begin].jump = uint32(end + 1)
		}
		return nil
	}
	if err := compilePartition(elements, 1); err != nil {
		return nil, err
	}
	for point, ok := range seen {
		if !ok {
			return nil, fmt.Errorf("concreteflow: point %d absent from WTO tape", point)
		}
	}
	return plan, nil
}

// HasNodeWork and HasEdgeWork expose the certified phase ownership without
// exposing mutable plan storage.
func (p *Plan) HasNodeWork(point cfg.Point) bool {
	return p != nil && uint64(point) < uint64(len(p.nodeWork)) && p.nodeWork[point]
}

func (p *Plan) HasEdgeWork(point cfg.Point) bool {
	return p != nil && uint64(point) < uint64(len(p.edgeWork)) && p.edgeWork[point]
}

func certifyReducible(graph cfg.Graph, elements []solve.WTOElement[cfg.Point]) bool {
	var check func([]solve.WTOElement[cfg.Point]) bool
	check = func(items []solve.WTOElement[cfg.Point]) bool {
		for _, item := range items {
			if !item.IsComponent() {
				continue
			}
			members := make(map[cfg.Point]struct{})
			var collect func(solve.WTOElement[cfg.Point])
			collect = func(element solve.WTOElement[cfg.Point]) {
				members[element.Vertex] = struct{}{}
				for _, child := range element.Body {
					collect(child)
				}
			}
			collect(item)
			for member := range members {
				if member == item.Vertex {
					continue
				}
				for _, pred := range cfg.PredecessorsReadOnly(graph, member) {
					if _, inside := members[pred]; !inside {
						return false
					}
				}
			}
			if !check(item.Body) {
				return false
			}
		}
		return true
	}
	return check(elements)
}
