package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/dominance"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// diagnosticFlowCache memoizes immutable CFG facts used while producing
// diagnostics for one solved body.Result. Diagnostics often ask the same
// reachability and definition questions from multiple producers; keeping this
// cache per result preserves deterministic evidence while avoiding repeated
// whole-graph walks.
type diagnosticFlowCache struct {
	result *body.Result
	graph  cfg.Graph
	rpo    []cfg.Point

	reachableFrom map[cfg.Point]map[cfg.Point]struct{}
	idom          map[cfg.Point]cfg.Point
	functionDefs  map[symbol.ID][]cfg.Point
	rootAssigns   map[symbol.ID][]cfg.Point
	indexed       bool
}

func newDiagnosticFlowCache(result *body.Result) *diagnosticFlowCache {
	if result == nil {
		return nil
	}
	graph := result.Graph()
	if graph == nil {
		return &diagnosticFlowCache{result: result}
	}
	return &diagnosticFlowCache{
		result:        result,
		graph:         graph,
		rpo:           append([]cfg.Point(nil), graph.RPO()...),
		reachableFrom: make(map[cfg.Point]map[cfg.Point]struct{}),
	}
}

func (c *diagnosticFlowCache) canReach(from, to cfg.Point) bool {
	if c == nil || c.graph == nil || from == 0 || to == 0 {
		return false
	}
	if from == to {
		return true
	}
	reachable := c.reachableSet(from)
	_, ok := reachable[to]
	return ok
}

func (c *diagnosticFlowCache) reachableSet(from cfg.Point) map[cfg.Point]struct{} {
	if reachable, ok := c.reachableFrom[from]; ok {
		return reachable
	}
	reachable := map[cfg.Point]struct{}{from: {}}
	stack := append([]cfg.Point(nil), c.graph.Successors(from)...)
	for len(stack) != 0 {
		point := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if _, ok := reachable[point]; ok {
			continue
		}
		reachable[point] = struct{}{}
		stack = append(stack, c.graph.Successors(point)...)
	}
	c.reachableFrom[from] = reachable
	return reachable
}

func (c *diagnosticFlowCache) immediateDominators() map[cfg.Point]cfg.Point {
	if c == nil || c.graph == nil {
		return nil
	}
	if c.idom == nil {
		c.idom = dominance.ComputeImmediateDominatorInfo(c.graph).Map()
	}
	return c.idom
}

func diagnosticCanReach(flow *diagnosticFlowCache, graph cfg.Graph, from, to cfg.Point) bool {
	if flow != nil {
		return flow.canReach(from, to)
	}
	return cfg.PointCanReach(graph, from, to)
}

func (c *diagnosticFlowCache) directFunctionReassignedAfterDefinition(point cfg.Point, id symbol.ID) bool {
	if c == nil || id == 0 {
		return false
	}
	c.ensureIndexed()
	for _, defPoint := range c.functionDefs[id] {
		if !c.canReach(defPoint, point) {
			continue
		}
		for _, candidate := range c.rootAssigns[id] {
			if candidate == defPoint {
				continue
			}
			if c.canReach(defPoint, candidate) && c.canReach(candidate, point) {
				return true
			}
		}
	}
	return false
}

func (c *diagnosticFlowCache) ensureIndexed() {
	if c == nil || c.indexed {
		return
	}
	c.indexed = true
	c.functionDefs = make(map[symbol.ID][]cfg.Point)
	c.rootAssigns = make(map[symbol.ID][]cfg.Point)
	if c.result == nil || c.graph == nil {
		return
	}
	for _, point := range c.rpo {
		if fact, ok := c.result.LocalAssignment(point); ok && fact.HasSymbol && fact.Symbol != 0 {
			if _, ok := directFunctionExprFromExpr(fact.Expr); ok {
				c.functionDefs[fact.Symbol] = append(c.functionDefs[fact.Symbol], point)
			}
		}
		if fact, ok := c.result.FunctionDefinition(point); ok && fact.HasTargetSymbol && fact.TargetSymbol != 0 && fact.Func != nil {
			c.functionDefs[fact.TargetSymbol] = append(c.functionDefs[fact.TargetSymbol], point)
		}
		if fact, ok := c.result.OrdinaryAssignment(point); ok && fact.HasSymbol && fact.Symbol != 0 && ordinaryAssignmentInvalidatesRootCallResult(fact) {
			c.rootAssigns[fact.Symbol] = append(c.rootAssigns[fact.Symbol], point)
		}
	}
}
