package body

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/factquery"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// ReadGraph is the immutable, preparation-derived graph of intrabody State
// reads performed by the authoritative concrete transfer. Interprocedural
// callee tuple dependencies are deliberately not represented here: the program
// fixpoint owns those as typed call edges.
type ReadGraph struct {
	graph cfg.Graph
	node  [][]cfg.Point
	edge  [][]cfg.Point
}

// NodeReads returns the sorted unique intrabody points which may be read while
// applying the authoritative node transfer at point.
func (g ReadGraph) NodeReads(point cfg.Point) []cfg.Point {
	if uint64(point) >= uint64(len(g.node)) {
		return nil
	}
	return append([]cfg.Point(nil), g.node[point]...)
}

// EdgeReads returns the sorted unique intrabody points which may be read while
// applying the authoritative edge transfer. A non-edge has no dependencies.
func (g ReadGraph) EdgeReads(from, to cfg.Point) []cfg.Point {
	if g.graph == nil || uint64(from) >= uint64(len(g.edge)) || !hasSuccessor(g.graph, from, to) {
		return nil
	}
	return append([]cfg.Point(nil), g.edge[from]...)
}

func hasSuccessor(graph cfg.Graph, from, to cfg.Point) bool {
	for _, next := range graph.Successors(from) {
		if next == to {
			return true
		}
	}
	return false
}

func compileReadGraph(s *Static) ReadGraph {
	if s == nil || s.cfg == nil || s.cfg.Graph == nil || s.operationPlan == nil {
		return ReadGraph{}
	}
	graph := s.cfg.Graph
	out := ReadGraph{
		graph: graph,
		node:  make([][]cfg.Point, graph.Size()),
		edge:  make([][]cfg.Point, graph.Size()),
	}
	for raw := 0; raw < graph.Size(); raw++ {
		point := cfg.Point(raw)
		nodeSources, edgeSources := pointReadSources(s.operationPlan, point)
		out.node[raw] = sourceReadClosure(point, s.facts, nodeSources)
		out.edge[raw] = sourceReadClosure(point, s.facts, edgeSources)
		appendGenericForReads(s, point, &out.node[raw])
		out.node[raw] = sortedUniquePoints(out.node[raw])
	}
	return out
}

func pointReadSources(plan *operationplan.Plan, point cfg.Point) (node, edge []factflow.ValueSource) {
	if plan == nil {
		return nil, nil
	}
	facts := plan.Facts()
	cursor := plan.Cursor(point)
	for {
		cell, ok := cursor.Next()
		if !ok {
			break
		}
		switch cell.Kind() {
		case operationplan.RootAssignment:
			if fact, ok := facts.RootAssignment(point); ok {
				node = append(node, fact.Source())
			}
		case operationplan.PathAssignment:
			if fact, ok := facts.PathAssignment(point); ok {
				node = append(node, fact.Source())
			}
		case operationplan.PathStaticMemberWrite:
			if fact, ok := facts.PathStaticMemberWrite(point); ok {
				node = append(node, fact.Source())
			}
		case operationplan.DynamicIndexWrite:
			if fact, ok := facts.DynamicIndexWrite(point); ok {
				switch fact.ReadbackIntent() {
				case factflow.DynamicIndexReadbackKey:
					node = append(node, fact.KeySource())
				case factflow.DynamicIndexReadbackValue:
					node = append(node, fact.Source())
				case factflow.DynamicIndexReadbackKeyAndValue:
					node = append(node, fact.KeySource(), fact.Source())
				}
			}
		case operationplan.PathDescendantInvalidation:
			if fact, ok := facts.PathDescendantInvalidation(point); ok {
				if _, source, _, ok := fact.DynamicTargetRef(); ok {
					node = append(node, source)
				}
			}
		case operationplan.Return:
			if fact, ok := facts.Return(point); ok {
				node = append(node, fact.Sources()...)
			}
		case operationplan.CallSite:
			if site, ok := facts.CallSiteView(point); ok {
				if source, ok := site.ReceiverSource(); ok {
					node = append(node, source)
				}
				site.ForEachArgumentSource(func(_ int, source factflow.ValueSource) bool {
					node = append(node, source)
					return true
				})
			}
		case operationplan.BranchConditionSource:
			if source, ok := facts.BranchConditionSource(point); ok {
				edge = append(edge, source)
			}
		}
	}
	return node, edge
}

// sourceReadClosure follows the immutable expression/source DAG iteratively.
// Cycles are legal malformed-input boundaries and terminate by identity, never
// by an artificial depth or work budget.
func sourceReadClosure(point cfg.Point, facts factflow.Facts, roots []factflow.ValueSource) []cfg.Point {
	type sourceTask struct {
		point  cfg.Point
		source factflow.ValueSource
	}
	type expressionTask struct {
		point cfg.Point
		expr  factflow.ExprRef
	}
	stack := make([]sourceTask, len(roots))
	for index, source := range roots {
		stack[index] = sourceTask{point: point, source: source}
	}
	seenExpr := make(map[expressionTask]struct{}, len(stack))
	seenMaterializedCall := make(map[cfg.Point]struct{})
	reads := make([]cfg.Point, 0, len(stack))
	appendMaterializedCallSources := func(callPoint cfg.Point) {
		if _, seen := seenMaterializedCall[callPoint]; seen {
			return
		}
		seenMaterializedCall[callPoint] = struct{}{}
		site, ok := facts.CallSiteView(callPoint)
		if !ok {
			return
		}
		if receiver, ok := site.ReceiverSource(); ok {
			stack = append(stack, sourceTask{point: callPoint, source: receiver})
		}
		site.ForEachArgumentSource(func(_ int, source factflow.ValueSource) bool {
			stack = append(stack, sourceTask{point: callPoint, source: source})
			return true
		})
	}
	for len(stack) != 0 {
		task := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		source := task.source
		if source.HasSourcePoint && source.SourcePoint != 0 && source.SourcePoint != task.point {
			reads = append(reads, source.SourcePoint)
			appendMaterializedCallSources(source.SourcePoint)
		}
		if source.HasCallPoint && source.CallPoint != 0 && source.CallPoint != task.point {
			reads = append(reads, source.CallPoint)
			appendMaterializedCallSources(source.CallPoint)
		}
		if !source.HasExpr || source.ExprRef == 0 {
			continue
		}
		expression := expressionTask{point: task.point, expr: source.ExprRef}
		if _, ok := seenExpr[expression]; ok {
			continue
		}
		seenExpr[expression] = struct{}{}
		if refinement, ok := facts.ExpressionRefinement(source.ExprRef); ok {
			stack = append(stack, sourceTask{point: task.point, source: refinement.Source()})
		}
		if operation, ok := facts.ExpressionOperation(source.ExprRef); ok {
			stack = append(stack, sourceTask{point: task.point, source: operation.Left()})
			if operation.Kind() == factflow.ExpressionOperationBinary {
				stack = append(stack, sourceTask{point: task.point, source: operation.Right()})
			}
		}
		if dynamic, ok := facts.DynamicIndexExpression(source.ExprRef); ok {
			stack = append(stack, sourceTask{point: task.point, source: dynamic.KeySource()})
			if table, ok := dynamic.TableSource(); ok {
				stack = append(stack, sourceTask{point: task.point, source: table})
			}
		}
		if literal, ok := facts.ObjectLiteralView(source.ExprRef); ok {
			literal.ForEachEntry(func(entry factflow.ObjectEntryView) bool {
				stack = append(stack, sourceTask{point: task.point, source: entry.Source()})
				return true
			})
			if list, ok := literal.ListElementSource(); ok {
				stack = append(stack, sourceTask{point: task.point, source: list})
			}
		}
	}
	return sortedUniquePoints(reads)
}

func appendGenericForReads(s *Static, point cfg.Point, reads *[]cfg.Point) {
	operation, ok := s.operationPlan.GenericForOperation(point)
	if !ok {
		return
	}
	for i := 0; i < operation.ProtocolSourceCount(); i++ {
		source, _ := operation.ProtocolSource(i)
		if source.HasCallPoint && source.CallPoint != 0 && source.CallPoint != point {
			*reads = append(*reads, source.CallPoint)
		}
	}
	source, ok := operation.ProtocolSource(0)
	if !ok || !source.HasCallPoint {
		return
	}
	site, ok := s.facts.CallSiteView(source.CallPoint)
	if !ok {
		return
	}
	appendSource := func(argument factflow.ValueSource, inspectDeclaration bool) {
		*reads = append(*reads, sourceReadClosure(point, s.facts, []factflow.ValueSource{argument})...)
		if !inspectDeclaration {
			return
		}
		path, ok := valueSourcePath(s.facts, s.visibility, argument)
		if !ok || path.Symbol == 0 || len(path.Segments) == 0 {
			return
		}
		declaration, ok := factquery.DominatingPathRootDeclarationSource(point, path, s.facts, s.cfg.Graph)
		if !ok {
			return
		}
		if declaration.Point != point {
			*reads = append(*reads, declaration.Point)
		}
		*reads = append(*reads, sourceReadClosure(declaration.Point, s.facts, []factflow.ValueSource{declaration.Source})...)
	}
	if receiver, ok := site.ReceiverSource(); ok {
		appendSource(receiver, false)
	}
	site.ForEachArgumentSource(func(_ int, argument factflow.ValueSource) bool {
		appendSource(argument, true)
		return true
	})
}

func sortedUniquePoints(points []cfg.Point) []cfg.Point {
	if len(points) == 0 {
		return nil
	}
	sort.Slice(points, func(i, j int) bool { return points[i] < points[j] })
	out := points[:1]
	for _, point := range points[1:] {
		if point != out[len(out)-1] {
			out = append(out, point)
		}
	}
	return out
}
